plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("dev.flutter.flutter-gradle-plugin")
}

flutter {
    source = if (file("../../ui").exists()) "../../ui" else "../.."
}

android {
    namespace = "dev.phonebridge"
    compileSdk = 34

    defaultConfig {
        applicationId = "dev.phonebridge"
        // Opt-in verification build: -PphonebridgeVerifySuffix=true produces
        // dev.phonebridge.verify so a build signed with a key we hold can be
        // installed ALONGSIDE the shipped app instead of replacing it.
        // Replacing it is refused outright (signature mismatch), and forcing it
        // would destroy the phone's Ed25519 identity and require a re-pair.
        // Off by default, so normal builds are unaffected.
        if (findProperty("phonebridgeVerifySuffix") == "true") {
            applicationIdSuffix = ".verify"
        }
        minSdk = 26
        targetSdk = 34
        versionCode = 1
        versionName = "0.1.0"

        // No ndk.abiFilters here on purpose: the `splits` block below is the
        // single ABI authority (AGP rejects both being set), and the Flutter
        // plugin otherwise re-adds armeabi-v7a — an ABI the Go core never
        // builds, which used to ship a libflutter.so with no matching
        // libphonebridge_core.so.

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = "17"
    }

    sourceSets {
        getByName("main") {
            jniLibs.srcDirs("src/main/jniLibs")
        }
    }

    signingConfigs {
        create("release") {
            val ksPath = System.getenv("PHONEBRIDGE_KEYSTORE") ?: findProperty("phonebridge.keystore.path") as String?
            val ksPass = System.getenv("PHONEBRIDGE_KEYSTORE_PASSWORD") ?: findProperty("phonebridge.keystore.password") as String?
            val keyAlias = System.getenv("PHONEBRIDGE_KEY_ALIAS") ?: findProperty("phonebridge.key.alias") as String? ?: "phonebridge"
            val keyPass = System.getenv("PHONEBRIDGE_KEY_PASSWORD") ?: findProperty("phonebridge.key.password") as String? ?: ksPass
            if (ksPath != null && ksPass != null && file(ksPath).exists()) {
                storeFile = file(ksPath)
                storePassword = ksPass
                this.keyAlias = keyAlias
                keyPassword = keyPass
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = true
            val hasReleaseKeystore = System.getenv("PHONEBRIDGE_KEYSTORE")?.let { file(it).exists() } == true ||
                (findProperty("phonebridge.keystore.path") as String?)?.let { file(it).exists() } == true
            signingConfig = if (hasReleaseKeystore) signingConfigs.getByName("release") else signingConfigs.getByName("debug")
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro"
            )
            // The Khronos validation layer is a graphics-debugging aid: it
            // must never ship in release builds (15 MB of dead weight).
            packaging {
                jniLibs {
                    excludes += "**/libVkLayer_khronos_validation.so"
                }
            }
        }
        debug {
            isMinifyEnabled = false
        }
    }

    // One APK per ABI instead of a universal APK shipping every .so three
    // times is produced at build time with
    //   flutter build apk --split-per-abi --target-platform=android-arm64,android-x64
    // (deliberately NOT via a `splits` block: AGP rejects `splits` together
    // with the ndk abiFilters the Flutter plugin contributes, and splits
    // would also change every existing artifact path and install command).
    //
    // The packaging excludes below are the repo-side part of the same goal:
    // armeabi-v7a is excluded because the Go core is never built for it
    // (core/Makefile targets arm64-v8a + x86_64 only) — shipping Flutter's
    // v7a engine without a matching libphonebridge_core.so would install and
    // then crash on launch, while excluding it makes PackageManager refuse
    // with INSTALL_FAILED_NO_MATCHING_ABIS instead.
    packaging {
        jniLibs {
            excludes += "lib/armeabi-v7a/*"
        }
    }

    packaging {
        resources {
            excludes += "/META-INF/{AL2.0,LGPL2.1}"
            excludes += "META-INF/versions/9/OSGI-INF/MANIFEST.MF"
        }
    }

testOptions {
    unitTests.isReturnDefaultValues = true
    unitTests.all {
        // JNI smoke tests load the host-built c-shared library (same sources
        // as the Android .so): build it with `make -C core host-lib`. Tests
        // skip when it is absent. Absolute path: the test JVM reads
        // java.library.path at startup and must not depend on CWD.
        it.systemProperty("java.library.path", "${projectDir}/../../core/build")
    }
}
}

dependencies {
    implementation("androidx.core:core-ktx:1.13.1")
    implementation("androidx.appcompat:appcompat:1.7.0")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.8.1")
    implementation("org.bouncycastle:bcprov-jdk18on:1.78.1")
    testImplementation("junit:junit:4.13.2")
    testImplementation("org.json:json:20240303")
    androidTestImplementation("junit:junit:4.13.2")
    androidTestImplementation("androidx.test:runner:1.5.2")
    androidTestImplementation("androidx.test.ext:junit:1.1.5")
}
