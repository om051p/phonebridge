#!/usr/bin/env python3
"""Spike 04 — prediction-safety comparison of frame-throttling strategies.

WHY THIS EXISTS
---------------
Spike 04 found the Android hardware encoder ignores KEY_MAX_FPS_TO_ENCODER and
keeps encoding at the panel rate (~120 fps), so a frame-rate cap has to be
applied by dropping access units downstream. Naive "keep every Nth AU" dropping
removes P-frames whose *references* were also removed, which corrupts the H.264
predictive chain.

HOW PREDICTION CORRECTNESS IS PROVEN (not merely "it decoded")
-------------------------------------------------------------
"ffmpeg decoded the stream" is NOT evidence of correctness: ffmpeg conceals
errors by substituting a previous frame, so a corrupt stream still yields N
frames. To separate the two we exploit the fact that **H.264 decoding is
deterministic for identical slice data and identical references**. Therefore:

    if every kept frame's references are present, the decoder reconstructs
    BYTE-IDENTICAL pixels to the reference decode  ->  SSIM == 1.0 exactly.

Any missing reference forces concealment and the pixels diverge, so SSIM drops
below 1. A per-frame SSIM of exactly 1.0 across every kept frame is therefore a
positive proof of an intact predictive chain, and any frame below 1.0 is a
concrete, countable prediction error.

All variants are derived from ONE reference capture, using the AU-boundary index
the receiver recorded. Because content is a function of elapsedRealtime() it is
not phase-stable across captures, so deriving every variant from the same
bitstream is what makes the comparison valid and the frame alignment exact.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
from dataclasses import dataclass, field

# ---------------------------------------------------------------- AU model


@dataclass
class AU:
    idx: int
    size: int
    ts: int
    idr: bool
    marker: bool


def load_aus(idx_path: str) -> list[AU]:
    aus: list[AU] = []
    with open(idx_path) as fh:
        for line in fh:
            parts = line.split()
            if len(parts) < 5:
                continue
            aus.append(AU(int(parts[0]), int(parts[1]), int(parts[2]),
                          parts[3] == "1", parts[4] == "1"))
    return aus


def gop_layout(aus: list[AU]) -> tuple[list[int], list[int]]:
    """Returns (gop_of_each_au, gop_lengths). GOP 0 starts at AU 0."""
    gop_of: list[int] = []
    lengths: list[int] = []
    g = -1
    for au in aus:
        if au.idr:
            g += 1
            lengths.append(0)
        gop_of.append(max(g, 0))
        lengths[max(g, 0)] += 1
    return gop_of, lengths


# ------------------------------------------------------------ strategies


@dataclass
class Strategy:
    name: str
    prediction_safe_claim: bool
    rule: str
    keep: list[int] = field(default_factory=list)
    # closed-form selector for ffmpeg's `select`, proven equal to `keep`
    expr: str = ""


def build_strategies(aus: list[AU], gop_of: list[int], gop_len: int,
                     stride: int, tail_keep: int, gopdrop_every: int) -> list[Strategy]:
    stride_s = Strategy(
        "stride", False,
        f"keep IDR + every {stride}th AU after it (naive; negative control)",
        expr=f"eq(mod(n,{stride}),0)",
    )
    tail_s = Strategy(
        "tail", True,
        f"keep a contiguous prefix of {tail_keep} AUs after each IDR",
        expr=f"lt(mod(n,{gop_len}),{tail_keep})",
    )
    gopdrop_s = Strategy(
        "gopdrop", True,
        f"keep entire GOPs, one in every {gopdrop_every}",
        expr=f"eq(mod(floor(n/{gop_len}),{gopdrop_every}),0)",
    )
    key_s = Strategy(
        "keyonly", True, "keep IDR AUs only (floor reference)",
        expr=f"eq(mod(n,{gop_len}),0)",
    )
    full_s = Strategy("full", True, "no dropping (reference)", expr="")

    for s in (stride_s, tail_s, gopdrop_s, key_s, full_s):
        if s.name == "full":
            s.keep = [au.idx for au in aus]
            continue
        since_key = 0
        for au in aus:
            if au.idr:
                since_key = 0
            else:
                since_key += 1
            if s.name == "stride":
                k = au.idr or (since_key % stride == 0)
            elif s.name == "tail":
                k = au.idr or (since_key < tail_keep)
            elif s.name == "gopdrop":
                k = gop_of[au.idx] % gopdrop_every == 0
            else:  # keyonly
                k = au.idr
            if k:
                s.keep.append(au.idx)
    return [full_s, stride_s, tail_s, gopdrop_s, key_s]


def expr_from_runs(keep: list[int]) -> str:
    """Run-compressed exact selector: between(n,a,b)+between(n,c,d)+...

    Used when the compact closed form does not reproduce the derived set (e.g.
    the encoder produced a non-uniform GOP). Kept frames form few runs, so this
    stays small and it is exact by construction.
    """
    runs: list[tuple[int, int]] = []
    start = prev = keep[0]
    for i in keep[1:]:
        if i == prev + 1:
            prev = i
            continue
        runs.append((start, prev))
        start = prev = i
    runs.append((start, prev))
    return "+".join(
        (f"eq(n,{a})" if a == b else f"between(n,{a},{b})") for a, b in runs
    )


def runs_of(keep: list[int]) -> list[tuple[int, int]]:
    runs: list[tuple[int, int]] = []
    start = prev = keep[0]
    for i in keep[1:]:
        if i == prev + 1:
            prev = i
            continue
        runs.append((start, prev))
        start = prev = i
    runs.append((start, prev))
    return runs


def cadence(keep: list[int], ts_by_au: dict[int, int]) -> dict:
    """Motion-prefix vs freeze durations from the source RTP timestamps (90 kHz)."""
    runs = runs_of(keep)
    motion: list[float] = []
    freeze: list[float] = []
    for i, (a, b) in enumerate(runs):
        motion.append((ts_by_au[b] - ts_by_au[a]) / 90.0)
        if i + 1 < len(runs):
            freeze.append((ts_by_au[runs[i + 1][0]] - ts_by_au[b]) / 90.0)
    def summ(v: list[float]) -> dict:
        if not v:
            return {}
        s = sorted(v)
        return {
            "count": len(v),
            "mean_ms": round(sum(v) / len(v), 1),
            "min_ms": round(s[0], 1),
            "p50_ms": round(s[len(s) // 2], 1),
            "max_ms": round(s[-1], 1),
        }
    return {
        "runs": len(runs),
        "motion_prefix": summ(motion),
        "freeze": summ(freeze),
        "judder_cycles_per_s": round(1000.0 / ((sum(motion) + sum(freeze)) / max(len(runs) - 1, 1)), 2)
        if len(runs) > 1 else 0.0,
    }


def verify_expr(keep: list[int], expr: str) -> bool:
    """Prove the compact ffmpeg selector selects exactly the derived AU set.

    Keeping the ffmpeg filtergraph compact matters (a 3500-term expression is
    unwieldy), but the derived set from the AU index is authoritative -- so the
    two are asserted equal instead of assuming a pattern.
    """
    if not expr:
        return True
    env = {"mod": lambda a, b: a % b, "eq": lambda a, b: a == b,
           "lt": lambda a, b: a < b, "floor": lambda a: int(a),
           "between": lambda a, b, c: b <= a <= c}
    sel = set()
    n_max = max(keep) + 1
    for n in range(n_max + 1):
        scope = dict(env)
        scope["n"] = n
        try:
            if eval(expr, {"__builtins__": {}}, scope):  # noqa: S307 - fixed rule set
                sel.add(n)
        except Exception:
            return False
    return sel == set(keep)


# ------------------------------------------------------------ pipelines


def run(cmd: list[str]) -> subprocess.CompletedProcess:
    return subprocess.run(cmd, capture_output=True, text=True, check=False)


DECODER_ERROR_HINTS = (
    "concealing", "error while decoding", "corrupt", "invalid nal",
    "missing reference", "co located poc", "decode_slice_header",
    "out of range", "reference picture missing", "error in",
)


def decode_count_and_errors(path: str, log_path: str) -> tuple[int, int, list[str]]:
    """Decode the stream alone; return (frames, error_lines, samples)."""
    r = run(["ffmpeg", "-hide_banner", "-nostdin", "-i", path,
             "-f", "null", "-"])
    with open(log_path, "w") as fh:
        fh.write(r.stderr or "")
    frames = 0
    m = re.findall(r"frame=\s*(\d+)", r.stderr or "")
    if m:
        frames = int(m[-1])
    bad: list[str] = []
    for line in (r.stderr or "").splitlines():
        low = line.lower()
        if any(h in low for h in DECODER_ERROR_HINTS):
            bad.append(line.strip())
    return frames, len(bad), bad[:12]


def ssim_vs_reference(variant: str, reference: str, expr: str,
                      stats_path: str, table_path: str) -> tuple[float | None, list[float]]:
    """Per-frame SSIM of `variant` against the reference's matching frames."""
    # Both inputs are re-stamped to PTS = frame index so the ssim framesync pairs
    # kept-frame k of the variant with selected-frame k of the reference.
    # settb on both branches: the ssim framesync warns when the two inputs' time
    # bases differ numerically, even when they are equivalent.
    if expr:
        graph = (f"[0:v]setpts=N,settb=1/90000[a];"
                 f"[1:v]select='{expr}',setpts=N,settb=1/90000[b];"
                 f"[a][b]ssim=stats_file={stats_path}")
    else:
        graph = (f"[0:v]setpts=N,settb=1/90000[a];"
                 f"[1:v]setpts=N,settb=1/90000[b];"
                 f"[a][b]ssim=stats_file={stats_path}")
    r = run(["ffmpeg", "-hide_banner", "-nostdin", "-i", variant, "-i", reference,
             "-filter_complex", graph, "-f", "null", "-"])
    with open(table_path, "w") as fh:
        fh.write(r.stderr or "")
    # ffmpeg prints e.g. "SSIM Y:1.000000 (inf) U:... V:... All:1.000000 (inf)",
    # so anchor on the All: field rather than the whole literal.
    overall = None
    for line in (r.stderr or "").splitlines():
        if "SSIM" not in line:
            continue
        m = re.search(r"All:([0-9.]+)", line)
        if m:
            overall = float(m.group(1))
    per_frame: list[float] = []
    if os.path.exists(stats_path):
        for line in open(stats_path):
            m = re.search(r"All:([\d.\-]+)", line)
            if m:
                per_frame.append(float(m.group(1)))
    return overall, per_frame


