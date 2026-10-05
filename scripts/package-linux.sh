#!/usr/bin/env bash
# PhoneBridge Linux Beta Packaging Script
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BUILD_DIR="$REPO_ROOT/build/dist"
PKG_NAME="phonebridge-linux-x86_64-beta"
STAGE_DIR="$BUILD_DIR/$PKG_NAME"
TARBALL="$BUILD_DIR/$PKG_NAME.tar.gz"

echo "=== Packaging PhoneBridge Linux Beta (x86_64) ==="

# 1. Resolve Go toolchain
if command -v go >/dev/null 2>&1; then
    GO_BIN="go"
elif [ -x "$HOME/go-sdk/go/bin/go" ]; then
    GO_BIN="$HOME/go-sdk/go/bin/go"
elif [ -x "/usr/local/go/bin/go" ]; then
    GO_BIN="/usr/local/go/bin/go"
else
    echo "Error: Go compiler not found" >&2
    exit 1
fi
export PATH="$(dirname "$GO_BIN"):$PATH"

# 2. Resolve Flutter toolchain
if ! command -v flutter >/dev/null 2>&1; then
    echo "Error: flutter CLI not found in PATH" >&2
    exit 1
fi

# 3. Clean prior build staging
rm -rf "$STAGE_DIR" "$TARBALL"
mkdir -p "$STAGE_DIR/bin" "$STAGE_DIR/ui" "$STAGE_DIR/systemd" "$STAGE_DIR/scripts"
for size in 16 22 24 32 48 64 128 256 512; do
    mkdir -p "$STAGE_DIR/desktop/icons/hicolor/${size}x${size}/apps"
done

# 4. Build Go binaries with metadata
VERSION="0.1.0"
GIT_COMMIT=$(git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS="-s -w -X main.version=$VERSION -X main.commit=$GIT_COMMIT -X main.buildDate=$BUILD_DATE"

echo "Building core Go binaries (commit $GIT_COMMIT)..."
mkdir -p "$REPO_ROOT/core/build"
(cd "$REPO_ROOT/core" && "$GO_BIN" build -ldflags="$LDFLAGS" -o "$REPO_ROOT/core/build/phonebridge-daemon" ./cmd/daemon)
(cd "$REPO_ROOT/core" && "$GO_BIN" build -ldflags="$LDFLAGS" -o "$REPO_ROOT/core/build/ipcdrv" ./cmd/ipcdrv)

install -m 0755 "$REPO_ROOT/core/build/phonebridge-daemon" "$STAGE_DIR/bin/phonebridge-daemon"
install -m 0755 "$REPO_ROOT/core/build/ipcdrv" "$STAGE_DIR/bin/ipcdrv"

# 5. Build C clipboard helpers
echo "Compiling C clipboard helpers..."
make -C "$REPO_ROOT/linux/mutter-helper" clean all
make -C "$REPO_ROOT/linux/wayland-helper" clean all

install -m 0755 "$REPO_ROOT/linux/mutter-helper/phonebridge-mutter-helper" "$STAGE_DIR/bin/phonebridge-mutter-helper"
install -m 0755 "$REPO_ROOT/linux/wayland-helper/phonebridge-wayland-helper" "$STAGE_DIR/bin/phonebridge-wayland-helper"

# 6. Build Flutter Linux release bundle
echo "Building Flutter Linux release bundle..."
(cd "$REPO_ROOT/ui" && flutter build linux --release)

echo "Staging Flutter bundle..."
cp -a "$REPO_ROOT/ui/build/linux/x64/release/bundle/"* "$STAGE_DIR/ui/"

# 7. Stage systemd unit, desktop files, icon, scripts and documentation
echo "Staging systemd service, desktop launcher, scripts and docs..."
install -m 0644 "$REPO_ROOT/linux/packaging/systemd/phonebridge.service" "$STAGE_DIR/systemd/phonebridge.service"
install -m 0644 "$REPO_ROOT/linux/packaging/desktop/dev.phonebridge.ui.desktop" "$STAGE_DIR/desktop/dev.phonebridge.ui.desktop"
for size in 16 22 24 32 48 64 128 256 512; do
    install -m 0644 "$REPO_ROOT/linux/packaging/desktop/icons/hicolor/${size}x${size}/apps/dev.phonebridge.ui.png" "$STAGE_DIR/desktop/icons/hicolor/${size}x${size}/apps/dev.phonebridge.ui.png"
done

install -m 0755 "$REPO_ROOT/linux/packaging/scripts/check-env.sh" "$STAGE_DIR/scripts/check-env.sh"
install -m 0755 "$REPO_ROOT/linux/packaging/scripts/install.sh" "$STAGE_DIR/scripts/install.sh"
install -m 0755 "$REPO_ROOT/linux/packaging/scripts/uninstall.sh" "$STAGE_DIR/scripts/uninstall.sh"
install -m 0644 "$REPO_ROOT/linux/packaging/README.md" "$STAGE_DIR/README.md"

# 8. Create compressed distribution tarball
echo "Creating $TARBALL..."
tar -C "$BUILD_DIR" -czf "$TARBALL" "$PKG_NAME"

# Also symlink or copy to repo root for easy access
cp "$TARBALL" "$REPO_ROOT/$PKG_NAME.tar.gz"

echo ""
echo "=== Package Built Successfully ==="
echo "Tarball: $REPO_ROOT/$PKG_NAME.tar.gz"
ls -lh "$REPO_ROOT/$PKG_NAME.tar.gz"
tar -tzf "$REPO_ROOT/$PKG_NAME.tar.gz" | head -n 25 || true
