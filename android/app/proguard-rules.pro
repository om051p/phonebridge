# ProGuard / R8 Rules for PhoneBridge Android App (Milestone M-BETA-1)

# Preserve line numbers and source file attributes for debugging crashes
-keepattributes SourceFile,LineNumberTable,Signature,InnerClasses,EnclosingMethod,*Annotation*

# ---------------------------------------------------------------------------
# JNI / Native Boundary: GoBridge and Callbacks
# ---------------------------------------------------------------------------
# Keep all classes containing native methods and prevent renaming of native methods
-keepclasseswithmembernames class * {
    native <methods>;
}

# Explicitly keep GoBridge singleton and all its members (reflection / JNI entry point)
-keep class dev.phonebridge.bridge.GoBridge {
    *;
}

# Keep all callback interfaces and their method implementations called from Go CGO/JNI
-keep interface dev.phonebridge.bridge.EventListener {
    void onEvent(long, java.lang.String, byte[]);
}
-keep class * implements dev.phonebridge.bridge.EventListener {
    *;
}

-keep interface dev.phonebridge.bridge.ClipboardHostCallback {
    boolean onWritePlatformClipboard(java.lang.String, byte[]);
    boolean onSendClipboardUpdate(byte[]);
    void onOversizedPayload(int);
}
-keep class * implements dev.phonebridge.bridge.ClipboardHostCallback {
    *;
}

-keep interface dev.phonebridge.bridge.TransferHostCallback {
    java.lang.String onBeginDownload(java.lang.String, java.lang.String, long);
    int onOpenPendingFd(java.lang.String);
    java.lang.String onCommitDownload(java.lang.String);
    void onAbortDownload(java.lang.String);
    long onFreeSpaceBytes();
    void onOversizedFrame(int);
}
-keep class * implements dev.phonebridge.bridge.TransferHostCallback {
    *;
}

-keep interface dev.phonebridge.bridge.InputHostCallback {
    boolean onTouch(int, int, float, float, float);
    boolean onKey(int, int, int);
    boolean onText(java.lang.String);
    boolean onScroll(float, float, float, float);
    boolean onGlobalAction(int);
}
-keep class * implements dev.phonebridge.bridge.InputHostCallback {
    *;
}

-keep class dev.phonebridge.bridge.** {
    *;
}

# ---------------------------------------------------------------------------
# Android Application & Services (Manifest-instantiated)
# ---------------------------------------------------------------------------
-keep public class dev.phonebridge.PhoneBridgeApp {
    public <init>();
    *;
}
-keep public class dev.phonebridge.ui.MainActivity {
    public <init>();
    *;
}
-keep public class dev.phonebridge.ui.CaptureConsentActivity {
    public <init>();
    *;
}
-keep public class dev.phonebridge.service.PhoneBridgeService {
    public <init>();
    *;
}
-keep public class dev.phonebridge.ime.PhoneBridgeImeService {
    public <init>();
    *;
}
-keep public class dev.phonebridge.clipboard.ClipboardPullTileService {
    public <init>();
    *;
}
-keep public class dev.phonebridge.input.PhoneBridgeAccessibilityService {
    public <init>();
    *;
}
-keep public class dev.phonebridge.notification.PhoneBridgeNotificationListenerService {
    public <init>();
    *;
}

# ---------------------------------------------------------------------------
# Security & Cryptography (BouncyCastle & Key Stores)
# ---------------------------------------------------------------------------
-keep class dev.phonebridge.security.** {
    *;
}
-keep class org.bouncycastle.** {
    *;
}
-dontwarn org.bouncycastle.**

# ---------------------------------------------------------------------------
# Protobuf runtime and generated classes
# ---------------------------------------------------------------------------
-keep class com.google.protobuf.** {
    *;
}
-dontwarn com.google.protobuf.**

# ---------------------------------------------------------------------------
# Flutter Embedding & Plugins
# ---------------------------------------------------------------------------
-keep class io.flutter.app.** { *; }
-keep class io.flutter.plugin.** { *; }
-keep class io.flutter.util.** { *; }
-keep class io.flutter.view.** { *; }
-keep class io.flutter.** { *; }
-keep class io.flutter.plugins.** { *; }
-dontwarn io.flutter.**