# ------------------------------------------------------------ main


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--h264", required=True, help="reference Annex-B capture")
    ap.add_argument("--idx", required=True, help="AU index sidecar for the capture")
    ap.add_argument("--out", required=True, help="output directory")
    ap.add_argument("--stride", type=int, default=4)
    ap.add_argument("--tail-keep", type=int, default=15)
    ap.add_argument("--gopdrop-every", type=int, default=4)
    ap.add_argument("--skip-ssim", action="store_true")
    args = ap.parse_args()

    os.makedirs(args.out, exist_ok=True)
    aus = load_aus(args.idx)
    if not aus:
        print("no AUs in index", file=sys.stderr)
        return 1

    blob = open(args.h264, "rb").read()
    total = sum(a.size for a in aus)
    if total != len(blob):
        print(f"index/file size mismatch: {total} vs {len(blob)}", file=sys.stderr)
        return 1

    gop_of, gop_lengths = gop_layout(aus)
    uniform = len(set(gop_lengths)) == 1
    gop_len = gop_lengths[0]
    duration_s = (aus[-1].ts - aus[0].ts) / 90000.0
    src_fps = len(aus) / duration_s if duration_s else 0.0

    print(f"reference: {len(aus)} AUs, {duration_s:.2f}s, {src_fps:.2f} fps, "
          f"{len(gop_lengths)} GOPs, gop_len={gop_len} (uniform={uniform})")

    strategies = build_strategies(aus, gop_of, gop_len,
                                 args.stride, args.tail_keep, args.gopdrop_every)

    summary: dict = {
        "reference": {
            "aus": len(aus), "bytes": len(blob), "duration_s": round(duration_s, 3),
            "fps": round(src_fps, 3), "gops": len(gop_lengths), "gop_len": gop_len,
            "gop_len_uniform": uniform,
            "keyframes": sum(1 for a in aus if a.idr),
            "kbps_payload": round(len(blob) * 8 / duration_s / 1000, 1),
        },
        "strategies": {},
    }

    # byte offsets for slicing AUs out of the reference stream
    offs: list[int] = []
    pos = 0
    for a in aus:
        offs.append(pos)
        pos += a.size

    ts_by_au = {a.idx: a.ts for a in aus}

    for s in strategies:
        # Prefer the compact closed form; fall back to the exact run-compressed
        # selector when the encoder produced a non-uniform GOP structure.
        if not verify_expr(s.keep, s.expr):
            alt = expr_from_runs(s.keep) if s.expr else ""
            if alt and verify_expr(s.keep, alt):
                print(f"  note: {s.name}: closed form {s.expr!r} did not match; "
                      f"using run-compressed selector ({alt.count('+') + 1} runs)")
                s.expr = alt
            else:
                print(f"FATAL: no selector reproduces the derived set for {s.name}",
                      file=sys.stderr)
                return 1

        out_h264 = os.path.join(args.out, f"{s.name}.h264")
        out_map = os.path.join(args.out, f"{s.name}.map")
        with open(out_h264, "wb") as fh:
            for i in s.keep:
                fh.write(blob[offs[i]:offs[i] + aus[i].size])
        with open(out_map, "w") as fh:
            for i in s.keep:
                fh.write(f"{i}\n")

        kept = len(s.keep)
        kept_bytes = sum(aus[i].size for i in s.keep)
        dropped = len(aus) - kept
        dropped_bytes = len(blob) - kept_bytes
        # delivered rate uses the same wall-clock window as the source
        delivered_fps = kept / duration_s if duration_s else 0.0

        rec: dict = {
            "rule": s.rule,
            "prediction_safe_claim": s.prediction_safe_claim,
            "aus_kept": kept,
            "aus_dropped": dropped,
            "kept_bytes": kept_bytes,
            "dropped_bytes": dropped_bytes,
            "delivered_fps": round(delivered_fps, 3),
            "delivered_kbps": round(kept_bytes * 8 / duration_s / 1000, 1) if duration_s else 0,
            "h264": out_h264,
            "selector": s.expr,
            "cadence": cadence(s.keep, ts_by_au),
        }

        if not args.skip_ssim:
            fr, errs, samples = decode_count_and_errors(
                out_h264, os.path.join(args.out, f"{s.name}.decode.log"))
            rec["decoded_frames"] = fr
            rec["decoder_error_lines"] = errs
            rec["decoder_error_samples"] = samples
            rec["decode_matches_kept"] = (fr == kept)
            if s.name != "full":
                overall, per_frame = ssim_vs_reference(
                    out_h264, args.h264, s.expr,
                    os.path.join(args.out, f"{s.name}.ssim.stats"),
                    os.path.join(args.out, f"{s.name}.ssim.log"))
                rec["ssim_overall"] = overall
                rec["ssim_frames"] = len(per_frame)
                if per_frame:
                    perfect = sum(1 for v in per_frame if v >= 0.999999)
                    rec["ssim_frames_perfect"] = perfect
                    rec["ssim_frames_imperfect"] = len(per_frame) - perfect
                    rec["ssim_min"] = min(per_frame)
                    imperfect = sorted(
                        ((i, v) for i, v in enumerate(per_frame) if v < 0.999999),
                        key=lambda t: t[1])[:10]
                    rec["ssim_worst"] = [
                        {"kept_au": i, "src_au": s.keep[i], "ssim": v}
                        for i, v in imperfect]

        summary["strategies"][s.name] = rec
        cd_ = rec["cadence"]
        mo = cd_.get("motion_prefix", {}).get("p50_ms")
        fz = cd_.get("freeze", {}).get("p50_ms")
        print(f"  {s.name:9s} kept={kept:5d} dropped={dropped:5d} "
              f"{delivered_fps:7.2f} fps {rec['delivered_kbps']:8.1f} kbps "
              f"ssim={rec.get('ssim_overall')} "
              f"motion_p50={mo}ms freeze_p50={fz}ms runs={cd_.get('runs')}")

    with open(os.path.join(args.out, "throttle-compare.json"), "w") as fh:
        json.dump(summary, fh, indent=1)
    print(f"\nwrote {os.path.join(args.out, 'throttle-compare.json')}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
