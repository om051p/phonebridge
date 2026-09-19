package dev.phonebridge.spike05

import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.os.Build
import android.os.Debug
import android.os.Process
import android.os.SystemClock
import android.util.Log
import java.io.File
import java.util.Locale

/**
 * Spike 05 support code: logging, JSON serialization, device/proc sampling.
 *
 * Standalone on purpose — no production code is referenced and no third-party
 * dependencies are used (the spike must not perturb app/build.gradle.kts).
 * Adapted from the Spike 03/04 support layer so evidence is directly comparable.
 */

const val S5_TAG = "Spike05"

object S5Log {
    fun i(msg: String) = Log.i(S5_TAG, msg)
    fun w(msg: String) = Log.w(S5_TAG, msg)
    fun e(msg: String, t: Throwable? = null) {
        if (t == null) Log.e(S5_TAG, msg) else Log.e(S5_TAG, msg, t)
    }

    /**
     * The single machine-readable result line the host harness parses.
     * Format: RESULT op=<x> label=<y> status=<z> k=v k=v ...
     * Keep it on one line so `adb logcat -d -s Spike05` is directly greppable.
     */
    fun result(op: String, label: String, status: String, vararg kv: Pair<String, Any?>) {
        val sb = StringBuilder("RESULT op=").append(op)
        sb.append(" label=").append(label)
        sb.append(" status=").append(status)
        for ((k, v) in kv) sb.append(' ').append(k).append('=').append(v)
        Log.i(S5_TAG, sb.toString())
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
        // OEM build identifiers matter here: the spike's central question is
        // whether a *HyperOS* build tightens AOSP clipboard policy further.
        m["oem_hyperos_version"] = systemProp("ro.mi.os.version.name")
        m["oem_hyperos_incremental"] = systemProp("ro.mi.os.version.incremental")
        m["oem_miui_ui_version"] = systemProp("ro.miui.ui.version.name")
        m["oem_build_version_incremental"] = systemProp("ro.build.version.incremental")
        return m
    }

    /** Read-only system property via reflection-free `getprop` equivalent. */
    fun systemProp(name: String): String? = try {
        val c = Class.forName("android.os.SystemProperties")
        val get = c.getMethod("get", String::class.java)
        (get.invoke(null, name) as? String)?.takeIf { it.isNotEmpty() }
    } catch (t: Throwable) {
        null
    }

    fun battery(ctx: Context): Map<String, Any?> {
        return try {
            val i = ctx.registerReceiver(null, IntentFilter(Intent.ACTION_BATTERY_CHANGED))
            if (i == null) emptyMap() else linkedMapOf(
                "level" to i.getIntExtra("level", -1),
                "scale" to i.getIntExtra("scale", -1),
                "temperature_c" to i.getIntExtra("temperature", 0) / 10.0,
                "voltage_mv" to i.getIntExtra("voltage", -1),
                "status" to i.getIntExtra("status", -1),
            )
        } catch (t: Throwable) {
            mapOf("error" to t.toString())
        }
    }
}

/** Process/system counters used for the CPU, memory and thermal measurements. */
object ProcStats {
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

    fun pss(): Map<String, Any?> = try {
        val mi = Debug.MemoryInfo()
        Debug.getMemoryInfo(mi)
        linkedMapOf<String, Any?>(
            "total_pss_kb" to mi.totalPss,
            "java_pss_kb" to mi.dalvikPss,
            "native_pss_kb" to mi.nativePss,
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

    fun clockMs(): Long = SystemClock.elapsedRealtime()

    /** Stable short digest so payloads can be matched without logging content. */
    fun hashOf(s: String): String = Integer.toHexString(s.hashCode())
}

/** Scenario/config extras shared between the host script and the app. */
object ProbeConfig {
    const val E_SCENARIO = "scenario"
    const val E_LABEL = "label"
    const val E_TEXT = "text"
    const val E_ITERATIONS = "iterations"
    const val E_HOLD_MS = "hold_ms"
    const val E_AUTO_BACKGROUND = "auto_background"
}
