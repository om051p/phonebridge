#!/usr/bin/env bash
set -euo pipefail

export WAYLAND_DISPLAY=wayland-1
export XDG_RUNTIME_DIR=/run/user/1000

SC_RUNNER="./06-cosmic-clipboard/bin/security_context_runner"
LOG_DIR="./06-cosmic-clipboard/results"
mkdir -p "$LOG_DIR"
OUT="$LOG_DIR/section_g_flatpak.log"
rm -f "$OUT"

exec > >(tee -a "$OUT") 2>&1

echo "=== SECTION G: FLATPAK & SANDBOX SECURITY EVALUATION ==="
echo "Compositor: cosmic-comp (Smithay)"
echo "Wayland Socket: $XDG_RUNTIME_DIR/$WAYLAND_DISPLAY"
echo "Date: $(date -u '+%Y-%m-%d %H:%M:%S UTC')"
echo

# G.1: Compositor Security Context Global Advertising
echo "--- G.1: wp_security_context_manager_v1 Global Check ---"
WAYLAND_DISPLAY=wayland-1 wayland-info | grep -A 3 -B 1 "wp_security_context_manager_v1" || true
echo

# G.2: Execution of Sandboxed Client via wp_security_context_v1
echo "--- G.2: Emulated Flatpak Sandbox via wp_security_context_v1 ---"
$SC_RUNNER
echo

# G.3: Architectural Analysis for Flatpak Packaging
echo "--- G.3: Empirical Permissions & Requirements for PhoneBridge Flatpak ---"
echo "Analysis based on live COSMIC/Smithay security context execution:"
echo "1. When an application runs under a standard Flatpak sandbox with '--socket=wayland',"
echo "   Flatpak attaches a wp_security_context_v1 metadata block to the Wayland connection."
echo "2. Results from G.2 demonstrate whether COSMIC hides or restricts zwlr_data_control_manager_v1."
echo "3. If restricted, a background sync daemon packaged as Flatpak requires:"
echo "   - Host Wayland socket hole (--filesystem=xdg-run/wayland-1 or unset security context), OR"
echo "   - Distribution as a native host binary / systemd user service rather than a sandboxed Flatpak,"
echo "   - OR custom permissions if COSMIC policy rules allow explicit data-control granting."
echo
echo "=== SECTION G COMPLETE ==="
