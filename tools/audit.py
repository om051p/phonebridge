#!/usr/bin/env python3
"""PhoneBridge connection audit runner.

Snapshots both sides of a session and drives the connection lifecycle so the
audit can assert two-sided agreement.

Linux side  : phonebridge-daemon via core/ipcdrv (session state).
Phone side  : the app's own UI, read from Flutter semantics over adb
              (Home connection card = authoritative peer; app-bar badge).

Usage:
  audit.py snapshot <serial>
  audit.py cycles <serial> <n>          # n Linux-initiated connect/disconnect pairs
  audit.py switch <serialA> <serialB> <n>   # n A<->B switches (Linux-initiated)
  audit.py full <serialA> <serialB>
"""
import os
import re
import subprocess
import sys
import time

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
CORE = os.path.join(ROOT, "core")
ADB = "/opt/android-sdk/platform-tools/adb"
IPCDRV = os.path.join(CORE, "ipcdrv")

DEV = {
    "pixel": "4326bfa04d0129c967f6a56db186507cf1e7fcd18205a1140b4234ead51ce095",
    "poco": "eb6781fb0e0ea44db148abff665c4cabb1505dea543cb46429fffaa1c466e979",
    "pixel_serial": "99061FFBA0008M",
    "poco_serial": "eff77dc",
}


def ipc(*args, timeout=60):
    env = dict(os.environ, XDG_RUNTIME_DIR=os.environ.get("XDG_RUNTIME_DIR", "/run/user/1000"))
    r = subprocess.run([IPCDRV] + list(args), cwd=CORE, capture_output=True,
                       text=True, env=env, timeout=timeout)
    return r.stdout + r.stderr


def linux_state():
    out = ipc("session")
    m = re.search(r'state:\s+(\S+)', out)
    d = re.search(r'device_id:\s+"([0-9a-f]+)"', out)
    return (m.group(1) if m else "?", d.group(1) if d else "")


def ui(serial, *args):
    r = subprocess.run(["python3", os.path.join(ROOT, "tools", "android_ui.py"),
                        args[0], serial] + list(args[1:]),
                       capture_output=True, text=True)
    return r.stdout


def phone_home(serial):
    subprocess.run([ADB, "-s", serial, "shell", "am", "start",
                    "-n", "dev.phonebridge/.ui.MainActivity"],
                   capture_output=True)
    subprocess.run([ADB, "-s", serial, "shell", "am", "broadcast",
                    "-a", "dev.phonebridge.NAVIGATE", "--ei", "tab", "0"],
                   capture_output=True)
    time.sleep(0.8)
    dump = ui(serial, "dump")
    # The connection card can sit below the fold (e.g. a permissions setup card
    # at the top of Home), so scroll it into view before classifying.
    if "No active connection" not in dump and "STOP SHARING" not in dump \
            and "streaming to your connected device" not in dump:
        subprocess.run([ADB, "-s", serial, "shell", "input", "swipe",
                        "540", "1600", "540", "600", "400"], capture_output=True)
        time.sleep(0.8)
        dump = ui(serial, "dump")
    # Home connection card title is the authoritative peer when a session is
    # live, and the literal "No active connection" when there is none.
    if "No active connection" in dump:
        return "IDLE"
    if "streaming to your connected device" in dump or "STOP SHARING" in dump:
        return "ACTIVE"
    if "\\nlinux · Connected" in dump or "\nlinux · Connected" in dump or "· Connected" in dump:
        return "ACTIVE"
    badge = phone_badge(serial)
    if "Connected" in badge or "Sharing" in badge:
        return "ACTIVE"
    if "Ready" in badge:
        return "IDLE"
    return "UNKNOWN"


def phone_badge(serial):
    return ui(serial, "badge").strip()


def start(device_key):
    return ipc("start", DEV[device_key])


def stop():
    return ipc("stop")


def _agree(ls, ph):
    linux_up = ls in ("SESSION_STATE_CONNECTED", "SESSION_STATE_STREAMING",
                      "SESSION_STATE_CONNECTING", "SESSION_STATE_RECONNECTING")
    phone_up = ph == "ACTIVE"
    return linux_up == phone_up


