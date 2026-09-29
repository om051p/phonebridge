# PhoneBridge — end-to-end communication audit (Android ↔ Linux)

Scope: the complete phone↔desktop communication path, inspected against the frozen
beta at `62baa1d` on branch `feat/phase4-file-transfer`. No features were added and
no working component was rewritten. One measurement test was added
(`core/pkg/localipc/shutdown_latency_test.go`) — test-only, no production change.

Legend: **[Confirmed]** = proven by code + a live measurement or test in this audit.
**[Likely]** = proven in code, not reproduced at runtime. **[Potential]** = optimisation,
no defect asserted.

---

## 1. Complete current communication architecture

Three processes/populations:

```
┌─ Android app ─────────────────────────────────────────────────────────────┐
│ Flutter UI (ui/lib)                                                        │
│   │ MethodChannel dev.phonebridge/control + EventChannel dev.phonebridge/events
│   ▼                                                                        │
│ Kotlin host: MainActivity (control dispatch, 1 Hz stats), PhoneBridgeService│
│   (foreground svc, owns Go lifecycle + MulticastLock + NSD), LanSignalingServer
│   (:7804, hand-rolled HTTP/1.1), SessionNegotiation (pure rules), adapters   │
│   (clipboard / transfer / input / notification), NsdAdvertiser, IME,         │
│   AccessibilityService, ScreenCaptureEngine (MediaProjection + MediaCodec)   │
│   │ JNI: invoke(method, byte[]):byte[]  + typed natives (media/clipboard/..)  │
│   ▼                                                                        │
│ In-process Go core (core/cmd/android, -buildmode=c-shared)                   │
│   transport.go (pion PC: video track + control/clipboard/transfer/input/     │
│   notifications DCs), clipboard_bridge, transfer_bridge, input_bridge,       │
│   notification_bridge, discovery_bridge                                       │
└───────────────────────────────────────────────────────────────────────────┘
                    │ LAN: mDNS (_phonebridge._tcp) + HTTP/1.1 signaling
                    │ + WebRTC/SRTP (DTLS) media & data channels
┌─ Linux daemon (systemd --user, phonebridge.service) ────────────────────────┐
│ localipc UDS gRPC : engine.sock (SO_PEERCRED + bearer)  ── frames.Hub        │
│ SessionManager (sessions, pairing, trust)                                    │
│ SignalingServer :7804  (/health,/pairing/*,/session/{offer,answer,stop})     │
│ discovery (pion mdns: advertise + browse, 7 s browse-refresh)                │
│ clipboard (Engine + LinuxAdapter + phonebridge-mutter-helper subprocess)      │
│ transfer (Engine + destination dir)                                          │
│ engine.Session ──► pkg/receiver (answerer: video + all DCs)                  │
│    └── frames.TapSink (ffmpeg H.264→MJPEG) ──► Hub ──► StreamFrames          │
│ engine.InboundSession (offerer path — only control/clipboard/transfer)       │
└───────────────────────────────────────────────────────────────────────────┘
                    │ UDS gRPC (unary + StreamEvents + StreamFrames)
┌─ Linux Flutter UI (ui/lib/services/local_ipc_client.dart) ──────────────────┐
└───────────────────────────────────────────────────────────────────────────┘
```

DataChannel inventory (the whole inter-device surface):

| channel | created by | ordered | retransmit | used for |
|---|---|---|---|---|
| `control` | both | yes | **0 (unreliable by design)** | stats/pacing probes |
| `clipboard` | both | yes | reliable | clipboard text, 768 KiB ceiling |
| `transfer` | both | yes | reliable | file transfer frames, 256 KiB low-watermark |
| `input` | initiator only | yes | reliable | DEC-027 remote input (Linux→phone) |
| `notifications` | initiator only | yes | reliable | DEC-028 mirroring (phone→Linux) |

ICE: `ICEServers: []` on both sides — **no STUN/TURN** (LAN-only by decision); mDNS
candidate obfuscation is explicitly disabled so host candidates are plain LAN IPs.

---

## 2. End-to-end connection flow (the production path)

Linux-initiated (the path that works and is exercised by the frozen beta):

