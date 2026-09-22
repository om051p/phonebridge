# Spike 08 Results: Bidirectional Large-File Transfer (Android ↔ Linux)

> **Status:** `COMPLETE` · **Classification:** `VALIDATED`
> **Author:** Senior PhoneBridge Integration & Release Engineer
> **Date:** 2026-09-23
> **Prerequisites:** Spike 04 (Pion WebRTC transport, DEC-021), Spike 09 (LAN discovery, DEC-022), Phase 3 clipboard DataChannel (DEC-023)
> **Target Decision:** DEC-024 (File-transfer transport, protocol & storage)
> **Commits:** `be6beff` (Phase 4 transfer implementation) · `adc846a` (Linux mDNS browse refresh) · `0c4d78a` (Android NSD registration recovery)
> **Framework doc:** [08-bidirectional-large-file.md](08-bidirectional-large-file.md) — `ABSORBED by Phase 4` (DEC-024)

---

## 1. Executive Summary

Spike 08 was never executed as a standalone prototype: its questions were settled directly as a ratified design in **DEC-024** and implemented in `core/pkg/transfer`, the `transfer` WebRTC DataChannel, and the Android `AndroidTransferHost` / Linux `FileDestination` storage seams. This document records the **physical end-to-end evidence** for that implementation on real hardware, in the same role the other `*-results.md` docs play for Spikes 01–04.

### Core verdict

Bidirectional file transfer **works end-to-end on real hardware with byte-exact integrity in both directions**. A 1 MiB payload was transferred Android → Linux and Linux → Android, and the destination bytes hash identically to the source bytes on the opposite machine in every case recorded here. The Android receive path completed the MediaStore `IS_PENDING` lifecycle (pending insert → streamed write → verification → commit) with **no pending or partial rows left behind**, and the Linux receive path promoted a staged partial into its final name with **no `.phonebridge-partial` payload remaining**.

### Evidence provenance (read this before quoting any number)

| Class | Meaning |
|---|---|
| **A — re-verified** | Re-measured on 2026-09-23 directly from artifacts still present on the host/device, or read from the shipped source/daemon. Every hash, size and path in §3, §7 and §8 is Class A. |
| **B — recorded, not re-verifiable** | Observed during the Phase 4 validation run and preserved in the session record, but the artifact that carried the proof no longer exists on either machine (or lived only in logcat). **No uploaded evidence document was reachable in the environment that produced this record**, so Class B items are reproduced here as previously observed and are labelled as such — none of them is used to support a "verified" claim in §3. |
| **C — unavailable** | Never captured, or capture was attempted and the source markdown could not be retrieved. Timing/throughput figures in particular are **not** recorded as measurements. |

The Phase 4 prompt that framed this work carried a pre-recorded digest, `20ab2cb…`, described as the SHA-256 of a "1 MB" `/tmp/e2e_a2l.txt`. Repository and on-disk evidence shows that prefix belongs to the **62-byte** `e2e_a2l.txt` payload; the **1 MiB** payload is `adf4927e8196355b…`. Both are recorded separately in §3 rather than merged.

---

## 2. Exact Test Environment

