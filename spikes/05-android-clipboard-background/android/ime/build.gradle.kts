plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

// Minimal companion IME. This is the candidate production fallback: on AOSP the
// clipboard read restriction is lifted for the *current input method*, so an IME
// we own is the one sanctioned way to read the clipboard while another app has
// focus. The spike measures whether that holds on HyperOS.
android {
    namespace = "dev.phonebridge.spike05ime"
    compileSdk = 35

    defaultConfig {
        applicationId = "dev.phonebridge.spike05ime"
        minSdk = 26
        targetSdk = 35
        versionCode = 1
        versionName = "0.1.0"
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = "17"
    }

    buildTypes {
        debug { isMinifyEnabled = false }
        release { isMinifyEnabled = false }
    }
}
