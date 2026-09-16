package dev.phonebridge.spike03

import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.hardware.display.DisplayManager
import android.os.Build
import android.os.Debug
import android.os.Process
import android.os.SystemClock
import android.util.Log
import android.view.Display
import java.io.File
import java.util.Locale

/**
 * Spike 03 support code: logging, JSON serialization, device/proc sampling.
 *
 * Standalone on purpose — no production code is referenced and no third-party
 * dependencies are used (the spike must not perturb app/build.gradle.kts).
 */

const val S3_TAG = "Spike03"

object S3Log {
    fun i(msg: String) = Log.i(S3_TAG, msg)
    fun w(msg: String) = Log.w(S3_TAG, msg)
    fun e(msg: String, t: Throwable? = null) {
        if (t == null) Log.e(S3_TAG, msg) else Log.e(S3_TAG, msg, t)
    }
}

/** Minimal, dependency-free JSON encoder for long-form evidence output. */
object Json {
    fun encode(value: Any?): String {
        val sb = StringBuilder(4096)
        write(sb, value, 0)
        return sb.toString()
    }

    private fun write(sb: StringBuilder, value: Any?, depth: Int) {
        when (value) {
            null -> sb.append("null")
            is String -> string(sb, value)
            is Boolean -> sb.append(value.toString())
            is Int, is Long, is Short, is Byte -> sb.append(value.toString())
            is Float -> number(sb, value.toDouble())
            is Double -> number(sb, value)
            is Map<*, *> -> {
                if (value.isEmpty()) { sb.append("{}"); return }
                sb.append("{\n")
                val pad = pad(depth + 1)
                var first = true
                for ((k, v) in value) {
                    if (!first) sb.append(",\n")
                    first = false
                    sb.append(pad)
                    string(sb, k.toString())
                    sb.append(": ")
                    write(sb, v, depth + 1)
                }
                sb.append("\n").append(pad(depth)).append("}")
            }
            is Iterable<*> -> {
                val list = value.toList()
                if (list.isEmpty()) { sb.append("[]"); return }
                sb.append("[\n")
                val pad = pad(depth + 1)
                list.forEachIndexed { i, v ->
                    if (i > 0) sb.append(",\n")
                    sb.append(pad)
                    write(sb, v, depth + 1)
                }
                sb.append("\n").append(pad(depth)).append("]")
            }
            is Array<*> -> write(sb, value.toList(), depth)
            is LongArray -> write(sb, value.toList(), depth)
            is IntArray -> write(sb, value.toList(), depth)
            is DoubleArray -> write(sb, value.toList(), depth)
            else -> string(sb, value.toString())
        }
    }

    private fun number(sb: StringBuilder, d: Double) {
        if (d.isNaN() || d.isInfinite()) { sb.append("null"); return }
        if (d == d.toLong().toDouble()) sb.append(d.toLong().toString())
        else sb.append(String.format(Locale.US, "%.3f", d))
    }

    private fun string(sb: StringBuilder, s: String) {
        sb.append('"')
        for (c in s) {
            when (c) {
                '"' -> sb.append("\\\"")
                '\\' -> sb.append("\\\\")
                '\n' -> sb.append("\\n")
                '\r' -> sb.append("\\r")
                '\t' -> sb.append("\\t")
                else -> if (c < ' ') sb.append(String.format(Locale.US, "\\u%04x", c.code)) else sb.append(c)
            }
        }
        sb.append('"')
    }

    private fun pad(depth: Int) = "  ".repeat(depth)
}

object Stats {
    fun percentile(sorted: List<Double>, p: Double): Double {
        if (sorted.isEmpty()) return Double.NaN
        if (sorted.size == 1) return sorted[0]
        val idx = (p / 100.0) * (sorted.size - 1)
        val lo = idx.toInt()
        val hi = minOf(lo + 1, sorted.size - 1)
        val frac = idx - lo
        return sorted[lo] * (1 - frac) + sorted[hi] * frac
    }

    /** p50/p90/p99/min/max/mean summary for a sample list. */
    fun summary(samples: List<Double>): Map<String, Any?> {
        if (samples.isEmpty()) return linkedMapOf("samples" to 0)
        val s = samples.sorted()
        return linkedMapOf<String, Any?>(
            "samples" to s.size,
            "p50" to percentile(s, 50.0),
            "p90" to percentile(s, 90.0),
            "p99" to percentile(s, 99.0),
            "min" to s.first(),
            "max" to s.last(),
            "mean" to s.sum() / s.size,
        )
    }
}

