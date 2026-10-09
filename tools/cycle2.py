#!/usr/bin/env python3
"""Phone-initiated connect/disconnect cycle runner.

The user-facing "connect" is phone-initiated: the phone selects the trusted
Linux peer on the Devices tab, Android shows its MediaProjection consent, then
capture starts and the daemon sees the session. This drives that flow over adb
(semantics taps) and asserts the two sides agree at every step.

Usage:
  cycle2.py <serial> <cycles> [--disconnect phone|linux]
"""
import os
import re
import subprocess
import sys
import time

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
CORE = os.path.join(ROOT, "core")
ADB = "/opt/android-sdk/platform-tools/adb"

sys.path.insert(0, os.path.join(ROOT, "tools"))
import android_ui as A  # noqa: E402


def ipc(*args):
    env = dict(os.environ, XDG_RUNTIME_DIR="/run/user/1000")
    r = subprocess.run([os.path.join(CORE, "ipcdrv")] + list(args),
                       cwd=CORE, capture_output=True, text=True, env=env)
    return r.stdout + r.stderr


LINUX_UP = {
    "SESSION_STATE_CONNECTED",
    "SESSION_STATE_STREAMING",
    "SESSION_STATE_RECONNECTING",
}


def linux_state():
    out = ipc("session")
    state = ""
    for line in out.splitlines():
        if line.strip().startswith("state:"):
            state = line.split(":", 1)[1].strip()
    return state


def tap_exact(serial, label):
    for n in A.nodes(serial):
        if (n["desc"] or n["text"]).strip() == label and n["cx"] is not None:
            A.adb(serial, "shell", "input", "tap", str(n["cx"]), str(n["cy"]))
            return True
    return False


def _bounds(b):
    m = re.match(r"\[(\d+),(\d+)\]\[(\d+),(\d+)\]", b or "")
    return tuple(map(int, m.groups())) if m else None


def tap_connect_for(serial, peer="x1"):
    """Tap CONNECT/SWITCH in the specific peer's row (a phone can trust two peers)."""
    ns = A.nodes(serial)
    row = None
    for n in ns:
        if peer.lower() in (n["desc"] or n["text"]).lower():
            row = _bounds(n["bounds"])
            if row:
                break
    btns = [n for n in ns
            if (n["desc"] or n["text"]).strip() in
            ("CONNECT", "SWITCH TO THIS DEVICE") and n["cx"] is not None]
    if not btns:
        return False
    chosen = btns[0]
    if row:
        y1, y2 = row[1], row[3]
        inside = [b for b in btns if y1 - 40 <= b["cy"] <= y2 + 40]
        if inside:
            chosen = inside[0]
    A.adb(serial, "shell", "input", "tap", str(chosen["cx"]), str(chosen["cy"]))
    return True


def phone_badge(serial):
    return subprocess.run(
        ["python3", os.path.join(ROOT, "tools", "android_ui.py"), "badge", serial],
        capture_output=True, text=True).stdout.strip()


def launch(serial):
    # Explicit component: `monkey -c LAUNCHER` can resolve to a leftover spike
    # package when several dev.phonebridge.* apps are installed.
    A.adb(serial, "shell", "am", "start", "-n",
          "dev.phonebridge/.ui.MainActivity")
    time.sleep(2.0)


def nav(serial, tab):
    A.adb(serial, "shell", "am", "broadcast", "-a", "dev.phonebridge.NAVIGATE",
          "--ei", "tab", str(tab))
    time.sleep(1.2)


def phone_connect(serial, peer="x1", wait=20):
    """Tap CONNECT/SWITCH for the trusted Linux peer and accept capture consent."""
    launch(serial)
    nav(serial, 1)
    tap_connect_for(serial, peer)
    # MediaProjection consent appears asynchronously: Android 13 buttons are
    # "Start now", Android 15 "Start". Poll instead of guessing a delay.
    deadline = time.time() + wait
    while time.time() < deadline:
        time.sleep(1.0)
        labels = [(n["desc"] or n["text"]).strip() for n in A.nodes(serial)]
        if tap_exact(serial, "Start now") or tap_exact(serial, "Start"):
            break
        if phone_badge(serial).endswith("Sharing"):
            break
    time.sleep(5)


def phone_disconnect(serial):
    nav(serial, 1)
    return tap_exact(serial, "DISCONNECT")


def main():
    if len(sys.argv) < 3:
        print(__doc__, file=sys.stderr)
        return 2
    serial = sys.argv[1]
    n = int(sys.argv[2])
    mode = "linux"
    if "--disconnect" in sys.argv:
        mode = sys.argv[sys.argv.index("--disconnect") + 1]

    fails = 0
    for i in range(1, n + 1):
        phone_connect(serial)
        ls, ph = linux_state(), phone_badge(serial)
        up = ph.endswith("Sharing")
        agree = (ls in LINUX_UP) == up
        print(f"cycle {i} connect   : linux={ls:28s} phone={ph!r:26s} agree={'YES' if agree else 'NO'}")
        if not agree:
            fails += 1

        if mode == "phone":
            phone_disconnect(serial)
        else:
            ipc("stop")
        time.sleep(4)
        ls2, ph2 = linux_state(), phone_badge(serial)
        up2 = ph2.endswith("Sharing")
        agree2 = (ls2 in LINUX_UP) == up2
        print(f"cycle {i} disconnect: linux={ls2:28s} phone={ph2!r:26s} agree={'YES' if agree2 else 'NO'}")
        if not agree2:
            fails += 1
    total = 2 * n
    print(f"RESULT: {total - fails}/{total} two-sided agreements (disconnect via {mode})")
    return 1 if fails else 0


if __name__ == "__main__":
    sys.exit(main())