1. Both devices advertise `_phonebridge._tcp` (Linux: pion mdns with `DeviceID`;
   Android: `NsdAdvertiser`). Both **browse** (Linux: pion mdns; Android: the new Go
   `discovery_bridge`). Linux `discovery.refreshLoop` restarts its browse session
   every 7 s to defeat pion's per-session `seen` dedup.
2. Linux UI → `StartSession(deviceID)` → `resolveEndpoint` from the registry.
3. Trust gate: `TrustStore.IsTrusted` (paired, not revoked) — re-checked on every
   reconnect attempt, not just the first.
4. `SignalingClient.RequestOffer` → `POST http://<phone>:7804/session/offer`, signed
   with Ed25519 (`crypto.SignRequest`) carrying `protocol_version`/`version`/`capabilities`/
   `requested`.
5. Phone: `AuthValidator.verify` → `SessionNegotiation.negotiate` (version → codec →
   fps/bitrate advisory → geometry consent-bound) → on accept
   `GoBridge.mediaRelease(); mediaInit(); mediaCreateOffer()` → returns **its own SDP
   offer** plus the typed `actual` tuple.
6. Linux `pkg/receiver` `SetRemoteOffer` → `CreateAnswer` → `gatherComplete(2s)` →
   `POST /session/answer` (signed) → phone `mediaSetAnswer` + `mediaStart`.
7. Media: MediaProjection → MediaCodec → `nativeMediaOnFrame` → `MediaTransport` queue
   (256 AU) + shaper (4000 kbps / burst 3000, under `writeMu` so sequence numbers stay
   ordered) → `WriteRTP` → SRTP/DTLS → Linux `pc.OnTrack` → depacketizer → PSIGuard →
   TapSink (ffmpeg MJPEG) → Hub → `StreamFrames` → Flutter preview; and → ffplay sink.
8. Reverse direction: `SendInput` (input DC, validated + rate-limited), clipboard engine
   (clipboard DC, SHA-256 echo filter), transfer engine (transfer DC, watermark-gated),
   phone→Linux notifications (notifications DC).
9. Teardown: `StopSession` → `Stop()`/`teardown()` → local `stop` POST where applicable,
   transport closed, hub session ended by token, plugins/engines detached.

Phone-initiated (broken — see B1): Android `_connectDevice` passes
`receiverUrl = http://<pc>:7804` → `MainActivity.initiateWebRtcHandshake` →
`POST http://<pc>:7804/offer` with the phone's raw SDP offer.

---

## 3. Component-by-component findings

| Component | State | Notes |
|---|---|---|
| Discovery (Linux) | OK | advertise + browse + 7 s refresh; `Events()` unused (F6) |
| Discovery (Android) | OK (new) | browse + MulticastLock + self-filter; NSD advertise |
| Pairing / trust | **Solid** | SAS verified both directions, Ed25519 confirm signature, revoke check per attempt |
| Signaling contract | **Split brain** | daemon speaks DEC-022 `/session/*` JSON; the Android handshake speaks legacy `/offer` (B1) |
| Session (Linux-initiated) | OK | `engine.Session` + `pkg/receiver`: video + all 5 planes |
| Session (Linux-offered) | **Incomplete + wedgeable** | `InboundSession`: no video/input/notifications, no state monitor, no reaper (B3) |
| RTP/WebRTC transport | OK | ordered shaper, PSI cache survives reconnects, no NACK/TWCC by design |
| ICE | OK for LAN | no STUN/TURN; mDNS candidates disabled (deliberate) |
| Clipboard | OK mechanics | digest echo filter (32/5 s), reconnect sync, honest failure on Android |
| Transfer | OK | high/low watermark, queue depth 16, no-resume by design, partial sweep |
| Input | OK on Linux-initiated only | receiver uses `pc.OnDataChannel`; `InboundSession` has no handle |
| Notifications | OK on Linux-initiated only | same asymmetry |
| Android JNI boundary | 1 race + 1 churn | callbackObj race (B4), attach/detach per call (I6) |
| Linux IPC/UDS | OK, slow shutdown | auth/idempotency good; GracefulStop blocked (B2) |
| Session state machine | **Solid** | explicit transitions, typed reasons, generation guards, hub tokens |
| Health monitoring | Partial | `Health`/`MediaStats` exist; no liveness for inbound sessions or signaling |
| Timeouts/retries | Mostly good | bounded 15 s reconnect budget, 500ms→4s backoff; polling where callbacks exist (I2) |
| Backpressure | **Solid** | latest-wins hub, bounded AU queue, watermark gates, non-blocking fan-out |
| Resource cleanup | Partial | `Hub.Unsubscribe` dead code (B5); inbound session unreaped (B3) |
| Duplicate/replay | Adequate | content-digest echo filter; signaling requests are not idempotent-keyed (potential) |

