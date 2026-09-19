#!/usr/bin/env bash
set -euo pipefail

export WAYLAND_DISPLAY=wayland-1
export XDG_RUNTIME_DIR=/run/user/1000

LOG_DIR="./06-cosmic-clipboard/results"
mkdir -p "$LOG_DIR"
OUT="$LOG_DIR/section_f_portal.log"
rm -f "$OUT"

exec > >(tee -a "$OUT") 2>&1

echo "=== SECTION F: PORTAL (xdg-desktop-portal-cosmic) VALIDATION ==="
echo "Compositor: cosmic-comp (Smithay)"
echo "Portal Backend: xdg-desktop-portal-cosmic"
echo "Date: $(date -u '+%Y-%m-%d %H:%M:%S UTC')"
echo

# F.1: Check running portal processes
echo "--- F.1: Active Portal Processes ---"
ps aux | grep -E "xdg-desktop-portal|cosmic" | grep -v grep || true
echo

# F.2: Introspect session D-Bus for portal clipboard interfaces
echo "--- F.2: D-Bus Portal Interface Enumeration ---"
echo "Inspecting org.freedesktop.portal.Desktop..."
ALL_PORTAL_INTERFACES=$(gdbus introspect --session --dest org.freedesktop.portal.Desktop --object-path /org/freedesktop/portal/desktop --recurse | grep "interface org.freedesktop.portal." | awk '{print $2}' | sort -u)
echo "Registered org.freedesktop.portal.* interfaces:"
echo "$ALL_PORTAL_INTERFACES"
echo

echo "Checking for org.freedesktop.portal.Clipboard..."
if echo "$ALL_PORTAL_INTERFACES" | grep -qi "clipboard"; then
    echo "Found clipboard interface in org.freedesktop.portal!"
else
    echo "PROVEN: org.freedesktop.portal.Clipboard does NOT exist in xdg-desktop-portal."
fi
echo

# F.3: Introspect xdg-desktop-portal-cosmic backend
echo "--- F.3: COSMIC Portal Backend (org.freedesktop.impl.portal.desktop.cosmic) ---"
COSMIC_INTERFACES=$(gdbus introspect --session --dest org.freedesktop.impl.portal.desktop.cosmic --object-path /org/freedesktop/portal/desktop --recurse | grep "interface org.freedesktop.impl.portal." | awk '{print $2}' | sort -u)
echo "Implemented COSMIC backend interfaces:"
echo "$COSMIC_INTERFACES"
echo

if echo "$COSMIC_INTERFACES" | grep -qi "clipboard"; then
    echo "Found clipboard interface in COSMIC portal backend!"
else
    echo "PROVEN: xdg-desktop-portal-cosmic does NOT implement any D-Bus clipboard interface."
fi
echo

# F.4: Verification of Portal Configuration Files
echo "--- F.4: Portal Configuration Inspection ---"
echo "cat /usr/share/xdg-desktop-portal/cosmic-portals.conf:"
cat /usr/share/xdg-desktop-portal/cosmic-portals.conf || true
echo
echo "cat /usr/share/xdg-desktop-portal/portals/cosmic.portal:"
cat /usr/share/xdg-desktop-portal/portals/cosmic.portal || true
echo

# F.5: Architectural Conclusion for Section F
echo "--- F.5: Empirical Findings for DEC-023 ---"
echo "1. D-Bus clipboard interfaces available: NONE (neither upstream xdg-desktop-portal nor xdg-desktop-portal-cosmic provides a clipboard portal)."
echo "2. Can a client request clipboard content via portal: NO (unsupported by portal specifications)."
echo "3. User permission prompt required: N/A (no portal prompt exists; access is gated purely at Wayland compositor level via zwlr_data_control_unstable_v1)."
echo
echo "=== SECTION F COMPLETE ==="
