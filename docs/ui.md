# PhoneBridge UI/UX Design System & Pairing Architecture

## 1. Overview & Design Principles

PhoneBridge delivers a unified multi-screen integration experience across Linux desktop and Android mobile platforms. The UI/UX system is built on Flutter with Material 3, adhering to high-density desktop usability and ergonomic mobile touch targets.

### Core Principles
1. **Accurate Trust & Session State**: Unilateral local trust (`~/.config/phonebridge/trusted_devices.json`) is never mislabeled as mutual pairing (`PAIRED`). The UI strictly distinguishes between local trust, active connections, and unpaired devices.
2. **Deterministic Feedback & Zero Deadlocks**: If remote app data is cleared or reinstalled, the user is never locked out of pairing. A visible `RE-PAIR` affordance allows immediate re-negotiation of mutual trust without manual file deletion.
3. **Responsive Adaptive Navigation**: Adapts between a persistent `NavigationRail` with header branding and live status badges on wide displays ($\ge 720\text{ dp}$), and a clean `NavigationBar` on compact mobile viewports.
4. **Privacy First (Zero Logging Rule)**: Sensitive payloads (clipboard content, notification titles/bodies, keystrokes, remote coordinates) are strictly excluded from console logs, debug toasts, and error presentations. Only event types and byte lengths are logged.

---

## 2. Design Tokens & Styling System

Centralized in `ui/lib/main.dart` via `PhoneBridgeApp`:

* **Primary Seed**: `#2563EB` (Tailwind / Material Blue 600)
* **Card Corner Radius**: `16 dp` with subtle outline border (`outlineVariant` alpha `0.35`–`0.45`), `elevation: 0`.
* **Button Corner Radius**: `12 dp` for `FilledButton`, `FilledButton.tonal`, and `OutlinedButton`.
* **Dialog Corner Radius**: `20 dp`, `elevation: 3`.
* **SnackBar**: Floating behavior with `12 dp` rounded corners.
* **Badges & Pills**: `BorderRadius.circular(12)` with colored background fill (`alpha: 0.12`–`0.15`) and matched outline border (`alpha: 0.4`).

---

## 3. Discovered Device Quad-State Model

In `ui/lib/screens/devices_screen.dart`, discovered devices map deterministically to one of four states:

| State | Badge | Color | Action Button | Meaning |
|---|---|---|---|---|
| **CONNECTED** | `CONNECTED` (Icons.link) | Emerald Green (`Colors.green`) | N/A | Active WebRTC session established with this peer. |
| **TRUSTED** | `TRUSTED` (Icons.verified_user_outlined) | Primary Blue (`theme.primary`) | `RE-PAIR` (`OutlinedButton`) | Device identity key stored in local trust store. If remote app was reset, user can tap `RE-PAIR` to re-exchange SAS. |
| **UNPAIRED** | `UNPAIRED` (Icons.link_off) | Amber (`Colors.amber.shade700`) | `PAIR` (`FilledButton.tonal`) | New device discovered on LAN; mutual trust has not yet been established. |
| **STALE** | `STALE` (Icons.schedule) | Orange (`Colors.orange`) | Disabled / None | mDNS announcement aged past `StaleTimeout` (10s) without renewal. |

### Pairing & SAS Verification Flow
1. **Initiation**: User clicks `PAIR` or `RE-PAIR` on a discovered device tile.
2. **Handshake**: Initiator calls remote LAN HTTP signaling (`POST /pairing/request` on port 7804).
3. **Verification Dialog**: Displays a dedicated security dialog featuring:
   - Security shield header icon
   - High-contrast 6-digit Short Authentication String (SAS) in monospace code card
   - Explicit `REJECT` vs `CONFIRM MATCH` actions
4. **Mutual Trust Commitment**: On confirmation, signature is verified and mutual Ed25519 identity keys are committed to disk on both peers.

---

## 4. Screen Surfaces

* **AppScaffold**: Adaptive shell with unified link status badge (`linkPhaseColor`), transfer activity indicator, and refresh/settings actions.
* **Home**: Connection hero card, live screen mirroring summary, clipboard quick toggle, file transfer summary card, and recent activity log.
* **Devices**: "This Device" identity card with network discovery pulse, Discovered Devices list with quad-state badges, Trusted Devices store with revoke confirmation gate, and 3-step Zero-Trust pairing guide.
* **Screen**: Responsive frame mirror supporting low-latency JPEG streams (`ScreenFrameView`), remote touch/mouse interaction, global action navigation bar (Back/Esc, Home, Recents, Notifications, Text commit), and stream telemetry diagnostics.
* **Clipboard**: Master sync toggle, sync state indicator (Ambient Active, Write-Only Dormant, Unavailable), manual sync trigger, Wayland / Android IME guidance cards, and privacy guarantees.
* **Activity**: Live notifications feed (Phase 8), active and past file transfers with cancel/open actions, filter chips (All, Screen, Clipboard, Files, Notifications), and detailed event audit tiles.
* **Settings & Diagnostics**: Streaming resolution/FPS controls, clipboard auto-sync toggle, LAN signaling configuration, and live frame telemetry view.
