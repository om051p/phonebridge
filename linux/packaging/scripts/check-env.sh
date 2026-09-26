#!/usr/bin/env bash
# PhoneBridge Linux Beta Environment Pre-flight Diagnostic Script
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

PASS_COUNT=0
WARN_COUNT=0
FAIL_COUNT=0

log_pass() {
    echo -e "  [${GREEN}PASS${NC}] $1"
    PASS_COUNT=$((PASS_COUNT + 1))
}

log_warn() {
    echo -e "  [${YELLOW}WARN${NC}] $1"
    WARN_COUNT=$((WARN_COUNT + 1))
}

log_fail() {
    echo -e "  [${RED}FAIL${NC}] $1"
    FAIL_COUNT=$((FAIL_COUNT + 1))
}

echo -e "${BLUE}=== PhoneBridge Linux Environment Pre-flight Check ===${NC}"

# 1. Architecture
ARCH=$(uname -m)
if [ "$ARCH" = "x86_64" ]; then
    log_pass "Architecture: $ARCH (supported)"
else
    log_fail "Architecture: $ARCH (expected x86_64)"
fi

# 2. Linux Kernel
KERNEL=$(uname -r)
log_pass "Linux Kernel: $KERNEL"

# 3. Glibc Version (minimum 2.35)
if command -v ldd >/dev/null 2>&1; then
    GLIBC_VER=$(ldd --version 2>&1 | head -n1 | grep -oE '[0-9]+\.[0-9]+' | head -n1 || true)
    if [ -n "$GLIBC_VER" ]; then
        MAJOR=$(echo "$GLIBC_VER" | cut -d. -f1)
        MINOR=$(echo "$GLIBC_VER" | cut -d. -f2)
        if [ "$MAJOR" -gt 2 ] || { [ "$MAJOR" -eq 2 ] && [ "$MINOR" -ge 35 ]; }; then
            log_pass "GNU libc: $GLIBC_VER (>= 2.35 required)"
        else
            log_fail "GNU libc: $GLIBC_VER (requires >= 2.35)"
        fi
    else
        log_warn "Unable to detect exact GNU libc version"
    fi
else
    log_warn "ldd command not found, cannot verify glibc"
fi

# 4. systemd --user
if command -v systemctl >/dev/null 2>&1; then
    if systemctl --user status >/dev/null 2>&1 || systemctl --user list-units >/dev/null 2>&1; then
        log_pass "systemd user daemon: active and reachable"
    else
        log_warn "systemd --user is present but user session manager is not active"
    fi
else
    log_fail "systemctl not found (systemd required for background daemon)"
fi

# 5. XDG Runtime Directory
XDG_RT="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"
if [ -d "$XDG_RT" ] && [ -w "$XDG_RT" ]; then
    log_pass "Runtime directory: $XDG_RT (writable)"
else
    log_fail "Runtime directory $XDG_RT does not exist or is not writable"
fi

# Ensure standard system admin tool paths are searchable for non-root queries (ldconfig)
for p in /usr/sbin /sbin /usr/local/sbin; do
    if [ -d "$p" ] && ! echo ":$PATH:" | grep -q ":$p:"; then
        export PATH="$PATH:$p"
    fi
done

# 6. Desktop UI Shared Libraries (GTK3, GLib, Gio)
check_lib() {
    local lib_name="$1"
    local desc="$2"
    local found=0
    if command -v ldconfig >/dev/null 2>&1; then
        if ldconfig -p 2>/dev/null | grep -q "$lib_name"; then
            found=1
        fi
    fi
    if [ "$found" -eq 0 ]; then
        for dir in /usr/lib/x86_64-linux-gnu /usr/lib64 /usr/lib /lib/x86_64-linux-gnu /lib64 /lib; do
            if [ -e "$dir/$lib_name" ]; then
                found=1
                break
            fi
        done
    fi

    if [ "$found" -eq 1 ]; then
        log_pass "Library $lib_name: present ($desc)"
    else
        log_fail "Library $lib_name: missing ($desc)"
    fi
}