def cycles(serial, n, which="pixel"):
    fails = 0
    for i in range(1, n + 1):
        start(which)
        time.sleep(4)
        ls, ld = linux_state()
        ph = phone_home(serial)
        badge = phone_badge(serial)
        agree = _agree(ls, ph)
        print(f"cycle {i} connect  : linux={ls:32s} phone_home={ph:6s} badge={badge!r:28s} agree={'YES' if agree else 'NO'}")
        if not agree:
            fails += 1
        stop()
        time.sleep(3)
        ls2, _ = linux_state()
        ph2 = phone_home(serial)
        agree2 = _agree(ls2, ph2)
        print(f"cycle {i} disconnect: linux={ls2:32s} phone_home={ph2:6s} agree={'YES' if agree2 else 'NO'}")
        if not agree2:
            fails += 1
    print(f"RESULT: {2*n - fails}/{2*n} two-sided agreements")
    return fails


def switch(serialA, serialB, n):
    whichA = "pixel" if serialA == DEV["pixel_serial"] else "poco"
    whichB = "pixel" if serialB == DEV["pixel_serial"] else "poco"
    devA = DEV[whichA]
    devB = DEV[whichB]
    fails = 0
    stop()
    time.sleep(2)
    for i in range(1, n + 1):
        start(whichA)
        time.sleep(4)
        lsA, ldA = linux_state()
        phA = phone_home(serialA)
        phB = phone_home(serialB)
        agreeA = _agree(lsA, phA) and (phB == "IDLE") and (ldA == devA)
        print(f"switch {i} -> {whichA:5s}: linux={lsA:26s} peer={ldA[:12]} phA={phA:6s} phB={phB:6s} agree={'YES' if agreeA else 'NO'}")
        if not agreeA:
            fails += 1

        stop()
        time.sleep(2)
        start(whichB)
        time.sleep(4)
        lsB, ldB = linux_state()
        phA2 = phone_home(serialA)
        phB2 = phone_home(serialB)
        agreeB = _agree(lsB, phB2) and (phA2 == "IDLE") and (ldB == devB)
        print(f"switch {i} -> {whichB:5s}: linux={lsB:26s} peer={ldB[:12]} phA={phA2:6s} phB={phB2:6s} agree={'YES' if agreeB else 'NO'}")
        if not agreeB:
            fails += 1
        stop()
        time.sleep(2)

    stop()
    time.sleep(2)
    print(f"RESULT: {2*n - fails}/{2*n} switch invariants held")
    return fails


def main():
    if len(sys.argv) < 2:
        print(__doc__, file=sys.stderr)
        return 2
    op = sys.argv[1]
    if op == "snapshot":
        serial = sys.argv[2]
        ls, ld = linux_state()
        print("linux:", ls, ld[:16])
        print("phone_home:", phone_home(serial))
        print("phone_badge:", phone_badge(serial))
        return 0
    if op == "cycles":
        serial, n = sys.argv[2], int(sys.argv[3])
        which = "pixel" if serial == DEV["pixel_serial"] else "poco"
        return 1 if cycles(serial, n, which) else 0
    if op == "switch":
        serialA, serialB, n = sys.argv[2], sys.argv[3], int(sys.argv[4])
        return 1 if switch(serialA, serialB, n) else 0
    if op == "full":
        serialA, serialB = sys.argv[2], sys.argv[3]
        fA = cycles(serialA, 3, "pixel" if serialA == DEV["pixel_serial"] else "poco")
        fB = cycles(serialB, 3, "pixel" if serialB == DEV["pixel_serial"] else "poco")
        fS = switch(serialA, serialB, 3)
        total_fails = fA + fB + fS
        print(f"\nFULL AUDIT RESULT: {'PASS' if total_fails == 0 else 'FAIL'} (failures={total_fails})")
        return 1 if total_fails else 0
    if op == "start":
        print(start(sys.argv[2]))
        return 0
    if op == "stop":
        print(stop())
        return 0
    print("unknown op", op, file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main())