| Property | Value | Source / verification |
|---|---|---|
| **Host OS** | Ubuntu 26.04.1 LTS (Pop!_OS lineage; `zwlr_data_control`/COSMIC gating per Spike 06 does not apply to transfer) | `/etc/os-release` (`PRETTY_NAME`) |
| **Host kernel** | `7.0.0-31-generic` | `uname -r` |
| **Go toolchain** | `go1.24.13 linux/amd64` | `go version` |
| **Repo HEAD at capture** | `0c4d78a` (`fix(android): recover lost mDNS registration`) | `git rev-parse --short HEAD` |
| **Linux daemon** | `~/.local/bin/phonebridge-daemon`, 26,293,302 bytes, built 2026-09-22 23:59 | `ls -la`; running as user unit `phonebridge.service` (`active`) |
| **Daemon receive config** | `File transfer: receiving into /home/rp-x1/Downloads (max 17179869184 bytes)` | startup log line; matches DEC-024's 16 GiB default |
| **Android device** | `23049PCD8I` (POCO F5) — the **only** Android device used | `getprop ro.product.model` |
| **Android OS** | Android **15**, API **35** | `getprop ro.build.version.release`, `ro.build.version.sdk` |
| **Android package** | `dev.phonebridge.spike04` | install/instrumentation runs |
| **Android diagnostics** | USB ADB serial **`89ceabd9`** (used for all shell/logcat during this phase) | `adb devices` |
| **Android↔Linux traffic** | Wi-Fi LAN (secondary Wi-Fi ADB endpoint `192.168.0.125:41141` used earlier in the phase) | session history |
| **Pixel 4 XL** | not attached, not touched | — |
| **DEVICE_ID (Android)** | `c121b1702bd33c2a8fceb54ee7c70ea7a8a222f797712ce43c97cfdc1fb1484e` | `ipcdrv devices` (listed and non-stale at capture) |
| **Linux identity** | `aa67e88a629d12dd914f64da225b74946cd66ce7fd6970ebc0423a00679f2b8a` | pairing record |

### 2.1 Capture-time session state (Class A)

`ipcdrv session` at capture returned the **last** session, `0ca27130b260914f`, in state `SESSION_STATE_FAILED` with `SESSION_REASON_RECONNECT_TIMEOUT` (“reconnect budget 15s exhausted after 10 attempt(s)”), requesting 720×1600 @30 fps. No live session was active while this record was written (screen mirroring was not running). This is the *terminal* state of the discovery-validation session, not a transfer failure: the device remained discoverable (`ipcdrv devices` returned the POCO) and the transfer plane's health at the time of the transfer runs is recorded in §4.

Two session identifiers therefore appear in Phase 4 records and must not be conflated:
- `99d515c8f77d7800` — the original pairing/E2E session from the Phase 4 acceptance run.
- `0ca27130b260914f` — the session used for the post-stale-window discovery validation (`adc846a`), whose transfer DataChannel was exercised by the 1 MiB Linux→Android transfer in §3.

---

## 3. Deterministic Payloads & Integrity Matrix

Payloads are deterministic files produced once and placed on both sides, so a matching digest proves byte integrity of whatever landed while the **distinct source and destination paths** establish direction.

### 3.1 Re-verified hash matrix (Class A — all four hashes re-measured 2026-09-23)

| # | Direction | Source (path, size) | Source SHA-256 | Destination (path, size) | Destination SHA-256 | Verdict |
|---|---|---|---|---|---|---|
| 1 | **A → L** | `/data/local/tmp/e2e_a2l_1m.bin` (1,048,576 B) on the POCO | `adf4927e8196355ba3d894e721448e522a6acbcc5a965a7097914d2f02873c9c` | `/home/rp-x1/Downloads/e2e_a2l_1m.bin` (1,048,576 B) | `adf4927e8196355ba3d894e721448e522a6acbcc5a965a7097914d2f02873c9c` | **MATCH** |
| 2 | **L → A** | `/tmp/e2e_a2l_1m.bin` (1,048,576 B) on the host | `adf4927e8196355ba3d894e721448e522a6acbcc5a965a7097914d2f02873c9c` | `/sdcard/Download/e2e_a2l_1m.bin` (1,048,576 B) | `adf4927e8196355ba3d894e721448e522a6acbcc5a965a7097914d2f02873c9c` | **MATCH** |
| 3 | **L → A** | `/tmp/e2e_a2l.txt` (62 B) on the host | `20ab2cb09768e2682139ad42f01f96b6a216db145818eb5101bdf7fec433a3a8` | `/sdcard/Download/e2e_a2l.txt` (62 B) | `20ab2cb09768e2682139ad42f01f96b6a216db145818eb5101bdf7fec433a3a8` | **MATCH** |
| 4 | **L → A** | `/tmp/nsd_recovery_check.txt` (23 B) on the host (discovery-validation payload, `adc846a`) | `506770523887c9f36c82ffa68f4bb9f48dda94d92d901fb0dc64fc4e387d6973` | `/sdcard/Download/nsd_recovery_check.txt` (23 B) | `506770523887c9f36c82ffa68f4bb9f48dda94d92d901fb0dc64fc4e387d6973` | **MATCH** |