---

## 4. Bottlenecks and inefficiencies

**I1 [Confirmed, high CPU impact] Eager ffmpeg MJPEG tap per session.**
`NewTapSink` spawns `go t.convertLoop()` at construction, and `StartSession` wraps every
sink in a tap whenever `hub != nil` — which the daemon always sets. So **every session
runs a second ffmpeg** (`-threads 1 -q:v 3 -f mjpeg`) transcoding the full stream even if
the Linux UI never opens the preview. ffmpeg/ffplay are present on the audited host.
Supporting journal data (cgroup totals incl. children):

| daemon run | wall | CPU | peak RSS | peak swap |
|---|---|---|---|---|
| Sep 26 (session-heavy) | 7 h 22 m | 5 m 32 s | 131.8 MB | 42.2 MB |
| Sep 27 (mostly idle) | 3 h 57 m | 6.8 s | 25.6 MB | — |

**I2 [Confirmed] Polling where Pion offers callbacks.** `gatherComplete` is a 5 ms spin
loop for up to 2 s in three places (`pkg/webrtc/session.go`, `engine/inbound_session.go`,
`pkg/receiver/receiver.go`); `Session.waitForTransportUp` and `webrtc.WaitForState` spin at
10 ms. `OnICEGatheringStateChange`/`OnConnectionStateChange` exist and are already used
elsewhere.

**I3 [Confirmed] Android 1 Hz stats push.** `STATS_INTERVAL_MS = 1000` posts the full
`collectStats()` map to Flutter forever while the UI is attached, and each tick calls
`AndroidClipboardAdapter.checkImeSelected()` → `Settings.Secure.getString` (ContentResolver
query) + `recomputeState()` — a *stats getter with a side effect* that can fire adapter
state transitions. Clipboard/capture transitions are already pushed via
`emitClipboardState()` and the service state listener.

**I4 [Confirmed, ratified] mDNS browse-session restart every 7 s from both devices.**
Small per-cycle cost, but continuous background multicast plus goroutine churn on the
phone, which now also browses under a `MulticastLock`. Ratified by DEC-007; see §8.

**I5 [Confirmed, minor] Flutter IPC resubscription uses fixed 500 ms/1 s delays** despite
the file header promising exponential backoff, with unbounded retries.

**I6 [Confirmed, minor] JNI attach/detach per clipboard host callback.** Every
`WritePlatformClipboard`/`SendClipboardUpdate`/`OnOversizedPayload` attaches and detaches a
JVM thread. Event-rate, not byte-rate, so negligible today, but avoidable.

**I7 [Potential] `AwaitDrain` 10 ms poll** behind `OnBufferedAmountLow`. Documented safety
net; acceptable.

**I8 [Confirmed, minor] Three paths for the same state.** Clipboard status reaches Flutter
via the 1 Hz stats map, `emitClipboardState()`, and the `getClipboardStatus` request —
and `getMediaStats` re-serves what the tick already pushed.

---

## 5. Confirmed bugs, races and resource leaks

### B1 [High — blocks the phone→PC direction] Android posts to a route the daemon does not serve

`MainActivity.initiateWebRtcHandshake` builds `"$receiverUrl/offer"` and posts the phone's
raw SDP offer there, expecting an SDP answer in the response body. The daemon serves a
different contract. Measured against the live daemon:

```
POST /offer          -> HTTP 404  (404 page not found)      <-- what the phone calls
POST /session/offer  -> HTTP 401  {"error":"missing authentication headers"}
POST /session/answer -> HTTP 401  {"error":"missing authentication headers"}
GET  /health         -> HTTP 200  {"status":"ok"}
POST /pairing/request-> HTTP 400  (empty body)
```