check_lib "libgtk-3.so.0" "GTK+ 3 desktop toolkit"
check_lib "libgio-2.0.so.0" "GLib I/O and DBus library"
check_lib "libgobject-2.0.so.0" "GObject type system"
check_lib "libglib-2.0.so.0" "Core GLib runtime"

# 7. Wayland / Compositor Libraries & Tools
wayland_found=0
if command -v ldconfig >/dev/null 2>&1; then
    if ldconfig -p 2>/dev/null | grep -q "libwayland-client.so.0"; then
        wayland_found=1
    fi
fi
if [ "$wayland_found" -eq 0 ]; then
    for dir in /usr/lib/x86_64-linux-gnu /usr/lib64 /usr/lib /lib/x86_64-linux-gnu /lib64 /lib; do
        if [ -e "$dir/libwayland-client.so.0" ]; then
            wayland_found=1
            break
        fi
    done
fi

if [ "$wayland_found" -eq 1 ]; then
    log_pass "Library libwayland-client.so.0: present (Wayland client)"
else
    log_warn "Library libwayland-client.so.0: not detected (wlroots clipboard helper may fail if needed)"
fi

# 8. Display & Compositor Session
SESSION_TYPE="${XDG_SESSION_TYPE:-unknown}"
CURRENT_DESKTOP="${XDG_CURRENT_DESKTOP:-unknown}"
echo -e "  [${BLUE}INFO${NC}] Desktop: $CURRENT_DESKTOP, Session: $SESSION_TYPE"

if [ -n "${WAYLAND_DISPLAY:-}" ]; then
    log_pass "Wayland display: $WAYLAND_DISPLAY"
elif [ -n "${DISPLAY:-}" ]; then
    log_pass "X11 display: $DISPLAY"
else
    log_warn "No graphical DISPLAY or WAYLAND_DISPLAY detected in current shell"
fi

# 9. Clipboard Backend Detection
if echo "$CURRENT_DESKTOP" | grep -qi "gnome"; then
    log_pass "Compositor backend: GNOME Shell / Mutter (phonebridge-mutter-helper)"
elif echo "$CURRENT_DESKTOP" | grep -qiE "sway|hyprland|wayfire"; then
    log_pass "Compositor backend: wlroots-based ($CURRENT_DESKTOP) (phonebridge-wayland-helper)"
elif echo "$CURRENT_DESKTOP" | grep -qi "cosmic"; then
    log_warn "Compositor backend: COSMIC (requires COSMIC_DATA_CONTROL_ENABLED=1 or fallback)"
else
    log_pass "Compositor backend: Generic / Auto-detect ($CURRENT_DESKTOP)"
fi

# 10. Screen Streaming Display Sink (ffplay / ffmpeg)
if command -v ffplay >/dev/null 2>&1; then
    log_pass "Video sink: ffplay found ($(command -v ffplay))"
elif command -v ffmpeg >/dev/null 2>&1; then
    log_warn "Video sink: ffmpeg found, but ffplay not found in PATH"
else
    log_warn "Video sink: ffplay / ffmpeg not found in PATH (needed for screen mirroring display)"
fi

# 11. User PATH check for ~/.local/bin
USER_BIN="$HOME/.local/bin"
if echo ":$PATH:" | grep -q ":$USER_BIN:"; then
    log_pass "PATH: $USER_BIN is in PATH"
else
    log_warn "PATH: $USER_BIN is NOT currently in PATH. Add 'export PATH=\"\$HOME/.local/bin:\$PATH\"' to your ~/.bashrc or ~/.profile"
fi

echo ""
echo -e "Check Summary: ${GREEN}${PASS_COUNT} passed${NC}, ${YELLOW}${WARN_COUNT} warnings${NC}, ${RED}${FAIL_COUNT} failures${NC}"

if [ "$FAIL_COUNT" -gt 0 ]; then
    echo -e "${RED}Pre-flight failed: Please resolve the above failures before running PhoneBridge.${NC}"
    exit 1
else
    echo -e "${GREEN}Pre-flight passed: System is ready for PhoneBridge installation.${NC}"
    exit 0
fi