Row 1 is the load-bearing **Android → Linux** proof: the source lives in the phone's app-accessible staging area (`/data/local/tmp`, not the Downloads collection, so it cannot be confused with a receive), and the destination is the Linux daemon's configured receive directory.

### 3.2 Payload inventory (Class A)

| Location | File | Size | Timestamp | Note |
|---|---|---|---|---|
| Host `/tmp` | `e2e_a2l.txt` | 62 B | 2026-09-22 | deterministic source |
| Host `/tmp` | `e2e_a2l_1m.bin` | 1,048,576 B | 2026-09-22 | deterministic source |
| Host `/tmp` | `nsd_recovery_check.txt` | 23 B | 2026-09-23 | deterministic source |
| Host `~/Downloads` | `e2e_a2l_1m.bin` | 1,048,576 B | 2026-09-22 23:06 | **A→L receipt**, mode `0600` |
| POCO `/data/local/tmp` | `e2e_a2l.txt` | 62 B | 2026-09-22 20:43 | A→L source (send side) |
| POCO `/data/local/tmp` | `e2e_a2l_1m.bin` | 1,048,576 B | 2026-09-22 20:43 | A→L source (send side) |
| POCO `/sdcard/Download` | `e2e_a2l.txt` | 62 B | 2026-09-22 22:30 | L→A receipt |
| POCO `/sdcard/Download` | `e2e_a2l_1m.bin` | 1,048,576 B | 2026-09-22 22:31 | L→A receipt |
| POCO `/sdcard/Download` | `e2e_a2l.txt (1)` | 62 B | 2026-09-22 22:49 | L→A receipt (collision-renamed) |
| POCO `/sdcard/Download` | `e2e_a2l_1m (1).bin` | 1,048,576 B | 2026-09-23 00:03 | L→A receipt (collision-renamed) |
| POCO `/sdcard/Download` | `nsd_recovery_check.txt` | 23 B | 2026-09-23 00:22 | L→A receipt |

The two `(1)` receipts are byte-identical to their originals (`20ab2cb09768e268…`, `adf4927e8196355b…`, both full size). Repeated transfers of the same filename therefore **did not overwrite** the earlier receipt — the receiving side allocated a fresh, collision-renamed destination, consistent with DEC-024's collision policy observed on the Android side.

### 3.3 Class B — previously observed, artifact no longer present

| Claim | Status |
|---|---|
| 62 B **A → L** receipt in the host Downloads directory | **Class B.** The host now holds only the 1 MiB A→L receipt; the 62 B host-side receipt was removed during Phase 4 cleanup. The 62 B direction is instead proven **L → A** in §3.1 row 3. |
| 256 MB interruption/retry-from-zero run | **Class B.** No 256 MB artifact remains on either machine. |
| Cancellation runs in both directions | **Class B.** A cancelled transfer by design leaves no receipt; see §5. |

---

## 4. Transfer Lifecycle

DEC-024 fixes the wire sequence, and `docs/protocol.md` states it identically:

```
FileOffer → FileAccept → FileChunk×N → FileComplete → FileResult
                 \___ FileCancel (either side, after the offer) ___/
```

Observed on hardware during Phase 4 validation (**Class B** unless noted):

