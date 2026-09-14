# Spike 07-cosmic-input — COSMIC Remote-Input Strategy

> Status: `EXPERIMENTAL` · Phase: 0 (framework only)


## Hypothesis
Remote input injection on COSMIC Wayland is possible via portal/EIS or `/dev/uinput` with appropriate privilege.

## Question
Portal RemoteDesktop/EIS vs `/dev/uinput` vs other: what works on COSMIC and how is it privileged/confined?

## Target environment
- COSMIC Wayland, XDG Desktop Portals, `libei` status, uinput permissions

## Minimal prototype
- Request RemoteDesktop session via portal; attempt input injection; fallback to uinput with udev rule; compare UX/privilege prompts.

## Success criteria
- At least one path works with user-approved privilege; behavior documented for deb/Flatpak/systemd.

## Failure criteria
- No approved path without root-equivalent privilege that we can justify for standard product.

## Perf / security concerns
- Never bypass Wayland security via hacks; isolate privileged helper.

## Decision to be made
Lock Linux input strategy and required packaging/permission model (standard vs enhanced tier).

