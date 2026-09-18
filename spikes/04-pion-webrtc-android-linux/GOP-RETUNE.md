# Spike 04 — Encoder GOP retuning for prediction-safe 30 fps delivery

Isolated experiment (no production code, no protocol changes, no decision-doc
changes). All evidence under `results/`.

## Question

`KEY_MAX_FPS_TO_ENCODER` is accepted by the encoder and ignored, so the hardware
encoder always runs at the panel rate (~119 fps). Throttling therefore has to
happen by dropping access units after encoding. GOP-tail dropping (keep a
contiguous prefix after each IDR) is prediction-safe but inherits the GOP length
as its judder period (~384 ms at the default 60-AU GOP).

Can the GOP be retuned so the freeze shrinks while prediction correctness holds?

## Method

`GOP_aus = KEY_I_FRAME_INTERVAL × KEY_FRAME_RATE` on this encoder, while
`KEY_FRAME_RATE` does **not** throttle the actual encode rate. So the pair is a
pure GOP lever.

For each configuration: capture a **full-rate, undropped** reference (throttle
`none`), recording an AU-boundary index, then derive every strategy from **that
same bitstream** on the host. Content is a function of `elapsedRealtime()` and is
not phase-stable across captures, so single-source derivation is what makes frame
alignment exact and the comparison valid.

Correctness is decided by per-frame SSIM against the reference decode of the
*same* source frames. H.264 reconstruction is deterministic given identical slice
data and references, so **SSIM == 1.0 exactly is a proof that the predictive chain
was intact**, and any frame below 1.0 is a concrete prediction error. "ffmpeg
decoded the stream" is not treated as evidence: the negative control below shows
ffmpeg silently concealing 93% of frames with zero error output.

## 1. Encoder parameters and observed GOP structure

| Variant | `KEY_FRAME_RATE` | `KEY_I_FRAME_INTERVAL` | expected GOP | **actual GOP** | GOP ms | IDRs | encoded fps | encoded kbps |
|---|---|---|---|---|---|---|---|---|
| A baseline | 30 | 2 | 60 | **60** (59×60 + 46) | 500 | 60 | 119.74 | 2501.3 |
| B "truthful" | 120 | 2 | 240 | **240** | 2004 | 10 | 119.78 | 2393.9 |
| C | 15 | 1 | 15 | **15** (uniform) | 125 | 95 | 119.33 | 2501.6 |
| D | 30 | 1 | 30 | **30** | 250 | 48 | 119.83 | 2484.0 |

The model holds exactly, and setting `KEY_FRAME_RATE` to 15 or 120 did not change
the actual encode rate — it is a GOP-only lever. All four streams are **I+P only
with zero reordered packets** (decode order == display order), which is what makes
contiguous-prefix dropping safe.

## 2. Delivered cadence and prediction correctness (tail rule, keep ≈ GOP/4)

| Variant | GOP AUs | keep | delivered fps | **SSIM** | imperfect | err | motion p50 | **freeze p50** | judder/s | IDR %bits | mean P bytes |
|---|---|---|---|---|---|---|---|---|---|---|---|
| A | 60 | 15 | 30.05 | **1.0** | 0/900 | 0 | 117.1 ms | **384.2 ms** | 2 | 11.1% | 2362 |
| B | 240 | 60 | 30.06 | **1.0** | 0/600 | 0 | 493.4 ms | **1510.9 ms** | 0.5 | 2.9% | 2437 |
| C | 15 | 4 | 31.82 | **1.0** | 0/380 | 0 | 25.0 ms | **100.3 ms** | 8 | 29.3% | 1985 |
| D | 30 | 8 | 32.15 | **1.0** | 0/384 | 0 | 58.8 ms | **191.9 ms** | 4 | 19.0% | 2171 |

### Formulas (measured)

```
motion_p50 ≈ (keep - 1) × frame_interval      # 8.33 ms at ~119 fps
freeze_p50 ≈ GOP_ms × (1 - keep / GOP_AUs)    # 0.75 × GOP_ms when keep = GOP/4
judder_cycles_per_s = 1000 / GOP_ms
```

