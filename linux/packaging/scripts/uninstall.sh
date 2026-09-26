#!/usr/bin/env bash
# PhoneBridge Linux Rootless Uninstaller
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

if [ "${EUID:-$(id -u)}" -eq 0 ]; then
    echo -e "${RED}Error: PhoneBridge runs in rootless user space.${NC}" >&2
    echo "Please run uninstall.sh as standard user without sudo or root privileges." >&2
    exit 1
fi

BIN_DIR="${HOME}/.local/bin"
LIB_DIR="${HOME}/.local/lib/phonebridge"
SYSTEMD_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
APP_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/applications"
ICON_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/icons/hicolor/512x512/apps"
CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/phonebridge"
RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$UID}/phonebridge"

PURGE=0
for arg in "$@"; do
    case "$arg" in
        --purge)
            PURGE=1
            ;;
        --help|-h)
            echo "Usage: ./scripts/uninstall.sh [options]"
            echo ""
            echo "Options:"
            echo "  --purge      Delete configuration and pairing credentials in ~/.config/phonebridge"
            echo "  --help, -h   Show this help message"
            exit 0
            ;;
    esac
done

echo -e "${BLUE}=== Uninstalling PhoneBridge Linux Beta ===${NC}"

# 1. Stop and disable systemd user service
if systemctl --user is-enabled --quiet phonebridge.service 2>/dev/null || systemctl --user is-active --quiet phonebridge.service 2>/dev/null; then
    echo "Stopping and disabling phonebridge user service..."
    systemctl --user disable --now phonebridge.service 2>/dev/null || true
fi

# Clean up lingering daemon process if any (strictly current user and exact name)
if pgrep -u "$UID" -x "phonebridge-daemon" >/dev/null 2>&1; then
    echo "Stopping lingering daemon process..."
    sleep 0.5
    pkill -u "$UID" -x "phonebridge-daemon" 2>/dev/null || true
fi

# 2. Remove systemd user unit
if [ -f "$SYSTEMD_DIR/phonebridge.service" ]; then
    echo "Removing systemd service unit..."
    rm -f "$SYSTEMD_DIR/phonebridge.service"
    systemctl --user daemon-reload 2>/dev/null || true
fi

# 3. Remove PhoneBridge binaries
echo "Removing binaries from $BIN_DIR..."
rm -f "$BIN_DIR/phonebridge-daemon"
rm -f "$BIN_DIR/phonebridge-mutter-helper"
rm -f "$BIN_DIR/phonebridge-wayland-helper"
rm -f "$BIN_DIR/ipcdrv"
rm -f "$BIN_DIR/phonebridge"

# 4. Remove UI bundle library
if [ -d "$LIB_DIR" ]; then
    echo "Removing application library at $LIB_DIR..."
    rm -rf "$LIB_DIR"
fi

# 5. Remove desktop launcher and icon
if [ -f "$APP_DIR/dev.phonebridge.ui.desktop" ]; then
    echo "Removing desktop launcher..."
    rm -f "$APP_DIR/dev.phonebridge.ui.desktop"
    if command -v update-desktop-database >/dev/null 2>&1; then
        update-desktop-database "$APP_DIR" 2>/dev/null || true
    fi
fi

if [ -f "$ICON_DIR/dev.phonebridge.ui.png" ]; then
    echo "Removing application icon..."
    rm -f "$ICON_DIR/dev.phonebridge.ui.png"
    if command -v gtk-update-icon-cache >/dev/null 2>&1; then
        gtk-update-icon-cache -f -t "${XDG_DATA_HOME:-$HOME/.local/share}/icons/hicolor" 2>/dev/null || true
    fi
fi

# 6. Remove runtime socket & token directory
if [ -d "$RUNTIME_DIR" ]; then
    echo "Cleaning up runtime directory ($RUNTIME_DIR)..."
    rm -rf "$RUNTIME_DIR"
fi

# 7. Configuration & Pairing Data
if [ "$PURGE" -eq 1 ]; then
    if [ -d "$CONFIG_DIR" ]; then
        echo -e "${YELLOW}Purging configuration and pairing keys at $CONFIG_DIR...${NC}"
        rm -rf "$CONFIG_DIR"
    fi
else
    if [ -d "$CONFIG_DIR" ]; then
        echo -e "${GREEN}Preserving configuration and pairing keys:${NC} $CONFIG_DIR"
        echo "  (To remove pairing data later, delete $CONFIG_DIR or run ./scripts/uninstall.sh --purge)"
    fi
fi

echo ""
echo -e "${GREEN}=== PhoneBridge Uninstallation Complete ===${NC}"