`/offer` (raw SDP in / answer out) is the legacy contract implemented by
`cmd/receiver/main.go`; the daemon implements DEC-022 `/session/*` JSON. Consequences:
the phone-side Connect/Share silently fails (`Log.w("Receiver returned HTTP 404")`), the
**phone keeps capturing locally**, the UI shows "Connecting to …"/sharing active because
`startCapture` returned `true` at consent time, and nothing about the failure reaches the
UI as a typed session error.

### B2 [High — shutdown] Graceful stop cannot complete while a frame stream is attached

`Serve` calls `closeAllSubscribers()` then `grpc.Server.GracefulStop()` with a 3 s timeout
and a hard `Stop()` fallback. `StreamFrames` is a long-lived server-stream RPC whose handler
parks in `frames.Hub.Subscribe(ctx)` until a session begins or the **client's** context ends.
That subscription is not registered in `s.subscribers`, so `closeAllSubscribers()` cannot
release it, and `GracefulStop` does not cancel stream contexts.

Measured (`core/pkg/localipc/shutdown_latency_test.go`, run in this audit):

| scenario | shutdown latency |
|---|---|
| no long-lived streams | **138.62 µs** |
| one StreamFrames client attached | **3.0024 s** (= the 3 s timeout) |

Production evidence, same mechanism:
`Sep 28 23:02:23 x1 phonebridge-daemon[2725]: graceful stop timed out; forcing stop`.

### B3 [High — latent wedge] `InboundSession` is incomplete and never reaped

`SessionManager.HandleInboundOffer` (Linux as SDP offerer) creates an `InboundSession` that:

* creates only `control`/`clipboard`/`transfer` — **no video track and no `OnTrack`**, so no
  screen stream can flow on this path;
* registers **no `pc.OnDataChannel`**, so the peer's `input`/`notifications` channels have no
  handle (Linux cannot send input, cannot receive mirrored notifications);
* sets `cfg.OnStateChange = nil` — no connection-state monitoring at all;
* is only ever closed by `HandleInboundStop` (a peer POST). If the peer dies without sending
  it, `m.inboundSess` stays non-nil with `IsClosed() == false` **forever**, so every later
  offer is rejected `SESSION_BUSY` until the daemon restarts. There is no idle timeout and no
  reaper. Only reachable today from another Linux node (B1 keeps the phone off this path),
  but it is a permanently-stuck state by construction.

Additionally `HandleInboundOffer` holds `m.mu` across `NewInboundSession` + `CreateOffer()`,
and `CreateOffer` waits up to 2 s for ICE gathering — so `StartSession`, `StopSession`,
`GetSessionState` and `ActiveSession` are all blocked for up to 2 s during an inbound offer.

### B4 [Medium — crash risk] Unsynchronised JNI global-reference teardown

`currentJniHost.callbackObj` is written under `clipboardHostMu` in
`nativeClipboardStop`/`nativeStop` (`DeleteGlobalRef` then `callbackObj = NULL`), while
`jniClipboardHost.WritePlatformClipboard`, `SendClipboardUpdate` and `OnOversizedPayload`
read `h.callbackObj` with **no lock**. `bridge.Stop()` sets `host = nil` first, but a call
already in flight holds `h`. A `CallBooleanMethod` on a just-deleted global ref is a JNI
use-after-free (SIGSEGV / `IllegalStateException`). Go's race detector cannot see a `C.jobject`
field, so the clean race run below does not cover it. Low frequency, real crash potential.

### B5 [Medium — leak] `frames.Hub.Unsubscribe` is dead code

`Hub.Unsubscribe` is called nowhere in production or tests. `StreamFrames` loops
`for f := range ch` with no `<-ctx.Done()` case, so a client that cancels mid-session leaves
its channel in `hub.subs` until the session ends: it keeps receiving fan-out and up to 2
buffered JPEGs per frame. Bounded, but it is a leak of both memory and work.

### B6 [Medium] `Discovery.Events()` has no consumer

`Discovery.Events()` (64-slot channel, allocated in `NewDiscovery`) is referenced nowhere in
production **or** tests; the registry still performs a non-blocking channel send on every
upsert/sweep. Meanwhile `Session.LocateTarget` polls the registry every 100 ms for up to 10 s
instead of being woken by it.