So the freeze is a **pure function of GOP length**: `freeze ≈ 0.75 × GOP_ms`,
confirmed across a 16× range of GOP sizes (0.754–0.802).

### Negative control — "decoded" ≠ "prediction-correct"

Naive 1-of-4 striding on the same four captures, **all with zero decoder error
lines**:

| Variant | stride SSIM | imperfect frames | worst frame |
|---|---|---|---|
| A | 0.950417 | **837/897** (93.3%) | 0.787 |
| B | 0.896174 | **588/598** (98.3%) | 0.701 |
| C | 0.979943 | **285/380** (75.0%) | 0.927 |
| D | 0.968522 | **334/382** (87.4%) | 0.898 |

ffmpeg concealed every one of those frames and logged nothing. On variant A the
perfect-frame count was exactly 60 — precisely the IDR count — i.e. **every
non-self-contained frame striding kept was corrupted**. Striding degrades as the
GOP grows (0.896 at 240-AU GOP), because more P-frames reference dropped frames.

## 3. In-app behaviour (device JSON)

| Variant | exp GOP | encoded fps | keyframes | kbps | lat p50 | lat p90 | **lat p99** | CPU %1core | queue max | queue drops |
|---|---|---|---|---|---|---|---|---|---|---|
| A | 60 | 119.45 | 40 | 2485.9 | 23.5 | 27.9 | 34.2 | 93.8 | 4/256 | 0 |
| B | 240 | 119.55 | 10 | 2396.4 | 18.4 | 23.4 | 26.2 | 93.4 | 7/256 | 0 |
| C | 15 | 118.75 | 95 | 2496.5 | 21.1 | 27.1 | **83.1** | 95.0 | 14/256 | 0 |
| D | 30 | 119.22 | 48 | 2478.5 | 21.2 | 27.1 | 31.0 | 92.5 | 8/256 | 0 |

Signaling/ICE succeeded on B, C and D. Encoded fps is unchanged across variants
(118.75–119.55) — retuning never reduced encoder load. CBR holds. The Go queue
never approaches its cap and drops nothing.

## 4. Conclusion

**GOP retuning materially improves the product cost.** Holding SSIM at exactly 1.0,
cutting the GOP from 60 to 15 access units reduces the freeze from **384 ms to
100 ms (3.8×)** and the judder period from 500 ms to 125 ms.

Two measured costs:
1. **Bitrate.** At fixed CBR, I-frames take 29.3% of the bit budget at GOP 15 vs
   11.1% at GOP 60, shrinking the per-P-frame budget by ~16% (2362 → 1985 B).
   Quality per frame drops at the same total bitrate.
2. **Latency tail.** p99 encode latency rises from ~34 ms to **83 ms** with 8× more
   IDRs/s; p50 is unaffected (~21 ms).

The "truthful" configuration (B) is the **worst** option: being honest that the
encoder runs at 120 fps quadruples the GOP to 2 s and produces a 1.5 s freeze.

A GOP around 15–30 AUs is the sweet spot: freeze 100–192 ms with p99 latency
within ~31 ms at GOP 30 (D) — arguably the better operating point than C if the
p99 spike matters more than the last 90 ms of freeze.

## 5. Remaining blockers before ratifying anything

- Receiver-side RTP loss/dup/late for the retuned captures: the AUDIO-less
  full-rate captures did connect (signaling ok), but per-variant receiver STATS
  were not always flushed (the receiver's SIGTERM flush did not fire reliably),
  so receiver-side numbers exist only for baseline and the earlier runs.
- Encode latency is device-side (PTS → dequeued buffer) plus Go push→RTP (≤4.3 ms).
  True one-way network latency is still unmeasured (receiver has no clock anchor).
- Consent-dialog automation and the emulator/phone adb serial ambiguity made
  unattended runs flaky; both are now fixed but the soak test has not been re-run
  with the retuned GOP.
