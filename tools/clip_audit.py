#!/usr/bin/env python3
"""Bidirectional clipboard audit for PhoneBridge (Android <-> Linux).

Linux side  : tools/clip.py (mutter helper stdio protocol).
Android side: dev.phonebridge.spike05setter (adb-driven, results on logcat).

Verifies exact byte-for-byte delivery both directions using a Java hashCode
digest (the Android read logs `echo=`), plus length.

Usage:
  clip_audit.py pc2phone <serial> [label]
  clip_audit.py phone2pc <serial> [label]
  clip_audit.py matrix <serial>            # full bidirectional matrix
"""
import re
import shlex
import subprocess
import sys
import time

ADB = "/opt/android-sdk/platform-tools/adb"
CLIP = "tools/clip.py"
SETTER = "dev.phonebridge.spike05setter/.SetterActivity"


def sh(cmd, **kw):
    return subprocess.run(cmd, capture_output=True, text=True, **kw)


def java_hash(s: str) -> str:
    h = 0
    for ch in s:
        h = (31 * h + ord(ch)) & 0xFFFFFFFF
    return format(h, "x")


def linux_set(text: str):
    data = text.encode()
    p = subprocess.run(["python3", CLIP, "writefile", "/dev/stdin"],
                       input=data, capture_output=True)
    return p.returncode == 0, p.stderr.decode()


def linux_get() -> bytes:
    p = subprocess.run(["python3", CLIP, "read"], capture_output=True)
    return p.stdout


def _device_shell(serial, cmd):
    """Run one device-shell command string (so shlex.quote works on-device)."""
    return sh([ADB, "-s", serial, "shell", cmd])


def android_set(serial, text):
    # The text is quoted for the DEVICE shell (adb re-parses), so newlines and
    # symbols survive exactly.
    cmd = (f"am start -W -n {SETTER} --es op write --es label audit "
           f"--es text {shlex.quote(text)} --ez auto_finish true")
    r = _device_shell(serial, cmd)
    return r.returncode == 0


def android_get(serial, label="auditread", retries=3):
    for attempt in range(retries):
        sh([ADB, "-s", serial, "logcat", "-c"])
        _device_shell(serial, f"am start -W -n {SETTER} --es op read "
                              f"--es label {label} --ez auto_finish true")
        time.sleep(1.2)
        out = sh([ADB, "-s", serial, "logcat", "-d", "-s", "Spike05Setter"]).stdout
        line = ""
        for ln in out.splitlines():
            if f"op=read label={label}" in ln:
                line = ln
        if not line:
            continue
        m_echo = re.search(r"echo=([0-9a-f]+)", line)
        m_bytes = re.search(r"bytes=(\d+)", line)
        m_text = re.search(r'text=(.*)$', line)
        if m_bytes and m_echo:
            return {
                "echo": m_echo.group(1),
                "bytes": int(m_bytes.group(1)),
                "preview": m_text.group(1) if m_text else "",
                "raw": line,
            }
    return None


def wait_for_android(serial, expect_bytes, timeout=6.0):
    """Poll the Android clipboard until it holds expect_bytes bytes."""
    deadline = time.time() + timeout
    last = None
    while time.time() < deadline:
        got = android_get(serial, "poll", retries=1)
        last = got
        if got and got["bytes"] == expect_bytes:
            return got
        time.sleep(0.4)
    return last


def check_pc2phone(serial, text, label):
    if not linux_set(text):
        return False, "linux set failed"
    exp = len(text.encode())
    got = wait_for_android(serial, exp)
    if not got:
        return False, "android read returned nothing"
    if got["bytes"] != exp:
        return False, f"bytes {got['bytes']} != {exp} ({got['raw'][:120]})"
    if got["echo"] != java_hash(text):
        return False, f"digest mismatch echo={got['echo']} want={java_hash(text)}"
    return True, f"{got['bytes']}B ok"


def check_phone2pc(serial, text, label):
    before = linux_get()
    if not android_set(serial, text):
        return False, "android set failed"
    exp = text.encode()
    deadline = time.time() + 6
    while time.time() < deadline:
        cur = linux_get()
        if cur == exp:
            return True, f"{len(exp)}B ok"
        time.sleep(0.4)
    return False, f"linux clipboard {len(linux_get())}B, want {len(exp)}B (before={len(before)}B)"


CASES = [
    ("short", "PB-short-42"),
    ("multiline", "line1\nline2\nline3"),
    ("numbers_symbols", "1234567890 !@#$%^&*()_+-=[]{}|;:'\",.<>/?`~"),
    ("unicode", "café ☕ 日本語 ✓"),
    ("spaces", "   leading and trailing   "),
]


def main():
    if len(sys.argv) < 3:
        print(__doc__, file=sys.stderr)
        return 2
    op, serial = sys.argv[1], sys.argv[2]

    if op in ("pc2phone", "phone2pc"):
        text = sys.argv[3] if len(sys.argv) > 3 else f"AUDIT-{int(time.time())}"
        fn = check_pc2phone if op == "pc2phone" else check_phone2pc
        ok, detail = fn(serial, text, op)
        print(("PASS" if ok else "FAIL"), op, repr(text), detail)
        return 0 if ok else 1

    if op == "matrix":
        results = []
        for name, text in CASES:
            for direction in ("pc2phone", "phone2pc"):
                fn = check_pc2phone if direction == "pc2phone" else check_phone2pc
                ok, detail = fn(serial, text, f"{direction}-{name}")
                print(f"{'PASS' if ok else 'FAIL'}  {direction:9s} {name:16s} {detail}")
                results.append(ok)
        # repeated same text (no drift / dedupe correctness)
        for i in range(3):
            ok, d = check_phone2pc(serial, "REPEAT-SAME", "repeat")
            print(f"{'PASS' if ok else 'FAIL'}  phone2pc  repeat#{i}       {d}")
            results.append(ok)
        # rapid alternating
        rapid_ok = 0
        for i in range(6):
            t = f"RAPID-{i}-{'PC' if i % 2 == 0 else 'PHONE'}"
            fn = check_pc2phone if i % 2 == 0 else check_phone2pc
            ok, d = fn(serial, t, "rapid")
            rapid_ok += ok
            print(f"{'PASS' if ok else 'FAIL'}  rapid#{i}  {t:14s} {d}")
            results.append(ok)
        print(f"\nTOTAL: {sum(results)}/{len(results)} passed")
        return 0 if all(results) else 1
    print("unknown op", op, file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main())
