# Spike 07 — P4 Result: ffmpeg runtime dependency & graceful degradation

## Result (summary)

| Question | Answer |
|---|---|
| New packages required? | **No.** The renderer tap needs only `ffmpeg` (decode H.264 → MJPEG stdout); the existing display path already needs `ffplay`. Both binaries ship in the **same distro package** (`ffmpeg`: `/usr/bin/ffmpeg` + `/usr/bin/ffplay`, confirmed via `dpkg -S` on the test host, ffmpeg 8.0.1). |
| Missing-ffmpeg behavior | **Typed errors, no crash, no session impact.** All three ffmpeg-family constructors return `exec.LookPath` errors (probe: `go/missingffmpeg`, output below); the session manager's sink-selection chain converts a failed sink into `NullSink` — the session starts normally and the snapshot reports `SINK_KIND_NULL`, which the Screen already renders as a real state. |
| Declared anywhere today? | **No.** `linux/packaging/` contains exactly one file (the systemd user unit). `.deb`/`Flatpak` manifests do not exist yet (Phase 8), so there is currently no dependency declaration to update — only to *create correctly later*. |
| Version floor | Tap uses `-probesize`, `-analyzeduration`, `-f mjpeg`, `-q:v` — available since ffmpeg 2.x/3.x; validated on 8.0.1. Recommend declaring **ffmpeg ≥ 4.4** (first long-term-supported baseline still in current distros). No hardware decode, filters, or version-specific flags. |

## Empirical probe (`go/missingffmpeg`, PATH stripped to /nonexistent)

```
NewDisplaySink (ffplay) err: receiver: binary "ffplay" not found in PATH: exec: "ffplay": executable file not found in $PATH
NewPipeSink (ffmpeg) err: receiver: binary "ffmpeg" not found in PATH: exec: "ffmpeg": executable file not found in $PATH
NewFFmpegVerifySink (ffmpeg) err: receiver: ffmpeg not found in PATH: exec: "ffmpeg": executable file not found in $PATH
NullSink usable without ffmpeg: true
```

## Graceful degradation chain (code inspection, `core/pkg/engine/manager.go` `StartSession`)

```text
sinkFactory() err        → sink stays nil        (frames-tap hook: SetSinkFactory, currently unused by daemon)
static sink              → nil in production
DISPLAY/WAYLAND set?     → NewDisplaySink() err → NOT assigned (sink stays nil)
sink still nil           → receiver.NewNullSink() + SinkKindNull   ← session starts anyway
```

- The session is **never** failed by a missing ffmpeg/ffplay; only the render
  path degrades, and `GetSessionState.sink_kind = SINK_KIND_NULL` +
  `VideoDisplayBanner` already surface that honestly (Phase 6 Slice 1).
- For the future frames path the same pattern applies (factory error →
  fallback), with one UI refinement to implement in production: today the NULL
  banner copy says "no graphical session"; a host that *has* a display but
  *lacks ffmpeg* should be distinguishable (recommend a presence probe
  `exec.LookPath("ffmpeg")` reported alongside `sink_kind`, e.g.
  `frames_available=false, frames_reason=FFMPEG_MISSING`). Schema/UX detail for
  the production slice — not implemented here.

## Packaging guidance (for Phase 8 — nothing exists to modify today)

| Artifact | Recommendation |
|---|---|
| `.deb` | `Recommends: ffmpeg` (NOT `Depends:` — the daemon must remain installable and runnable headless; in-app mirroring and ffplay display degrade gracefully, clipboard/transfer/notifications unaffected) |
| systemd user unit | Keep as-is: only `XDG_RUNTIME_DIR` is set, so the unit inherits the user manager's PATH, which contains `/usr/bin`. Do **not** hardcode an PATH that could hide ffmpeg. |
| Flatpak | **Open risk:** the freedesktop runtime does not ship the ffmpeg CLI, and a sandboxed app cannot exec host binaries. Options: bundle ffmpeg into the Flatpak, use the ffmpeg extension point, or declare in-app mirroring unsupported under Flatpak (native `.deb` remains primary per DEC-023 precedent). Manifest does not exist yet — decide at Phase 8. |
| Logging | Never log frame bytes (Zero-Logging Rule precedent); log only ffmpeg presence, pid, and AU/JPEG counters. |

## Not changed

No production file was modified for P4. The probe lives entirely under
`spikes/07-frame-renderer/go/missingffmpeg/`.