### B7 [Low] Nil-deref if discovery init fails

`SessionManager.StartSession` calls `m.discovery.Registry()` unguarded. The daemon only logs a
warning when discovery fails and leaves `m.discovery == nil`, so the first Connect would panic
(recovered at the gRPC boundary, surfacing as an opaque error).

### B8 [Low] Magic port literal

`resolveEndpoint` and `LocateAndConnect` fall back to the literal `7804` instead of
`engine.DefaultSignalingPort`.

### B9 [Low] Capability advertisement is inconsistent

Linux mDNS advertises `["SCREEN","CLIPBOARD"]` (no `FILES`) although the transfer engine is
running; Linux `signalingCapabilities = ["SCREEN"]`; Android NSD advertises
`screen,files,clipboard`. No consumer reads `Device.Capabilities` in the Flutter UI today, so
it is latent rather than breaking — but any future capability gate would be wrong.

---

## 6. Redundant or unnecessary communication

1. **Second ffmpeg per session** (I1) — pure waste when no preview is open.
2. **1 Hz full stats map** (I3) + `getMediaStats` on every refresh + `emitClipboardState`
   (I8) — three transports for one state.
3. **mDNS browse-restart churn every 7 s on two devices** (I4).
4. **`Discovery.Events()`** — allocated, written on every device upsert/sweep, never read (B6).
5. **`Hub.Unsubscribe`** — intended cancellation path never called (B5).
6. **Polling where Pion callbacks exist** (I2): 5 ms gather spins, 10 ms transport-state spins,
   100 ms discovery-registry poll.

---

## 7. Recommended optimizations, by impact and risk

| # | Optimisation | Impact | Risk | Evidence |
|---|---|---|---|---|
| 1 | Fix the phone→PC route: give the daemon a `/offer` compatibility route (raw-SDP in, answer out — preserving the existing contract exactly for `cmd/receiver`), **or** teach the Android handshake `/session/offer` + `/session/answer`; either way return a typed session error to the UI instead of logcat only | **High** (unblocks the direction) | Low–Med | B1, measured 404 |
| 2 | Harden `InboundSession`: register `OnConnectionStateChange`, close+free on Failed/Disconnected/Closed, add an idle timeout; drop `m.mu` before `CreateOffer()` | High | Med | B3 |
| 3 | Make shutdown prompt: track frame-stream subscriptions in the server and cancel them on shutdown; invert the characterization test to require <1 s | High | Low | B2, 3.0024 s measured |
| 4 | Lazy-start the MJPEG tap ffmpeg on the first `StreamFrames` subscriber, stop on the last | High (CPU) | Low | I1 |
| 5 | Replace gather/state polling with `OnICEGatheringStateChange`/`OnConnectionStateChange` | Med | Low | I2 |
| 6 | Make the Android stats push event-driven (capture/clipboard/transfer transitions only) and stop calling `checkImeSelected` from a getter | Med | Low | I3 |
| 7 | Call `Hub.Unsubscribe` from the `StreamFrames` cancellation path | Med | Low | B5 |
| 8 | Guard `callbackObj` with `clipboardHostMu` (or re-check under the lock around the call); cache JVM thread attachment | Med | Low | B4, I6 |
| 9 | Consume `Discovery.Events()` and make `LocateTarget` event-driven | Low | Low | B6 |
| 10 | Real exponential backoff + cap in `local_ipc_client.dart`; `DefaultSignalingPort` instead of `7804`; nil-guard `m.discovery` | Low | Low | I5, B7, B8 |

---

## 8. Changes that should NOT be made

* **Do not remove or weaken the mDNS browse-refresh (7 s)** or the DEC-007/DEC-022
  semantics without a new decision record. The refresh exists to defeat a measured
  staleness bug; the cheaper variant (refresh `LastSeen` on identical answers) is an
  engine change that should be ratified, not slipped in.
* **Do not add STUN/TURN or any WAN path** (DEC-008; explicitly out of scope).
* **Do not make the `control` DataChannel reliable.** `MaxRetransmits: 0` is deliberate:
  freshest stats win, stale samples are worthless.
