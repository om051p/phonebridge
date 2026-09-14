# Spike 09-lan-discovery — LAN Discovery

> Status: `EXPERIMENTAL` · Phase: 0 (framework only)


## Hypothesis
mDNS (with fallback) reliably discovers peers on same LAN for automatic connect.

## Question
mDNS robustness across home routers/AP isolation/client isolation, plus fallback (e.g. UDP broadcast, manual IP, QR-assisted).

## Target environment
- Home routers, mobile hotspot, enterprise APs, varied mDNS responders (Avahi/Bonjour)

## Minimal prototype
- mDNS advertise/browse between Android and Linux; measure discovery latency; test AP isolation case and fallback.

## Success criteria
- < 5 s discovery on typical home LAN; documented fallback when mDNS is blocked.

## Failure criteria
- Universal "auto discover always works" cannot be claimed.

## Perf / security concerns
- Only advertise minimal identity; no PII in mDNS.

## Decision to be made
Choose primary + fallback discovery and the UX when discovery fails.

