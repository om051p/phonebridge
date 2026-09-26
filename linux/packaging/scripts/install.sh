#!/usr/bin/env bash
# PhoneBridge Linux Rootless Beta Installer
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

if [ "${EUID:-$(id -u)}" -eq 0 ]; then
    echo -e "${RED}Error: PhoneBridge is designed to run in rootless user space.${NC}" >&2
    echo "Please run install.sh as a standard user without sudo or root privileges." >&2
    exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PKG_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

BIN_DIR="${HOME}/.local/bin"
LIB_DIR="${HOME}/.local/lib/phonebridge"
SYSTEMD_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
APP_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/applications"
ICON_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/icons/hicolor/512x512/apps"

NO_START=0
for arg in "$@"; do
    case "$arg" in
        --no-start)
            NO_START=1
            ;;
        --help|-h)
            echo "Usage: ./scripts/install.sh [options]"
            echo ""
            echo "Options:"
            echo "  --no-start   Install components without starting the systemd user service"
            echo "  --help, -h   Show this help message"
            exit 0
            ;;
    esac
done

echo -e "${BLUE}=== Installing PhoneBridge Linux Beta ===${NC}"

# 1. Run Pre-flight Checks
if [ -f "$SCRIPT_DIR/check-env.sh" ]; then
    echo "Running pre-flight environment checks..."
    "$SCRIPT_DIR/check-env.sh" || {
        echo -e "${RED}Pre-flight checks failed. Aborting installation.${NC}" >&2
        exit 1
    }
    echo ""
fi

# 2. Stop running service if present (idempotent upgrade)
if systemctl --user is-active --quiet phonebridge.service 2>/dev/null; then
    echo "Stopping existing phonebridge user service..."
    systemctl --user stop phonebridge.service 2>/dev/null || true
fi

# If lingering daemon process exists, safely terminate exact process for this user
if pgrep -u "$UID" -x "phonebridge-daemon" >/dev/null 2>&1; then
    echo "Waiting for existing daemon process to exit..."
    sleep 0.5
    pkill -u "$UID" -x "phonebridge-daemon" 2>/dev/null || true
fi

# 3. Create target directory tree
mkdir -p "$BIN_DIR" "$LIB_DIR/ui" "$SYSTEMD_DIR" "$APP_DIR" "$ICON_DIR"

# 4. Install binaries
echo "Installing binaries into $BIN_DIR..."
install -m 0755 "$PKG_ROOT/bin/phonebridge-daemon" "$BIN_DIR/phonebridge-daemon"
install -m 0755 "$PKG_ROOT/bin/phonebridge-mutter-helper" "$BIN_DIR/phonebridge-mutter-helper"
install -m 0755 "$PKG_ROOT/bin/phonebridge-wayland-helper" "$BIN_DIR/phonebridge-wayland-helper"
install -m 0755 "$PKG_ROOT/bin/ipcdrv" "$BIN_DIR/ipcdrv"

