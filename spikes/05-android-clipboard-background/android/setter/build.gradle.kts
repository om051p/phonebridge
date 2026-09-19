plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

// Independent peer app. Distinct applicationId so Android treats it as a
// genuinely different clipboard owner — the spike needs to tell "the spike
// wrote this" apart from "some other app wrote this".
android {
    namespace = "dev.phonebridge.spike05setter"
    compileSdk = 35

    defaultConfig {
        applicationId = "dev.phonebridge.spike05setter"
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
