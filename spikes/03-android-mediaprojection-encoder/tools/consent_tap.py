#!/usr/bin/env python3
"""Find a node in a uiautomator XML dump and print "x y" tap coordinates.

  consent_tap.py <dump.xml>            -> best guess for the dialog's confirm button
  consent_tap.py <dump.xml> <pattern>  -> largest node whose text / content-desc /
                                          resource-id matches the regex

Exits non-zero when nothing matches. Used by tools/spike03.sh to drive the
MediaProjection consent dialog (including the Android 15 capture-scope spinner)
unattended. The dialog is SystemUI-owned, so matching is attribute-based rather
than layout-based.
"""
import re
import sys
import xml.etree.ElementTree as ET

BOUNDS = re.compile(r"\[(\d+),(\d+)\]\[(\d+),(\d+)\]")

# Ordered preference for the confirm button: (attribute, regex, weight)
RULES = [
    ("resource-id", r".*android:id/button1$", 100),
    ("text", r"^\s*(start now|start|allow|share|cast)\s*$", 80),
    ("content-desc", r"^\s*(start now|start|allow|share|cast)\s*$", 70),
    ("text", r"(?i)(start now|start recording|begin)", 60),
]


def center(node):
    m = BOUNDS.match(node.get("bounds") or "")
    if not m:
        return None
    x1, y1, x2, y2 = (int(g) for g in m.groups())
    if x2 - x1 <= 0 or y2 - y1 <= 0:
        return None
    return ((x1 + x2) // 2, (y1 + y2) // 2, (x2 - x1) * (y2 - y1))


def main() -> int:
    if len(sys.argv) < 2:
        print("usage: consent_tap.py <ui-dump.xml> [regex]", file=sys.stderr)
        return 2
    try:
        tree = ET.parse(sys.argv[1])
    except Exception as exc:  # noqa: BLE001
        print(f"parse error: {exc}", file=sys.stderr)
        return 3

    if len(sys.argv) >= 3:
        pattern = sys.argv[2]
        best = None
        for node in tree.iter("node"):
            c = center(node)
            if not c:
                continue
            haystacks = [
                node.get("text") or "",
                node.get("content-desc") or "",
                node.get("resource-id") or "",
            ]
            if any(re.search(pattern, h, re.IGNORECASE) for h in haystacks):
                if node.get("enabled") == "false":
                    continue
                if best is None or c[2] > best[2]:
                    best = c
        if best is None:
            print(f"no node matching {pattern!r}", file=sys.stderr)
            return 1
        print(f"{best[0]} {best[1]}")
        return 0

    best = None  # (weight, x, y, label)
    for node in tree.iter("node"):
        c = center(node)
        if not c:
            continue
        for attr, pattern, weight in RULES:
            value = node.get(attr) or ""
            if value and re.search(pattern, value):
                label = f"{attr}={value.strip()!r}"
                if best is None or weight > best[0]:
                    best = (weight, c[0], c[1], label)
                break

    if best is None:
        print("no candidate button found", file=sys.stderr)
        return 1

    _, cx, cy, label = best
    print(f"{cx} {cy}")
    print(f"# {label}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