* **Do not restart capture on reconnect** (DEC-020): re-binding an encoder to a live
  VirtualDisplay does not resume delivery, and a geometry change needs new consent.
* **Do not add NACK/TWCC interceptors** to the pre-encoded RTP path; the bare registry is
  spike-validated and adding buffering changes the packet contract.
* **Do not change SDP/JSON shapes or the `localipc` proto** — protocol compatibility is a
  hard constraint; the phone is authoritative for media params (DEC-020).
* **Do not "tidy" the transfer outbound queue (depth 16), the hub latest-wins policy, or
  the 768 KiB clipboard ceiling** — all three are load-bearing backpressure contracts.
* **Do not make the phone's clipboard write path force a write when the IME is not
  selected**; that would fabricate delivery the platform cannot honour (DEC-023).
* **Do not add an "Other" transport for clipboard/notifications** outside the WebRTC
  DataChannels; the per-channel independence is what keeps frame backpressure from
  costing control events.

---

## 9. Test / benchmark evidence

**Measured in this audit**

* Shutdown latency, new test `core/pkg/localipc/shutdown_latency_test.go`:
  `138.62 µs` (no streams) vs `3.0024 s` (one StreamFrames client) — the second is exactly
  the 3 s graceful-stop timeout.
* Live signaling route probe against the running daemon (results in B1).
* `go test -race -count=1 ./pkg/engine/... ./pkg/localipc/... ./pkg/frames/...` → all `ok`
  (6.785 s / 4.726 s / 1.390 s). Note this cannot cover B4 (C field).
* Live host state: daemon PID 2718 (15 threads, 16 fds, 23.4 MB RSS), UDS
  `/run/user/1000/phonebridge/engine.sock`, mDNS bound on `0.0.0.0:5353` and `[::]:5353`,
  signaling `*:7804`, `ipcdrv clipboard` → `AMBIENT_ACTIVE`/`READY`, `ipcdrv session` →
  `DISCONNECTED`, `ipcdrv devices` → one LAN peer.
* Clipboard adapter startup convergence (journal): `NO_BACKEND → STARTING → NO_BACKEND →
  STARTING → READY` across 4 helper probes in ~3 s; then stable.
* Journal CPU/RSS table in I1.

**Not measured (no device attached during this audit — `adb devices` empty)**

* On-device cost of the 1 Hz stats tick and the `MulticastLock`/browse on the phone.
* The ffmpeg tap's real CPU share inside a live session (needs a streaming phone).
* End-to-end throughput and the phone-sleep / network-interruption recovery paths.

---

## 10. Final assessment

**Security and authorization: production-ready.** Ed25519-signed signaling per request,
trust-store checks re-evaluated on every reconnect, SAS verified in both pairing directions,
UDS `SO_PEERCRED` UID gate plus constant-time bearer token with 0700/0600 paths, validated
and rate-limited input frames, validated notification frames, explicit payload ceilings.

**Concurrency and backpressure: good.** Bounded queues everywhere, latest-wins frame policy,
non-blocking control fan-out, generation-guarded transport callbacks, hub session tokens that
prevent cross-session stalls, and a bounded reconnect budget with a real backoff schedule.

**Efficiency: mixed.** The media plane does no redundant per-packet work and the shaper is
serialised correctly; but the daemon spends an entire ffmpeg transcode per session that is
usually unwatched, spins on polls where Pion has callbacks, and the phone pushes a full state
map every second.

**Connectivity: not production-ready as a bidirectional system.**

* The **Linux→phone** direction works and is what the frozen beta exercises.
* The **phone→PC** direction **cannot connect at all**: it posts to a route the daemon does
  not serve (measured 404), and the failure is invisible in the UI.
* The daemon **cannot shut down promptly** while the Linux UI holds `StreamFrames`
  (measured 3.0024 s, with a production journal line), so every `systemctl --user restart`
  degrades to a hard stop.
* The **inbound-session** path is incomplete (no video/input/notifications) and can wedge
  permanently on `SESSION_BUSY` with no reaper.

Priority order: **B1 → B3 → B2 → I1 → I3**. B1 and B3 are correctness; B2 and I1 are
lifecycle/CPU; everything else is hygiene. None of the fixes requires a protocol change,
and none requires touching the ratified security or consent model.
