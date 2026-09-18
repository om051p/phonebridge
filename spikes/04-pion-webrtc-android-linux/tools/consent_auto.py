#!/usr/bin/env python3
"""Deterministic MediaProjection consent automation for Spike 04.

The SystemUI consent dialog is a bottom sheet that *animates in*, and
`uiautomator dump` refuses (or returns stale output) unless the UI is idle.
Scraping coordinates from a mid-animation dump is what made the previous bash
automation flaky: the tap either landed on the wrong widget or was ignored, and
the run then failed with `resultCode != -1` (consent_denied) while the driver
logged "scope option not offered".

This script replaces that with a small state machine that:

  * only acts on SETTLED dumps -- it requires two consecutive byte-identical
    reads before deciding anything;
  * READS the current capture scope from the spinner's child node and verifies
    the value actually changed before pressing Start (never assumes a tap
    worked);
  * retries scope selection, with a keyboard fallback (Spinner + DPAD) for when
    the popup does not materialise in the dump;
  * never taps Cancel.

Exits 0 once the requested scope is confirmed selected and Start has been
pressed, 1 on failure. Progress goes to stdout for the driver to capture.
"""

from __future__ import annotations

import argparse
import os
import re
import subprocess
import sys
import time
import xml.etree.ElementTree as ET

DUMP_PATH = "/sdcard/spike04-ui.xml"
SPINNER_ID = "screen_share_mode_spinner"
BOUNDS = re.compile(r"\[(\d+),(\d+)\]\[(\d+),(\d+)\]")


def log(msg: str) -> None:
    print(f"[consent] {msg}", flush=True)


# Target device. With more than one device/emulator attached (an emulator often
# is), every adb call without -s fails with "more than one device/emulator",
# which looks exactly like "the dialog never appeared".
SERIAL = ""


def adb(*args: str, binary: bool = False):
    base = ("adb", "-s", SERIAL) if SERIAL else ("adb",)
    # stdin MUST be detached: adb forwards its stdin to the device shell, so an
    # inherited stdin drains any heredoc the caller script is holding. That
    # silently emptied the driver's probe scripts (the screen-off scenario ran
    # its sleeps but none of its adb commands).
    return subprocess.run(
        (*base, *args), capture_output=True, text=not binary, check=False,
        stdin=subprocess.DEVNULL,
    )


def dump_once() -> bytes | None:
    """One uiautomator dump. Returns hierarchy bytes, or None if not idle."""
    r = adb("shell", "uiautomator", "dump", "--compressed", DUMP_PATH)
    out = (r.stdout or "") if isinstance(r.stdout, str) else ""
    if "ERROR" in out or "could not get idle state" in out:
        return None
    g = adb("exec-out", "cat", DUMP_PATH, binary=True)
    if g.returncode != 0 or b"<hierarchy" not in (g.stdout or b""):
        return None
    return g.stdout


def settled_dump(tries: int = 20, delay: float = 0.4,
                 stable: int = 3) -> bytes | None:
    """Dump until `stable` consecutive reads are identical.

    Two identical reads are not enough: the consent sheet re-lays-out when the
    capture scope changes (the warning paragraph differs per scope), and a
    mid-animation pair can look stable. Tapping Start from that stale geometry
    lands outside the button -- which dismisses the dialog and yields
    resultCode=0 (reported as consent_denied).
    """
    prev: bytes | None = None
    same = 0
    for _ in range(tries):
        cur = dump_once()
        if cur is not None:
            same = same + 1 if cur == prev else 1
            prev = cur
            if same >= stable:
                return cur
        time.sleep(delay)
    return prev


