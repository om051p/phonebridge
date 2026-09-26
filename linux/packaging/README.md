# PhoneBridge Linux Beta Distribution (x86_64)

PhoneBridge bridges your Android mobile device and Linux desktop seamlessly over local WebRTC, enabling real-time screen mirroring, bi-directional clipboard synchronization, file transfers, and notification mirroring without cloud dependencies.

## Package Contents

```text
phonebridge-linux-x86_64-beta/
├── bin/
│   ├── phonebridge-daemon          # Background engine daemon (systemd service)
│   ├── phonebridge-mutter-helper   # GNOME/Mutter Wayland clipboard backend
│   ├── phonebridge-wayland-helper  # wlroots (Sway/Hyprland) clipboard backend
│   └── ipcdrv                      # Diagnostic and testing CLI tool
├── ui/                             # Flutter Linux desktop application bundle
├── systemd/
│   └── phonebridge.service         # systemd --user service unit definition
├── desktop/
│   ├── dev.phonebridge.ui.desktop  # XDG desktop launcher entry
│   └── icons/hicolor/512x512/apps/dev.phonebridge.ui.png
├── scripts/
│   ├── check-env.sh                # Pre-flight environment diagnostic tool
│   ├── install.sh                  # Rootless user installation script
│   └── uninstall.sh                # Clean uninstallation script
└── README.md                       # This document
```

## System Requirements

- **Architecture**: x86_64 (amd64)
- **Operating System**: Linux with GNU C Library (glibc >= 2.35)
  - Ubuntu 22.04 LTS / 24.04 LTS+, Fedora 38+, Arch Linux, Debian 12+
- **Desktop Environment**:
  - GNOME Shell (Wayland/X11) or wlroots-based compositors (Sway, Hyprland, Wayfire)
- **User Manager**: `systemd --user`
- **Graphics/UI Libraries**: `libgtk-3.so.0`, `libgio-2.0.so.0`, `libglib-2.0.so.0`
- **Optional Tools**: `ffplay` or `ffmpeg` (for video display sink)

## Pre-flight Check

Before installing, you can verify your environment:
```bash
./scripts/check-env.sh
```

## Installation

Do **NOT** use `sudo`. PhoneBridge installs strictly into your user directory (`~/.local`):
```bash
./scripts/install.sh
```

The installer will:
1. Validate system prerequisites.
2. Install executables and clipboard helpers into `~/.local/bin/`.
3. Install the Flutter desktop client bundle into `~/.local/lib/phonebridge/ui/`.
4. Install and enable the `systemd --user` background service (`phonebridge.service`).
5. Register the application launcher and icon in your desktop application menu.
6. Verify Local IPC responsiveness via the diagnostic tool.

## Usage

### Launching the Graphical User Interface
Launch PhoneBridge from your desktop application menu or via terminal:
```bash
phonebridge
```

### Checking Daemon Status
The background engine daemon runs as a `systemd --user` service:
```bash
systemctl --user status phonebridge.service
```

To view live daemon logs:
```bash
journalctl --user -u phonebridge.service -f
```

## Diagnostics (`ipcdrv`)

`ipcdrv` is a command-line diagnostic tool for testing and inspecting the local engine:
```bash
# Verify daemon health and Local IPC communication
ipcdrv ping

# Inspect current WebRTC session state
ipcdrv session

# List discovered and trusted Android devices
ipcdrv devices
ipcdrv trusted

# Inspect clipboard synchronization status
ipcdrv clipboard
```

## Uninstallation

To remove PhoneBridge:
```bash
./scripts/uninstall.sh
```

By default, existing device pairings and trust keys in `~/.config/phonebridge/` are preserved. To completely remove all credentials and pairing data, use:
```bash
./scripts/uninstall.sh --purge
```

## Security & Architecture Notes

- **Zero Root Privileges**: The daemon, helpers, and GUI client execute entirely unprivileged in user space.
- **Local IPC Isolation**: Local IPC operates over a Unix Domain Socket at `$XDG_RUNTIME_DIR/phonebridge/engine.sock` with directory permissions `0700`, socket permissions `0600`, and a bearer token authenticated with `0600` permissions.
- **LAN Communication**: LAN transport on port 7804 requires mutual Ed25519 authentication and Numeric SAS verification before establishing WebRTC peer channels.