object Device {
    fun info(ctx: Context): Map<String, Any?> {
        val m = linkedMapOf<String, Any?>(
            "manufacturer" to Build.MANUFACTURER,
            "model" to Build.MODEL,
            "device" to Build.DEVICE,
            "product" to Build.PRODUCT,
            "hardware" to Build.HARDWARE,
            "board" to Build.BOARD,
            "abis" to Build.SUPPORTED_ABIS.toList(),
            "android_release" to Build.VERSION.RELEASE,
            "android_sdk" to Build.VERSION.SDK_INT,
            "security_patch" to Build.VERSION.SECURITY_PATCH,
            "build_incremental" to Build.VERSION.INCREMENTAL,
            "build_fingerprint" to Build.FINGERPRINT,
            "app_target_sdk" to ctx.applicationInfo.targetSdkVersion,
        )
        if (Build.VERSION.SDK_INT >= 31) {
            m["soc_manufacturer"] = Build.SOC_MANUFACTURER
            m["soc_model"] = Build.SOC_MODEL
        }
        try {
            val dm = ctx.getSystemService(Context.DISPLAY_SERVICE) as DisplayManager
            val d = dm.getDisplay(Display.DEFAULT_DISPLAY)
            m["display_refresh_hz"] = d.refreshRate.toDouble()
            m["display_native_mode"] = d.mode?.toString()
            val dmMetrics = ctx.resources.displayMetrics
            m["display_size_px"] = "${dmMetrics.widthPixels}x${dmMetrics.heightPixels}"
            m["display_density_dpi"] = dmMetrics.densityDpi
        } catch (t: Throwable) {
            m["display_error"] = t.toString()
        }
        return m
    }

    fun battery(ctx: Context): Map<String, Any?> {
        return try {
            val i = ctx.registerReceiver(null, IntentFilter(Intent.ACTION_BATTERY_CHANGED))
            if (i == null) emptyMap() else linkedMapOf(
                "level" to i.getIntExtra("level", -1),
                "scale" to i.getIntExtra("scale", -1),
                "temperature_c" to i.getIntExtra("temperature", 0) / 10.0,
                "voltage_mv" to i.getIntExtra("voltage", -1),
            )
        } catch (t: Throwable) {
            mapOf("error" to t.toString())
        }
    }
}

/** Process/system counters used for the CPU, memory and thermal measurements. */
object ProcStats {
    /** Cumulative CPU time of this process (user + system), milliseconds. */
    fun appCpuMs(): Long = Process.getElapsedCpuTime()

    private fun statusField(name: String): Long? = try {
        File("/proc/self/status").readLines()
            .firstOrNull { it.startsWith(name) }
            ?.split(Regex("\\s+"))
            ?.getOrNull(1)
            ?.toLongOrNull()
    } catch (t: Throwable) {
        null
    }

    fun rssKb(): Long? = statusField("VmRSS:")
    fun peakRssKb(): Long? = statusField("VmHWM:")
    fun threads(): Long? = statusField("Threads:")

    /** PSS of this process (includes codec buffers owned by the app, excludes codec service). */
    fun pss(): Map<String, Any?> = try {
        val mi = Debug.MemoryInfo()
        Debug.getMemoryInfo(mi)
        linkedMapOf<String, Any?>(
            "total_pss_kb" to mi.totalPss,
            "java_pss_kb" to mi.dalvikPss,
            "native_pss_kb" to mi.nativePss,
            "graphics_pss_kb" to (mi.getMemoryStat("summary.graphics")?.toIntOrNull() ?: -1),
        )
    } catch (t: Throwable) {
        mapOf("error" to t.toString())
    }

    /** (totalJiffies, busyJiffies) from /proc/stat — device-wide CPU. */
    fun deviceCpuJiffies(): Pair<Long, Long>? = try {
        val line = File("/proc/stat").readLines().firstOrNull { it.startsWith("cpu ") }
        if (line == null) null else {
            val nums = line.trim().split(Regex("\\s+")).drop(1).mapNotNull { it.toLongOrNull() }
            if (nums.size < 5) null else {
                val total = nums.sum()
                val idle = nums[3] + nums[4]
                total to (total - idle)
            }
        }
    } catch (t: Throwable) {
        null
    }

    /** Readable thermal zones → degrees C. Filtered/truncated to stay small. */
    fun thermal(): Map<String, Any?> {
        val out = linkedMapOf<String, Any?>()
        try {
            val zones = File("/sys/class/thermal").listFiles() ?: return mapOf("error" to "no /sys/class/thermal")
            for (z in zones.filter { it.name.startsWith("thermal_zone") }.sortedBy { it.name }) {
                val type = try { File(z, "type").readText().trim() } catch (t: Throwable) { continue }
                val raw = try { File(z, "temp").readText().trim().toDouble() } catch (t: Throwable) { continue }
                if (type.isEmpty() || out.containsKey(type)) continue
                out[type] = if (raw > 1000) raw / 1000.0 else raw
            }
        } catch (t: Throwable) {
            out["error"] = t.toString()
        }
        return out
    }

    fun clockMs(): Long = SystemClock.elapsedRealtime()
}

/**
 * Hooks the foreground Activity installs so the capture service (which has no UI)
 * can drive device-visible probes: FLAG_SECURE and backgrounding.
 */
object UiHooks {
    @Volatile var secureSetter: ((Boolean) -> Unit)? = null
    @Volatile var backgroundMover: (() -> Unit)? = null

    /** Freezes the on-screen animation to test damage-driven composition. */
    @Volatile var contentFreezer: ((Boolean) -> Unit)? = null
}
