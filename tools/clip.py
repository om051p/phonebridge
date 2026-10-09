#!/usr/bin/env python3
"""Set/get the GNOME (mutter) system clipboard via phonebridge-mutter-helper.

Usage:
  clip.py read                 -> print current system clipboard text to stdout
  clip.py write <text>         -> set the system clipboard (text/plain;charset=utf-8)
  clip.py writefile <path>     -> set the system clipboard from a file's bytes

Exits non-zero on protocol failure so audit scripts can trust the result.
"""
import os
import subprocess
import sys
import time

HELPER = os.path.expanduser("~/.local/bin/phonebridge-mutter-helper")
MIME = "text/plain;charset=utf-8"


def _read_line(f, timeout=5.0):
    end = time.time() + timeout
    buf = b""
    while time.time() < end:
        ch = f.read(1)
        if not ch:
            return None
        buf += ch
        if ch == b"\n":
            return buf.decode("utf-8", "replace").rstrip("\n")
    return None


def read_clipboard(timeout=3.0):
    p = subprocess.Popen([HELPER], stdin=subprocess.PIPE,
                         stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
    try:
        line = _read_line(p.stdout)  # STATUS=READY ...
        if line is None or not line.startswith("STATUS=READY"):
            return None, "helper not ready: %r" % (line,)
        deadline = time.time() + timeout
        while time.time() < deadline:
            line = _read_line(p.stdout, timeout=max(0.1, deadline - time.time()))
            if line is None:
                return None, "stream closed"
            if line.startswith("EVENT=READ_DATA"):
                # EVENT=READ_DATA mime=<m> len=<n>  then <n> bytes + '\n'
                parts = dict(kv.split("=", 1) for kv in line.split(" ")[1:])
                n = int(parts["len"])
                data = p.stdout.read(n)
                p.stdout.read(1)  # trailing newline
                return data, None
            if line.startswith("EVENT=SELECTION_CLEARED"):
                return b"", None
            if line.startswith("EVENT=READ_OVERSIZED"):
                return None, "oversized"
        return None, "timeout waiting for READ_DATA"
    finally:
        try:
            p.stdin.write(b"CMD=SHUTDOWN\n")
            p.stdin.flush()
        except Exception:
            pass
        try:
            p.wait(timeout=2)
        except Exception:
            p.kill()


def write_clipboard(data: bytes):
    if isinstance(data, str):
        data = data.encode("utf-8")
    p = subprocess.Popen([HELPER], stdin=subprocess.PIPE,
                         stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
    try:
        line = _read_line(p.stdout)
        if line is None or not line.startswith("STATUS=READY"):
            return "helper not ready: %r" % (line,)
        cmd = b"CMD=SET_SELECTION mime=%s len=%d\n" % (MIME.encode(), len(data))
        p.stdin.write(cmd + data + b"\n")
        p.stdin.flush()
        deadline = time.time() + 5
        while time.time() < deadline:
            line = _read_line(p.stdout, timeout=max(0.1, deadline - time.time()))
            if line is None:
                return "stream closed"
            if line.startswith("STATUS=OK cmd=SET_SELECTION"):
                return None
            if line.startswith("STATUS=ERROR"):
                return line
        return "timeout waiting for SET_SELECTION ack"
    finally:
        try:
            p.stdin.write(b"CMD=SHUTDOWN\n")
            p.stdin.flush()
        except Exception:
            pass
        try:
            p.wait(timeout=2)
        except Exception:
            p.kill()


def main():
    if len(sys.argv) < 2:
        print(__doc__, file=sys.stderr)
        return 2
    op = sys.argv[1]
    if op == "read":
        data, err = read_clipboard()
        if err:
            print("ERR", err, file=sys.stderr)
            return 1
        sys.stdout.buffer.write(data)
        return 0
    if op == "write":
        err = write_clipboard(" ".join(sys.argv[2:]))
        if err:
            print("ERR", err, file=sys.stderr)
            return 1
        return 0
    if op == "writefile":
        with open(sys.argv[2], "rb") as fh:
            err = write_clipboard(fh.read())
        if err:
            print("ERR", err, file=sys.stderr)
            return 1
        return 0
    print("unknown op", op, file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main())
