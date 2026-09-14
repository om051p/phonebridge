# Spike 10-remote-p2p-turn — Remote WebRTC P2P + TURN Fallback

> Status: `EXPERIMENTAL` · Phase: 0 (framework only)


## Hypothesis
ICE/STUN/P2P with TURN fallback via a lightweight signaling rendezvous can connect devices across separate networks.

## Question
P2P success rate across NAT types, STUN/TURN cost/latency, and minimal signaling server scope?

## Target environment
- Separate networks (mobile data ↔ home), varied NATs, TURN relay

## Minimal prototype
- Minimal signaling (SDP/ICE exchange) + ICE gathering + TURN fallback; measure P2P success, TURN bandwidth/latency; estimate relay cost.

## Success criteria
- Documented P2P rate and TURN fallback behavior; relay cost model.

## Failure criteria
- TURN cost/latency unacceptable or signaling scope creep (must not see plaintext payloads).

## Perf / security concerns
- TURN is ops cost; signaling must not handle plaintext app data; auth before SDP.

## Decision to be made
Lock signaling scope, STUN/TURN strategy, and self-hosted vs managed TURN choice.

