# Spike 06-cosmic-clipboard — COSMIC Clipboard Access

> Status: `EXPERIMENTAL` · Phase: 0 (framework only)


## Hypothesis
Wayland/COSMIC clipboard can be accessed reliably from the Go daemon (data-control/portal/clipboard manager).

## Question
Which Wayland clipboard mechanism works on COSMIC (and fallback on other compositors): `wlr-data-control`, XDG portal, or clipboard manager?

## Target environment
- COSMIC Wayland (primary) + GNOME Wayland fallback

## Minimal prototype
- Go/C helper that reads/writes clipboard via candidate protocols; test under COSMIC session, including sandboxed (Flatpak) case.

## Success criteria
- Reliable read/write when app is focused and documented behavior when not.

## Failure criteria
- No stable protocol on COSMIC or portal-gated behavior that blocks daemon use.

## Perf / security concerns
- Clipboard is sensitive; scope access and avoid logging contents.

## Decision to be made
Choose COSMIC clipboard path and document compositor-specific behavior.