1. **Offer → Accept.** The receiving side accepted after the free-space preflight (size + 256 MiB margin) and filename sanitisation; the sender never pre-hashed (`FileOffer.sha256_digest` is intentionally empty per DEC-024, so the source is read once).
2. **Chunk×N.** 64 KiB frames over the dedicated reliable/ordered `transfer` DataChannel, with sender backpressure at the 1 MiB high-watermark / 256 KiB low-watermark. The receiver validated `chunk_index` contiguity and `offset == index × chunk_size` (`CODE_INVALID_ARGUMENT` otherwise).
3. **Complete → Result.** `FileComplete` carried bytes-sent + SHA-256; the receiver verified the digest **before** promotion and then emitted exactly one `FileResult`.
4. **Terminal state.** `TRANSFER_STATE_COMPLETE` for rows 1–4 of §3.1 (the receipts and MediaStore rows in §7 are the durable consequence of that terminal state).

**DataChannel.** The `transfer` channel is created by the same offerer that creates `control`/`clipboard` (`webrtc.Session` on Android, `engine.InboundSession` on Linux) and accepted by `receiver.Receiver`. Its presence on the session that followed the stale window (session `0ca27130b260914f`, phone app not restarted) was confirmed by the 1 MiB L→A transfer in §3.1 row 2 completing (**Class A** for the artifact; the channel-creation log line itself is Class B). Capability signalling is the channel's presence; mDNS `files`/`FILES` text remains a UI hint only.

---

## 5. Cancellation

`FileCancel` is available to either side after the offer, and the engine terminates with exactly one `FileResult` **or** `FileCancel`; `CODE_TRANSFER_CANCELLED` is the typed terminal code.

- **Physical result (Class B):** cancellation was exercised in both directions during the Phase 4 acceptance run and reported as PASS, with no receipt produced and no partial retained.
- **Artifact-level consequence (Class A):** no cancelled artifact exists anywhere at capture time — the host staging directory holds **zero** payloads (§8.2) and Android holds **zero** pending/partial names (§7.2). A cancelled transfer that had leaked a staging file would be visible as a `.part` or an `is_pending=1` row; neither is present.
- **Deterministic coverage (Class A, repository):** `core/pkg/transfer/pair_test.go` (`CancelBySenderMidTransfer`, `CancelByReceiverMidTransfer`, `ChannelLossInterruptsAndCleansUp`), `core/pkg/transfer/engine_test.go` (`CloseInterruptsEveryActiveTransfer`), `core/pkg/webrtc/transfer_loopback_test.go` (channel-level loopback transfer), and the Flutter-side `ui/test/transfer_channel_test.dart` / `transfer_controller_test.dart`.

---

## 6. Interruption & Retry-From-Zero

DEC-024 is explicit that Phase 4 has **no resume**: a lost DataChannel or session aborts in-flight transfers with `CODE_TRANSFER_INTERRUPTED`, deletes partials, and requires an explicit retry under a new transfer id.

- **Physical result (Class B):** a 256 MB transfer was interrupted by disrupting the phone's network path and reported to produce `INTERRUPTED` behaviour, with a subsequent retry restarting from **zero bytes** and completing. USB ADB remained available throughout for diagnostics.
- **Deterministic coverage (Class A, repository):** `core/pkg/transfer/engine_test.go` (`TestAttachChannelReplacementInterrupts`), `core/pkg/transfer/violations_test.go` (`TestInbound_PeerCancelAbortsAndCleansUp`), and the staging-sweep tests in `core/pkg/transfer/destination_test.go`. Resume metadata is deliberately absent from the protocol; adding it would be a protocol change, not a bug fix.
- **Reconnect policy (Class A, observed at capture):** the session layer enforces a bounded reconnect budget — `0ca27130b260914f` ended with `SESSION_REASON_RECONNECT_TIMEOUT` after 10 attempts / 15 s. Transfers do not survive that; they interrupt and clean up.

---

## 7. Android Receive: MediaStore `IS_PENDING` Lifecycle

DEC-024: Kotlin inserts a `MediaStore.Downloads` item with **`IS_PENDING=1`**, hands the Go engine the write FD (`detachFd()`), and clears the pending flag **only** on verified commit (app-scoped fallback for API 26–28).

### 7.1 Committed rows (Class A — queried 2026-09-23)

