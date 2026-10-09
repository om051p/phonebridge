#!/usr/bin/env python3
"""Drive the PhoneBridge Android app's Flutter semantics over adb.

The app runs Flutter, whose semantics tree is exposed to uiautomator because an
accessibility service is enabled on the target devices. Each node carries
`content-desc` (Flutter's label) and `bounds`, which is enough to read state and
tap controls deterministically.

Usage:
  android_ui.py dump <serial> [substr]        # list content-desc [ + bounds]
  android_ui.py badge <serial>                # print the app-bar badge line
  android_ui.py tap <serial> <substr>         # tap centre of first match
  android_ui.py tapall <serial> <substr>      # tap centre of every match
  android_ui.py contains <serial> <substr>    # exit 0 if any content-desc match
"""
import re
import subprocess
import sys
import time
import xml.etree.ElementTree as ET

ADB = "/opt/android-sdk/platform-tools/adb"


def adb(serial, *args, capture=True):
    cmd = [ADB, "-s", serial] + list(args)
    return subprocess.run(cmd, capture_output=capture, text=True)


def dump_xml(serial, retries=3):
    for _ in range(retries):
        adb(serial, "shell", "uiautomator", "dump", "--compressed", "/sdcard/pb_ui.xml")
        out = adb(serial, "exec-out", "cat", "/sdcard/pb_ui.xml").stdout
        if out and "<hierarchy" in out:
            return out
        time.sleep(0.4)
    return ""


def nodes(serial):
    xml = dump_xml(serial)
    if not xml:
        return []
    xml = xml[xml.index("<hierarchy"):]
    try:
        root = ET.fromstring(xml)
    except ET.ParseError:
        return []
    out = []
    for n in root.iter("node"):
        desc = n.get("content-desc") or ""
        text = n.get("text") or ""
        bounds = n.get("bounds") or ""
        m = re.match(r"\[(\d+),(\d+)\]\[(\d+),(\d+)\]", bounds)
        cx = cy = None
        if m:
            x1, y1, x2, y2 = map(int, m.groups())
            cx, cy = (x1 + x2) // 2, (y1 + y2) // 2
        out.append({"desc": desc, "text": text, "bounds": bounds,
                    "cx": cx, "cy": cy, "clickable": n.get("clickable")})
    return out


def matches(ns, substr):
    return [n for n in ns if (substr.lower() in n["desc"].lower()
                              or substr.lower() in n["text"].lower())]


def main():
    if len(sys.argv) < 3:
        print(__doc__, file=sys.stderr)
        return 2
    op, serial = sys.argv[1], sys.argv[2]
    arg = sys.argv[3] if len(sys.argv) > 3 else ""

    ns = nodes(serial)
    if op == "dump":
        for n in ns:
            label = n["desc"] or n["text"]
            if not label:
                continue
            if arg and arg.lower() not in label.lower():
                continue
            print(f"{n['bounds']:>22}  {label!r}")
        return 0
    if op == "badge":
        # App-bar badge: the first content-desc starting with "PhoneBridge".
        for n in ns:
            if n["desc"].startswith("PhoneBridge"):
                print(n["desc"].replace("\n", " | "))
                return 0
        print("(badge not found)")
        return 1
    if op in ("tap", "tapall"):
        ms = [n for n in matches(ns, arg) if n["cx"] is not None]
        if not ms:
            print("no match for", repr(arg), file=sys.stderr)
            return 1
        if op == "tap":
            ms = ms[:1]
        for n in ms:
            print("tap", n["desc"] or n["text"], "@", n["cx"], n["cy"])
            adb(serial, "shell", "input", "tap", str(n["cx"]), str(n["cy"]))
            time.sleep(0.3)
        return 0
    if op == "contains":
        ok = bool(matches(ns, arg))
        print("yes" if ok else "no")
        return 0 if ok else 1
    print("unknown op", op, file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main())