# 5. Install Flutter desktop bundle
echo "Installing desktop client bundle into $LIB_DIR/ui..."
rm -rf "$LIB_DIR/ui"/*
cp -a "$PKG_ROOT/ui/"* "$LIB_DIR/ui/"
chmod 0755 "$LIB_DIR/ui/phonebridge"

# Install executable wrapper script in ~/.local/bin
cat << 'EOF' > "$BIN_DIR/phonebridge"
#!/usr/bin/env sh
APP_DIR="${HOME}/.local/lib/phonebridge/ui"
exec "$APP_DIR/phonebridge" "$@"
EOF
chmod 0755 "$BIN_DIR/phonebridge"

# 6. Install desktop launcher and icon
echo "Installing desktop launcher and icon..."
if [ -f "$PKG_ROOT/desktop/icons/hicolor/512x512/apps/dev.phonebridge.ui.png" ]; then
    install -m 0644 "$PKG_ROOT/desktop/icons/hicolor/512x512/apps/dev.phonebridge.ui.png" "$ICON_DIR/dev.phonebridge.ui.png"
fi

cat << EOF > "$APP_DIR/dev.phonebridge.ui.desktop"
[Desktop Entry]
Name=PhoneBridge
GenericName=Phone Integration Client
Comment=Seamless Linux and Android Integration (Screen, Clipboard, Files, Notifications)
Exec=$BIN_DIR/phonebridge %u
TryExec=$BIN_DIR/phonebridge
Icon=dev.phonebridge.ui
Terminal=false
Type=Application
Categories=Utility;Network;
StartupNotify=true
StartupWMClass=dev.phonebridge.phonebridge
Keywords=android;phone;screen;mirror;clipboard;notifications;remote;
MimeType=x-scheme-handler/phonebridge;
EOF
chmod 0644 "$APP_DIR/dev.phonebridge.ui.desktop"

if command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database "$APP_DIR" 2>/dev/null || true
fi
if command -v gtk-update-icon-cache >/dev/null 2>&1; then
    gtk-update-icon-cache -f -t "${XDG_DATA_HOME:-$HOME/.local/share}/icons/hicolor" 2>/dev/null || true
fi

# 7. Install and configure systemd --user service
echo "Configuring systemd --user service..."
install -m 0644 "$PKG_ROOT/systemd/phonebridge.service" "$SYSTEMD_DIR/phonebridge.service"

# Import active graphical session environment into systemd --user manager
systemctl --user import-environment DISPLAY WAYLAND_DISPLAY XDG_CURRENT_DESKTOP XDG_SESSION_TYPE 2>/dev/null || true
systemctl --user daemon-reload

if [ "$NO_START" -eq 0 ]; then
    echo "Starting phonebridge user service..."
    systemctl --user enable --now phonebridge.service

    # Health check probe: poll ipcdrv ping for up to 5 seconds
    echo "Verifying daemon responsiveness..."
    HEALTHY=0
    for _ in $(seq 1 10); do
        if "$BIN_DIR/ipcdrv" ping >/dev/null 2>&1; then
            HEALTHY=1
            break
        fi
        sleep 0.5
    done

    if [ "$HEALTHY" -eq 1 ]; then
        echo -e "  [${GREEN}OK${NC}] Daemon responded to Local IPC ping"
    else
        echo -e "  [${YELLOW}WARN${NC}] Daemon did not respond to ping immediately. Inspect: systemctl --user status phonebridge.service"
    fi

    # Security check: verify socket & token permissions
    XDG_RT="${XDG_RUNTIME_DIR:-/run/user/$UID}/phonebridge"
    if [ -d "$XDG_RT" ]; then
        DIR_PERM=$(stat -c "%a" "$XDG_RT" 2>/dev/null || true)
        if [ "$DIR_PERM" = "700" ]; then
            echo -e "  [${GREEN}OK${NC}] Socket directory permissions: $DIR_PERM"
        else
            echo -e "  [${YELLOW}WARN${NC}] Socket directory permissions: $DIR_PERM (expected 700)"
        fi
        if [ -S "$XDG_RT/engine.sock" ]; then
            SOCK_PERM=$(stat -c "%a" "$XDG_RT/engine.sock" 2>/dev/null || true)
            echo -e "  [${GREEN}OK${NC}] UDS socket permissions: $SOCK_PERM"
        fi
        if [ -f "$XDG_RT/token" ]; then
            TOK_PERM=$(stat -c "%a" "$XDG_RT/token" 2>/dev/null || true)
            echo -e "  [${GREEN}OK${NC}] Bearer token permissions: $TOK_PERM"
        fi
    fi
else
    echo "Skipping service startup (--no-start specified)."
fi

echo ""
echo -e "${GREEN}=== PhoneBridge Installation Complete ===${NC}"
echo "Installed components:"
echo "  - Daemon:      $BIN_DIR/phonebridge-daemon"
echo "  - Helpers:     $BIN_DIR/phonebridge-mutter-helper"
echo "                 $BIN_DIR/phonebridge-wayland-helper"
echo "  - Diagnostics: $BIN_DIR/ipcdrv"
echo "  - GUI App:     $BIN_DIR/phonebridge"
echo "  - Service:     $SYSTEMD_DIR/phonebridge.service"
echo "  - Launcher:    $APP_DIR/dev.phonebridge.ui.desktop"
echo ""
echo "To launch the user interface, run:"
echo "  phonebridge"
echo ""
echo "To inspect service status:"
echo "  systemctl --user status phonebridge.service"
echo "  ipcdrv ping"
echo "  ipcdrv session"