`content query --uri content://media/external/downloads` returned, for every Payload-08 file:

| `_id` | `_display_name` | `_size` | `is_pending` | `relative_path` |
|---|---|---|---|---|
| 9993 | `e2e_a2l.txt` | 62 | **0** | `Download/` |
| 9994 | `e2e_a2l_1m.bin` | 1,048,576 | **0** | `Download/` |
| 9999 | `e2e_a2l.txt (1)` | 62 | **0** | `Download/` |
| 10001 | `e2e_a2l_1m (1).bin` | 1,048,576 | **0** | `Download/` |
| 10002 | `nsd_recovery_check.txt` | 23 | **0** | `Download/` |

Every row is committed (`is_pending=0`) with a size matching the source exactly, and every file is visible in the Downloads collection — pending creation → streamed write → digest verification → commit completed for all five receives.

### 7.2 No pending residue

- `ls /sdcard/Download/ | grep -iE "pending|partial|-part|\.tmp"` → **none**.
- The MediaStore query returned **no** row with `is_pending=1` (all five `e2e`/`nsd` rows are `0`).
- The two rows named `… (1)` show that a repeat insert of a colliding name is resolved by MediaStore renaming rather than overwriting a committed receipt, and that the renamed item is *also* committed — not left pending.

Unit-test coverage of this seam is repository-side: `GoBridgeTransferTest` (`android/app/src/test/kotlin/dev/phonebridge/bridge/GoBridgeTransferTest.kt` — name/MIME sanitisation, single FD hand-off, commit returning the display name, idempotent abort, free-space preflight, oversized frames). There is **no** on-device `androidTest` for the MediaStore lifecycle (see §10 CONDITIONAL 3).

---

## 8. Linux Receive: Staging → Promote → Cleanup

### 8.1 Staging-then-promote (Class A)

`core/pkg/transfer/destination.go` creates the staging directory as `filepath.Join(destinationDir, ".phonebridge-partial")` with mode `0o700` (`partialDirName = ".phonebridge-partial"`), writes `<transfer_id>.part` with mode `0o600`, and `os.Rename`s it into the destination only after the receiver has verified the digest (same filesystem ⇒ atomic).

Observed directly at capture:

- `~/Downloads/.phonebridge-partial/` exists with mode **`drwx------`** — exactly the `0o700` staging directory created by `NewFileDestination`.
- The promoted receipt `~/Downloads/e2e_a2l_1m.bin` carries mode **`-rw-------`** (`0o600`) — the mode with which `Begin()` created the staged partial. A rename preserves it, so the receipt is demonstrably *the staged file moved*, not a re-copied file.

### 8.2 No partial remains (Class A)

- `find ~/Downloads/.phonebridge-partial -mindepth 1` → **empty** (zero entries).
- `ls ~/Downloads | grep -iE "partial|staging"` → only the (empty) staging directory.
- Startup sweep is exercised by `destination_test.go` (`SweepRemovesStalePartials`, `KeepsStagingInsideDestinationFilesystem`), and the daemon calls `SweepPartial()` at start.

