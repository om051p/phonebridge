plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

// Deliberately dependency-free (framework APIs only) so the spike builds
// without pulling any third-party artifact and cannot leak into production deps.
android {
    namespace = "dev.phonebridge.spike05"
    compileSdk = 35

    defaultConfig {
        applicationId = "dev.phonebridge.spike05"
        minSdk = 26
        // Overridable so the same prototype can be measured at both target levels:
        //   ./gradlew :app:assembleDebug -PtargetSdkOverride=34
        targetSdk = (project.findProperty("targetSdkOverride") as String?)?.toIntOrNull() ?: 35
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
        debug {
            isMinifyEnabled = false
        }
        release {
            isMinifyEnabled = false
        }
    }
}