def center(node: ET.Element) -> tuple[int, int] | None:
    m = BOUNDS.match(node.get("bounds") or "")
    if not m:
        return None
    x1, y1, x2, y2 = (int(g) for g in m.groups())
    if x2 - x1 <= 0 or y2 - y1 <= 0:
        return None
    return ((x1 + x2) // 2, (y1 + y2) // 2)


def tap(x: int, y: int) -> None:
    adb("shell", "input", "tap", str(x), str(y))


def key(code: str) -> None:
    adb("shell", "input", "keyevent", code)


def spinner(root: ET.Element) -> ET.Element | None:
    for n in root.iter("node"):
        if SPINNER_ID in (n.get("resource-id") or ""):
            return n
    return None


def current_scope(root: ET.Element) -> str | None:
    """The spinner's displayed value lives in a child TextView."""
    sp = spinner(root)
    if sp is None:
        return None
    kids = list(sp.iter("node"))[1:]
    for k in kids:
        t = (k.get("text") or "").strip()
        if t:
            return t
    return None


def find_text(root: ET.Element, want: str) -> ET.Element | None:
    """A node whose text equals `want` (case-insensitive)."""
    for n in root.iter("node"):
        if (n.get("text") or "").strip().lower() == want.strip().lower():
            return n
    return None


def start_button(root: ET.Element) -> ET.Element | None:
    for n in root.iter("node"):
        if (n.get("resource-id") or "").endswith("android:id/button1"):
            return n
    for n in root.iter("node"):
        t = (n.get("text") or "").strip().lower()
        if t in ("start", "start now", "allow", "share", "cast"):
            return n
    return None


def dialog_present(root: ET.Element) -> bool:
    for n in root.iter("node"):
        rid = n.get("resource-id") or ""
        if "screen_share_dialog_title" in rid or SPINNER_ID in rid:
            return True
        t = (n.get("text") or "").strip().lower()
        if "start recording or casting" in t:
            return True
    return False


def select_scope(want: str, attempts: int) -> bool:
    """Drive the Scope spinner until it displays `want`."""
    for attempt in range(1, attempts + 1):
        root = ET.fromstring(settled_dump() or b"<hierarchy/>")
        if not dialog_present(root):
            log(f"attempt {attempt}: dialog gone")
            return False

        cur = current_scope(root)
        log(f"attempt {attempt}: current scope = {cur!r} (want {want!r})")
        if cur and cur.lower() == want.lower():
            return True

        sp = spinner(root)
        if sp is None:
            log("attempt: no scope spinner in this dialog")
            return True  # nothing to select; caller may just press Start

        # 1) tap the spinner and look for the popup option in the dump.
        c = center(sp)
        if c:
            tap(*c)
        time.sleep(0.6)
        root2 = ET.fromstring(settled_dump() or b"<hierarchy/>")
        opt = find_text(root2, want)
        if opt is not None and center(opt) is not None:
            tap(*center(opt))
            # The sheet re-lays-out after the scope changes (different warning
            # text), so wait it out before reading the Start button geometry.
            time.sleep(1.5)
            root3 = ET.fromstring(settled_dump() or b"<hierarchy/>")
            if (current_scope(root3) or "").lower() == want.lower():
                log(f"selected {want!r} via popup tap")
                return True
            log("popup tap did not change the scope; trying keyboard")
            root = root3
            sp = spinner(root)
            if sp is None:
                continue

        # 2) keyboard fallback: focus the Spinner and walk it with DPAD.
        c = center(sp) if sp is not None else None
        if c:
            tap(*c)
            time.sleep(0.4)
            # two options exist; step until the value changes or we give up
            for _ in range(4):
                key("KEYCODE_DPAD_DOWN")
                time.sleep(0.3)
                key("KEYCODE_DPAD_CENTER")
                time.sleep(0.5)
                root4 = ET.fromstring(settled_dump() or b"<hierarchy/>")
                if (current_scope(root4) or "").lower() == want.lower():
                    log(f"selected {want!r} via keyboard")
                    return True
            time.sleep(0.5)

    root = ET.fromstring(settled_dump() or b"<hierarchy/>")
    return (current_scope(root) or "").lower() == want.lower()


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--scope", default="Entire screen")
    ap.add_argument("--attempts", type=int, default=6)
    ap.add_argument("--serial", default="",
                    help="adb target serial (else $ANDROID_SERIAL / $SERIAL)")
    ap.add_argument("--timeout", type=float, default=60.0,
                    help="seconds to wait for the dialog to appear")
    args = ap.parse_args()

    global SERIAL
    SERIAL = (args.serial or os.environ.get("ANDROID_SERIAL")
              or os.environ.get("SERIAL") or "")
    # Auto-pick the only attached device, or the single physical one.
    if not SERIAL:
        d = adb("devices").stdout or ""
        ids = [ln.split()[0] for ln in d.splitlines()
               if len(ln.split()) > 1 and ln.split()[1] == "device"]
        physical = [i for i in ids if "emulator-" not in i and ":" not in i]
        if len(ids) == 1:
            SERIAL = ids[0]
        elif len(physical) == 1:
            SERIAL = physical[0]
    if SERIAL:
        log(f"adb target: {SERIAL}")
    else:
        log("WARN: could not determine a unique adb target")

    deadline = time.time() + args.timeout
    root = None
    while time.time() < deadline:
        root = ET.fromstring(settled_dump(tries=6) or b"<hierarchy/>")
        if dialog_present(root):
            break
        time.sleep(0.5)

    if root is None or not dialog_present(root):
        log("consent dialog never appeared")
        return 1

    cur = current_scope(root)
    log(f"dialog up; scope={cur!r}")

    if args.scope and spinner(root) is not None:
        if not select_scope(args.scope, args.attempts):
            log(f"FAILED to select scope {args.scope!r}; refusing to press Start")
            return 1
        root = ET.fromstring(settled_dump() or b"<hierarchy/>")

    cur = current_scope(root)
    if cur is not None and args.scope and cur.lower() != args.scope.lower():
        log(f"scope is {cur!r}, expected {args.scope!r}; not pressing Start")
        return 1

    # Re-read from a freshly settled dump: the sheet has just re-laid-out.
    time.sleep(1.5)
    root = ET.fromstring(settled_dump(stable=4) or b"<hierarchy/>")
    dump_dir = os.environ.get("CONSENT_DUMP_DIR")
    if dump_dir:
        try:
            os.makedirs(dump_dir, exist_ok=True)
            with open(os.path.join(dump_dir, "consent-final.xml"), "wb") as fh:
                fh.write(ET.tostring(root))
        except Exception:
            pass

    for press in range(1, 4):
        btn = start_button(root)
        if btn is None:
            log("no Start button found")
            return 1
        c = center(btn)
        if c is None:
            log("Start button has no usable bounds")
            return 1
        scope_now = current_scope(root)
        log(f"pressing Start (#{press}) at {c} (scope={scope_now!r})")
        tap(*c)
        time.sleep(1.5)
        root = ET.fromstring(settled_dump(stable=2) or b"<hierarchy/>")
        if not dialog_present(root):
            return 0
        # Still up: either the tap missed or a follow-up dialog appeared.
        log(f"dialog still present after press #{press}; re-reading geometry")
        if args.scope and spinner(root) is not None:
            sel_scope = current_scope(root)
            if (sel_scope or "").lower() != args.scope.lower():
                log(f"scope reverted to {sel_scope!r}; reselecting")
                select_scope(args.scope, args.attempts)
                root = ET.fromstring(settled_dump(stable=4) or b"<hierarchy/>")


if __name__ == "__main__":
    raise SystemExit(main())