**Nuance recorded deliberately:** the staging *directory* is retained (it is `MkdirAll`'d at destination construction); what must not remain is any *payload*, and none does. The empty directory is therefore expected behaviour, not residue.

### 8.3 Discrepancy found against DEC-024 (documentation, not implementation)

DEC-024 originally described the Linux staging path as `<XDG_DOWNLOAD_DIR>/PhoneBridge/.phonebridge-partial/<transfer_id>.part`. The **shipped implementation and the live daemon disagree with that text in two ways** (Class A):

1. **No `PhoneBridge/` sub-directory.** The destination directory is `DefaultDownloadDir()` (`$XDG_DOWNLOAD_DIR` → `~/.config/user-dirs.dirs` → `~/Downloads` → temp fallback), and the staging directory is created *inside it* — confirmed by the daemon startup log (`receiving into /home/rp-x1/Downloads`) and by the observed `~/Downloads/.phonebridge-partial`.
2. Consequently the receipt lands directly in `~/Downloads`, not `~/Downloads/PhoneBridge/`.

The implementation is self-consistent, ships with tests, and matches the comment in `destination.go` (“the staging directory created inside the destination”); only the decision record's path text was stale.

**Resolved 2026-09-23 — DEC-024's storage sentence was corrected** in `docs/decisions.md` to read `<download dir>/.phonebridge-partial/<transfer_id>.part`, with the reason recorded inline. Basis for treating it as a stale *planned* path rather than a contract the code violated:

- `git log -S"PhoneBridge" -- core/pkg/transfer/destination.go core/cmd/daemon/main.go` returns **no commit** — no version of the transfer storage code ever contained the folder.
- The daemon's own flag help defines the receipt directory as `$XDG_DOWNLOAD_DIR` or `~/Downloads` (`--download-dir`, overridable by `PHONEBRIDGE_DOWNLOAD_DIR`), and `DefaultDownloadDir()` never appends a component.
- The physically validated Phase 4 receipts landed directly in `~/Downloads` (§3.2), and the Android side writes to `Download/` with no grouping folder (§7.1) — so the folder existed only in the decision text.
- Choosing the other interpretation — implementing the folder — would change shipped, physically validated behaviour and would therefore be a **new decision**, not a correction.

---

## 9. Timing, Throughput & Byte-Offset Measurements

**Unavailable (Class C).** No per-chunk timing, throughput, or byte-offset trace was retained: the Phase 4 run captured lifecycle/state evidence and digests, not a latency histogram, and no timing artifact exists on either machine at capture. Nothing in this document should be read as a throughput measurement. DEC-024's operational bounds — 64 KiB chunks, 1 MiB/256 KiB sender watermarks, 1 MiB buffered writes, 30 s offer/chunk-stall timeouts, max(60 s, 2×size at 100 MB/s) complete→result timeout — are **design constants**, not measurements from this run.

Likewise, the 16 GiB maximum and the free-space margin appearing in §2/§7 are configured limits, not observed transfers.

---

## 10. Classification of Findings

### **PROVEN (Class A — re-verified at capture)**
1. **Android → Linux integrity:** 1 MiB source on the phone and 1 MiB receipt on the host have identical SHA-256 (`adf4927e…`).
2. **Linux → Android integrity:** three payloads (1 MiB, 62 B, 23 B) have identical SHA-256 on the host source and the Android receipt.
3. **MediaStore `IS_PENDING` lifecycle:** five committed Downloads rows, all `is_pending=0`, sizes exact, all visible.
4. **No Android pending/partial residue.**
5. **Linux staging-then-promote:** `0o700` staging dir + `0o600` promoted receipt prove the rename, and the staging directory contains zero payloads.
6. **Collision handling on repeat receive:** byte-identical `… (1)` receipts, no overwrite, both committed.
7. **Deterministic test coverage** for lifecycle, cancellation, interruption/cleanup, staging sweep and sanitisation (`core/pkg/transfer`, `core/pkg/webrtc`, `ui/test/*transfer*`).

### **PROVEN (physical, Class B — observed during the Phase 4 run)**
8. Full lifecycle offer → accept → chunk×N → complete → result on real hardware, both directions.
9. Cancellation in both directions terminating cleanly with no residue.
10. 256 MB interruption producing `INTERRUPTED` behaviour and a retry-from-zero.
11. `transfer` DataChannel open on a session established **after** the discovery stale window.

### **CONDITIONAL**
1. **Repeat evidence beyond 1 MiB.** Every re-verifiable artifact is ≤ 1 MiB. The large-file behaviour (16 GiB ceiling, backpressure under sustained load, GB-scale streams) rests on design plus the Class B 256 MB run, not on a surviving artifact.
2. **Android send path is SAF-driven.** DEC-024 records Android *send* using SAF `ACTION_OPEN_DOCUMENT` + read-FD handoff; the A→L test bytes came from `/data/local/tmp` rather than a user-picked SAF document, so the SAF picker/fd hand-off is covered by unit tests, not by this physical run.
3. **MediaStore lifecycle has no on-device instrumented test.** Coverage is unit-level plus the committed rows above.

### **NOT PROVEN / DEFERRED**
1. **Resume.** Phase 4 is restart-from-zero by decision; no resume metadata exists in the protocol.
2. **Data-only / capture-free session mode.** Transfers require an active session (piggybacking on it, like clipboard); the deferred mode is a follow-up decision.
3. **SAF “Save as” / drag-and-drop / Share Sheet hooks.** Stubbed or deferred per DEC-024.
4. **Remote (non-LAN) transfer.** Depends on Spike 10 (TURN/ICE), which is unbuilt.

### **BLOCKED**
- None for the LAN transfer scope. The **mDNS discovery** defects that blocked session start were fixed in `adc846a` (Linux browse refresh) and `0c4d78a` (Android registration recovery) and are recorded separately; the Spike 09 fallback strategy (discovery when multicast is unavailable) remains unimplemented.

---

## 11. Architectural Implications for DEC-024

1. **Keep the staging-then-rename design.** Atomic same-filesystem `os.Rename` after digest verification is what makes a torn transfer impossible to observe as a completed file; the `0o600`/`0o700` modes are the trace that this is the live path.
2. **DEC-024 path text corrected 2026-09-23** to match `DefaultDownloadDir()` + `.phonebridge-partial` inside it, so the record and the code agree before Phase 5 builds on either.
3. **Treat restart-from-zero as a contract, not a limitation.** The retry-after-interruption path must allocate a **new transfer id**; any future resume work is a protocol change requiring a decision.
4. **MediaStore commit is the Android completion signal.** A receipt is only real once `is_pending=0`; the receiver must never clear pending before verification.
5. **Discovery health is a transfer prerequisite.** Both discovery defects surfaced as “transfer cannot start”, which is why the session layer's bounded reconnect (`SESSION_REASON_RECONNECT_TIMEOUT`) must be reported distinctly from `TRANSFER_INTERRUPTED` in any user-facing status work (Phase 5 “connection status”).

---

## 12. Evidence Boundaries & Limitations

1. **The uploaded evidence markdown was not retrievable.** This record was built from artifacts on the host and device, the shipped source, the live daemon, and the session record. Items whose proof artifact no longer exists are labelled Class B and are not used to support the “MATCH” verdicts in §3.
2. **Hashes are content hashes.** Because the payloads are deterministic and present on both sides, identical digests prove byte integrity of what landed; direction is established by the distinct source/destination paths, not by the digest alone.
3. **No timestamps of the transfer runs themselves.** File mtimes reflect when the file was written, which is used here only as ordering evidence.
4. **No timing/throughput data** (§9).
5. **Sessions are not live at capture.** The session listed by `ipcdrv` is terminal (`SESSION_REASON_RECONNECT_TIMEOUT`); nothing in §3 required an active session, since every claim was re-measured from artifacts on disk.
6. **Instrumented device tests needed `-t --user 0`** on this MIUI build; normal `androidTest` installs are blocked interactively.

---

## 13. Summary Status

- **Spike 08 (Bidirectional Large-File Transfer):** `COMPLETE` · **`VALIDATED`** — bidirectional transfer with byte-exact integrity proven in both directions, MediaStore `IS_PENDING` lifecycle completed with no residue, and staging→promote verified on the Linux side.
- **DEC-024:** implementation matches the decision's transport, integrity, limits and no-resume semantics; its Linux staging-path text was **corrected 2026-09-23** to `<download dir>/.phonebridge-partial/<transfer_id>.part` (there is no `PhoneBridge/` sub-directory).
- **Open items:** resume (deferred), data-only sessions (deferred), SAF “Save as” / drag-and-drop (deferred), Spike 09 discovery fallback (unimplemented), Android-side instrumented MediaStore coverage (missing).
