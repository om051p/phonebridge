// Android host application: Flutter UI shell, Kotlin platform layer, and the
// in-process Go core loaded as a c-shared library (DEC-019). The Flutter Gradle
// tooling below is resolved from local.properties, so the SDK path is provided
// per machine (CI writes it in .github/workflows/ci.yml).

pluginManagement {
    val flutterSdkPath = run {
        val properties = java.util.Properties()
        file("local.properties").let { if (it.exists()) it.inputStream().use { s -> properties.load(s) } }
        properties.getProperty("flutter.sdk") ?: "/opt/flutter"
    }

    includeBuild("$flutterSdkPath/packages/flutter_tools/gradle")

    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}

plugins {
    id("dev.flutter.flutter-plugin-loader") version "1.0.0"
    id("com.android.application") version "8.4.0" apply false
    id("org.jetbrains.kotlin.android") version "1.9.22" apply false
}

include(":app")


