//go:build android || jni

// Package main implements the Android in-process Go core entrypoint for PhoneBridge.
// Built with -buildmode=c-shared to produce libphonebridge_core.so.
//
// Ratified under DEC-019 (Spike 02): In-process c-shared Go library + JNI inside
// an Android Foreground Service.
package main

/*
#include <jni.h>
#include <stdlib.h>
#include <string.h>

static int isNull(jobject obj) {
    return obj == NULL ? 1 : 0;
}

static jobject nullObject() {
    return NULL;
}

static jbyteArray nullByteArray() {
    return NULL;
}

static char* getUTFChars(JNIEnv *env, jstring str) {
    if (!str) return NULL;
    return (char*)(*env)->GetStringUTFChars(env, str, NULL);
}

static void releaseUTFChars(JNIEnv *env, jstring str, const char *chars) {
    if (str && chars) {
        (*env)->ReleaseStringUTFChars(env, str, chars);
    }
}

static jbyteArray newByteArray(JNIEnv *env, int len, const char *bytes) {
    jbyteArray arr = (*env)->NewByteArray(env, len);
    if (arr && len > 0 && bytes) {
        (*env)->SetByteArrayRegion(env, arr, 0, len, (const jbyte*)bytes);
    }
    return arr;
}

static int getByteArray(JNIEnv *env, jbyteArray arr, char *buf, int maxLen) {
    if (!arr) return 0;
    int len = (*env)->GetArrayLength(env, arr);
    if (len > maxLen) len = maxLen;
    if (len > 0 && buf) {
        (*env)->GetByteArrayRegion(env, arr, 0, len, (jbyte*)buf);
    }
    return len;
}

static int getByteArrayLen(JNIEnv *env, jbyteArray arr) {
    if (!arr) return 0;
    return (*env)->GetArrayLength(env, arr);
}

static jmethodID resolveListenerMethod(JNIEnv *env, jobject listener) {
    if (!env || !listener) return NULL;
    jclass cls = (*env)->GetObjectClass(env, listener);
    if (!cls) return NULL;
    jmethodID mid = (*env)->GetMethodID(env, cls, "onEvent", "(JLjava/lang/String;[B)V");
    if (!mid) {
        (*env)->ExceptionClear(env);
        jclass ifaceCls = (*env)->FindClass(env, "dev/phonebridge/bridge/EventListener");
        if (ifaceCls) {
            mid = (*env)->GetMethodID(env, ifaceCls, "onEvent", "(JLjava/lang/String;[B)V");
            (*env)->DeleteLocalRef(env, ifaceCls);
        }
    }
    (*env)->DeleteLocalRef(env, cls);
    return mid;
}

static int isNullMethod(jmethodID mid) {
    return mid == NULL ? 1 : 0;
}

static int callOnEvent(JNIEnv *env, jobject listener, jmethodID method, jlong seq, const char *kind, const char *payload, int payloadLen) {
    if (!env || !listener || !method) return -1;
    jstring jKind = (*env)->NewStringUTF(env, kind);
    jbyteArray jPayload = newByteArray(env, payloadLen, payload);
    (*env)->CallVoidMethod(env, listener, method, seq, jKind, jPayload);
    if (jKind) (*env)->DeleteLocalRef(env, jKind);
    if (jPayload) (*env)->DeleteLocalRef(env, jPayload);
    return 0;
}

static jmethodID resolveClipboardWriteMethod(JNIEnv *env, jobject host) {
    if (!env || !host) return NULL;
    jclass cls = (*env)->GetObjectClass(env, host);
    if (!cls) return NULL;
    jmethodID mid = (*env)->GetMethodID(env, cls, "onWritePlatformClipboard", "(Ljava/lang/String;[B)Z");
    (*env)->DeleteLocalRef(env, cls);
    return mid;
}

static jmethodID resolveClipboardSendMethod(JNIEnv *env, jobject host) {
    if (!env || !host) return NULL;
    jclass cls = (*env)->GetObjectClass(env, host);
    if (!cls) return NULL;
    jmethodID mid = (*env)->GetMethodID(env, cls, "onSendClipboardUpdate", "([B)Z");
    (*env)->DeleteLocalRef(env, cls);
    return mid;
}

static jmethodID resolveClipboardOversizedMethod(JNIEnv *env, jobject host) {
    if (!env || !host) return NULL;
    jclass cls = (*env)->GetObjectClass(env, host);
    if (!cls) return NULL;
    jmethodID mid = (*env)->GetMethodID(env, cls, "onOversizedPayload", "(I)V");
    (*env)->DeleteLocalRef(env, cls);
    return mid;
}

static int callClipboardWrite(JNIEnv *env, jobject host, jmethodID mid, const char *mime, const char *payload, int payloadLen) {
    if (!env || !host || !mid) return 0;
    jstring jMime = (*env)->NewStringUTF(env, mime);
    jbyteArray jBytes = newByteArray(env, payloadLen, payload);
    jboolean res = (*env)->CallBooleanMethod(env, host, mid, jMime, jBytes);
    if (jMime) (*env)->DeleteLocalRef(env, jMime);
    if (jBytes) (*env)->DeleteLocalRef(env, jBytes);
    return res == JNI_TRUE ? 1 : 0;
}

static int callClipboardSend(JNIEnv *env, jobject host, jmethodID mid, const char *payload, int payloadLen) {
    if (!env || !host || !mid) return 0;
    jbyteArray jBytes = newByteArray(env, payloadLen, payload);
    jboolean res = (*env)->CallBooleanMethod(env, host, mid, jBytes);
    if (jBytes) (*env)->DeleteLocalRef(env, jBytes);
    return res == JNI_TRUE ? 1 : 0;
}

static void callClipboardOversized(JNIEnv *env, jobject host, jmethodID mid, int size) {
    if (!env || !host || !mid) return;
    (*env)->CallVoidMethod(env, host, mid, (jint)size);
}


static void deleteLocalRef(JNIEnv *env, jobject obj) {
    if (env && obj) (*env)->DeleteLocalRef(env, obj);
}

static jint getOrAttachEnv(JavaVM *jvm, JNIEnv **env, int *didAttach) {
    *didAttach = 0;
    jint res = (*jvm)->GetEnv(jvm, (void**)env, JNI_VERSION_1_6);
    if (res == JNI_OK) {
        return 0;
    }
#if defined(__ANDROID__)
    res = (*jvm)->AttachCurrentThread(jvm, env, NULL);
#else
    res = (*jvm)->AttachCurrentThread(jvm, (void**)env, NULL);
#endif
    if (res == 0) {
        *didAttach = 1;
        return 0;
    }
    return res;
}

static void releaseEnv(JavaVM *jvm, int didAttach) {
    if (didAttach) {
        (*jvm)->DetachCurrentThread(jvm);
    }
}

static jint getJavaVM(JNIEnv *env, JavaVM **jvm) {
    return (*env)->GetJavaVM(env, jvm);
}

static jint attachCurrentThread(JavaVM *jvm, JNIEnv **env) {
#if defined(__ANDROID__)
    return (*jvm)->AttachCurrentThread(jvm, env, NULL);
#else
    return (*jvm)->AttachCurrentThread(jvm, (void**)env, NULL);
#endif
}

static jint detachCurrentThread(JavaVM *jvm) {
    return (*jvm)->DetachCurrentThread(jvm);
}

static jobject makeGlobalRef(JNIEnv *env, jobject obj) {
    return (*env)->NewGlobalRef(env, obj);
}

static void deleteGlobalRef(JNIEnv *env, jobject obj) {
    (*env)->DeleteGlobalRef(env, obj);
}

static void throwIllegalState(JNIEnv *env, const char *msg) {
    jclass cls = (*env)->FindClass(env, "java/lang/IllegalStateException");
    if (cls != NULL) {
        (*env)->ThrowNew(env, cls, msg);
        (*env)->DeleteLocalRef(env, cls);
    }
}
*/
import "C"

import (
	"context"
	"fmt"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

var (
	jvm         *C.JavaVM
	mu          sync.Mutex
	initialized atomic.Bool

	// Lifecycle state: 0=STOPPED, 1=RUNNING, 2=TRIMMED
	engineState atomic.Int32

	// Diagnostics counters
	invocations  atomic.Uint64
	eventsSent   atomic.Uint64
	panicsCaught atomic.Uint64

	// Active stream cancellation
	streamCancel context.CancelFunc
	streamWg     sync.WaitGroup
)

const (
	StateStopped = 0
	StateRunning = 1
	StateTrimmed = 2
)

func main() {}

//export Java_dev_phonebridge_bridge_GoBridge_nativeStart
func Java_dev_phonebridge_bridge_GoBridge_nativeStart(env *C.JNIEnv, clazz C.jclass, jStorageDir C.jstring) C.jboolean {
	_ = clazz
	mu.Lock()
	defer mu.Unlock()

	if initialized.Load() {
		return C.JNI_TRUE
	}

	if jvm == nil {
		if C.getJavaVM(env, &jvm) != 0 || jvm == nil {
			C.throwIllegalState(env, C.CString("Failed to obtain JavaVM reference"))
			return C.JNI_FALSE
		}
	}

	if C.isNull(C.jobject(jStorageDir)) == 0 {
		cChars := C.getUTFChars(env, jStorageDir)
		if cChars != nil {
			storageDir := C.GoString(cChars)
			C.releaseUTFChars(env, jStorageDir, cChars)
			_ = storageDir
		}
	}

	// Begin browsing for LAN peers as soon as the engine is up, so the Devices
	// tab has results the first time it asks instead of waiting for its own poll
	// to warm the browse session. A bind failure is not fatal: the browse then
	// reports an empty peer list, which is what the UI renders as "nothing
	// discovered yet" — the phone's own advertisement (NsdAdvertiser) makes it
	// discoverable in the other direction regardless.
	_ = currentDiscoveryBridge().Start()

	engineState.Store(StateRunning)
	initialized.Store(true)
	return C.JNI_TRUE
}

//export Java_dev_phonebridge_bridge_GoBridge_nativeInvoke
func Java_dev_phonebridge_bridge_GoBridge_nativeInvoke(env *C.JNIEnv, clazz C.jclass, jMethod C.jstring, jPayload C.jbyteArray) C.jbyteArray {
	_ = clazz
	if !initialized.Load() || engineState.Load() == StateStopped {
		C.throwIllegalState(env, C.CString("Go engine is not running"))
		return C.nullByteArray()
	}

	// Safe panic recovery to protect the JVM from crashing if a Go panic occurs
	defer func() {
		if r := recover(); r != nil {
			panicsCaught.Add(1)
			errStr := fmt.Sprintf("Go panic recovered: %v", r)
			cErr := C.CString(errStr)
			defer C.free(unsafe.Pointer(cErr))
			C.throwIllegalState(env, cErr)
		}
	}()

	invocations.Add(1)

	if C.isNull(C.jobject(jMethod)) == 1 {
		C.throwIllegalState(env, C.CString("method cannot be null"))
		return C.nullByteArray()
	}

	cMethod := C.getUTFChars(env, jMethod)
	if cMethod == nil {
		C.throwIllegalState(env, C.CString("failed to read method string"))
		return C.nullByteArray()
	}
	method := C.GoString(cMethod)
	C.releaseUTFChars(env, jMethod, cMethod)

	var payload []byte
	if C.isNull(C.jobject(jPayload)) == 0 {
		length := int(C.getByteArrayLen(env, jPayload))
		if length > 0 {
			payload = make([]byte, length)
			C.getByteArray(env, jPayload, (*C.char)(unsafe.Pointer(&payload[0])), C.int(length))
		}
	}

	// Method routing
	var resp []byte
	if out, handled := invokeTransfer(method, payload); handled {
		if out == nil {
			out = []byte("{}")
		}
		resp = out
	} else if out, handled := invokeDiscovery(method, payload); handled {
		if out == nil {
			out = []byte("{}")
		}
		resp = out
	} else {
		switch method {
		case "ping":
			resp = append([]byte("pong:"), payload...)
		case "echo":
			resp = make([]byte, len(payload))
			copy(resp, payload)
		case "state":
			resp = []byte(fmt.Sprintf("%d", engineState.Load()))
		case "panic":
			panic("simulated Go runtime panic")
		default:
			cErr := C.CString(fmt.Sprintf("unknown method: %s", method))
			defer C.free(unsafe.Pointer(cErr))
			C.throwIllegalState(env, cErr)
			return C.nullByteArray()
		}
	}

	var respPtr *C.char
	if len(resp) > 0 {
		respPtr = (*C.char)(unsafe.Pointer(&resp[0]))
	}
	return C.newByteArray(env, C.int(len(resp)), respPtr)
}

//export Java_dev_phonebridge_bridge_GoBridge_nativeSubscribe
func Java_dev_phonebridge_bridge_GoBridge_nativeSubscribe(env *C.JNIEnv, clazz C.jclass, jListener C.jobject, count C.jint, intervalMs C.jint) C.jboolean {
	_ = clazz
	if !initialized.Load() || engineState.Load() == StateStopped {
		C.throwIllegalState(env, C.CString("Go engine is not running"))
		return C.JNI_FALSE
	}

	if C.isNull(jListener) == 1 {
		C.throwIllegalState(env, C.CString("listener cannot be null"))
		return C.JNI_FALSE
	}

	onEventMid := C.resolveListenerMethod(env, jListener)
	if C.isNullMethod(onEventMid) == 1 {
		C.throwIllegalState(env, C.CString("Failed to resolve onEvent method ID from EventListener"))
		return C.JNI_FALSE
	}

	// Create a global reference to prevent GC while Go goroutine runs
	globalListener := C.makeGlobalRef(env, jListener)

	mu.Lock()
	if streamCancel != nil {
		streamCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	streamCancel = cancel
	mu.Unlock()

	total := int(count)
	interval := time.Duration(intervalMs) * time.Millisecond
	if interval < time.Millisecond {
		interval = time.Millisecond
	}

	streamWg.Add(1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer streamWg.Done()

		var threadEnv *C.JNIEnv
		if C.attachCurrentThread(jvm, &threadEnv) != 0 || threadEnv == nil {
			return
		}
		defer func() {
			C.deleteGlobalRef(threadEnv, globalListener)
			C.detachCurrentThread(jvm)
		}()

		cKind := C.CString("event")
		defer C.free(unsafe.Pointer(cKind))

		for i := 1; i <= total; i++ {
			select {
			case <-ctx.Done():
				return
			default:
			}

			payloadBytes := []byte(fmt.Sprintf("event-%d", i))
			var pPtr *C.char
			if len(payloadBytes) > 0 {
				pPtr = (*C.char)(unsafe.Pointer(&payloadBytes[0]))
			}

			C.callOnEvent(threadEnv, globalListener, onEventMid, C.jlong(i), cKind, pPtr, C.int(len(payloadBytes)))
			eventsSent.Add(1)

			if i < total {
				select {
				case <-ctx.Done():
					return
				case <-time.After(interval):
				}
			}
		}
	}()

	return C.JNI_TRUE
}

// ---------------------------------------------------------------------------
// Media transport (Step 3, DEC-020/021): dedicated JNI exports for the
// production WebRTC pipeline. Control plane (nativeInvoke) stays generic;
// frame data NEVER routes through invoke. All media exports delegate to the
// process-wide MediaTransport (transport.go) and are panic-guarded so a Go
// panic can never abort the JVM (DEC-019 rule).
//
// Lifecycle contract (full semantics in transport.go):
//	idle --mediaInit--> initialized --createOffer/setAnswer--> negotiated
//	     --mediaStart--> streaming --mediaStop--> stopped --mediaRelease--> idle
// Backpressure: OnFrame never blocks; bounded queue drops non-key frames when
// full (returns false to the caller), evicts to admit keyframes. Calls before
// readiness are safe no-ops with documented returns; misuse errors escalate
// to IllegalStateException.

// mediaRecover converts a Go panic into an IllegalStateException (DEC-019).
func mediaRecover(env *C.JNIEnv, what string) {
	if r := recover(); r != nil {
		panicsCaught.Add(1)
		errStr := fmt.Sprintf("%s: Go panic recovered: %v", what, r)
		cErr := C.CString(errStr)
		defer C.free(unsafe.Pointer(cErr))
		C.throwIllegalState(env, cErr)
	}
}

// cBytes views a Java byte[] as a Go slice for one call (borrows JVM memory;
// valid only until the export returns — copy before keeping).
func cBytes(env *C.JNIEnv, arr C.jbyteArray) []byte {
	if C.isNull(C.jobject(arr)) == 1 {
		return nil
	}
	n := int(C.getByteArrayLen(env, arr))
	if n <= 0 {
		return nil
	}
	buf := make([]byte, n)
	C.getByteArray(env, arr, (*C.char)(unsafe.Pointer(&buf[0])), C.int(n))
	return buf
}

// goString copies a Java String into a Go string (empty for null). The JVM
// chars are released immediately, so the result owns no JVM memory (JNI
// critical: a borrowed pointer would be invalid after the export returns).
func goString(env *C.JNIEnv, s C.jstring) string {
	if C.isNull(C.jobject(s)) == 1 {
		return ""
	}
	chars := C.getUTFChars(env, s)
	if chars == nil {
		return ""
	}
	out := C.GoString(chars)
	C.releaseUTFChars(env, s, chars)
	return out
}

// goBytesToJava copies a Go slice into a fresh Java byte[].
func goBytesToJava(env *C.JNIEnv, b []byte) C.jbyteArray {
	var ptr *C.char
	if len(b) > 0 {
		ptr = (*C.char)(unsafe.Pointer(&b[0]))
	}
	return C.newByteArray(env, C.int(len(b)), ptr)
}

//export Java_dev_phonebridge_bridge_GoBridge_nativeMediaInit
func Java_dev_phonebridge_bridge_GoBridge_nativeMediaInit(env *C.JNIEnv, clazz C.jclass) C.jboolean {
	_ = clazz
	defer mediaRecover(env, "mediaInit")
	if !initialized.Load() {
		C.throwIllegalState(env, C.CString("Go engine is not running"))
		return C.JNI_FALSE
	}
	if err := currentTransport().MediaInit(); err != nil {
		cErr := C.CString(err.Error())
		defer C.free(unsafe.Pointer(cErr))
		C.throwIllegalState(env, cErr)
		return C.JNI_FALSE
	}
	return C.JNI_TRUE
}

//export Java_dev_phonebridge_bridge_GoBridge_nativeMediaCreateOffer
func Java_dev_phonebridge_bridge_GoBridge_nativeMediaCreateOffer(env *C.JNIEnv, clazz C.jclass) C.jbyteArray {
	_ = clazz
	defer mediaRecover(env, "mediaCreateOffer")
	if !initialized.Load() {
		C.throwIllegalState(env, C.CString("Go engine is not running"))
		return C.nullByteArray()
	}
	offer, err := currentTransport().MediaCreateOffer()
	if err != nil {
		cErr := C.CString(err.Error())
		defer C.free(unsafe.Pointer(cErr))
		C.throwIllegalState(env, cErr)
		return C.nullByteArray()
	}
	return goBytesToJava(env, offer)
}

//export Java_dev_phonebridge_bridge_GoBridge_nativeMediaSetAnswer
func Java_dev_phonebridge_bridge_GoBridge_nativeMediaSetAnswer(env *C.JNIEnv, clazz C.jclass, jAnswer C.jbyteArray) C.jboolean {
	_ = clazz
	defer mediaRecover(env, "mediaSetAnswer")
	if !initialized.Load() {
		C.throwIllegalState(env, C.CString("Go engine is not running"))
		return C.JNI_FALSE
	}
	answer := cBytes(env, jAnswer)
	if answer == nil {
		C.throwIllegalState(env, C.CString("answer cannot be null"))
		return C.JNI_FALSE
	}
	if err := currentTransport().MediaSetAnswer(answer); err != nil {
		cErr := C.CString(err.Error())
		defer C.free(unsafe.Pointer(cErr))
		C.throwIllegalState(env, cErr)
		return C.JNI_FALSE
	}
	return C.JNI_TRUE
}

//export Java_dev_phonebridge_bridge_GoBridge_nativeMediaStart
func Java_dev_phonebridge_bridge_GoBridge_nativeMediaStart(env *C.JNIEnv, clazz C.jclass) C.jboolean {
	_ = clazz
	defer mediaRecover(env, "mediaStart")
	if !initialized.Load() {
		C.throwIllegalState(env, C.CString("Go engine is not running"))
		return C.JNI_FALSE
	}
	if err := currentTransport().MediaStart(); err != nil {
		cErr := C.CString(err.Error())
		defer C.free(unsafe.Pointer(cErr))
		C.throwIllegalState(env, cErr)
		return C.JNI_FALSE
	}
	return C.JNI_TRUE
}

//export Java_dev_phonebridge_bridge_GoBridge_nativeMediaReportSessionError
func Java_dev_phonebridge_bridge_GoBridge_nativeMediaReportSessionError(env *C.JNIEnv, clazz C.jclass, jCode C.jstring, jMessage C.jstring) C.jboolean {
	_ = clazz
	defer mediaRecover(env, "mediaReportSessionError")
	code := goString(env, jCode)
	message := goString(env, jMessage)
	if code == "" {
		C.throwIllegalState(env, C.CString("session error code cannot be empty"))
		return C.JNI_FALSE
	}
	if err := currentTransport().ReportSessionError(code, message); err != nil {
		// A failure to report is not itself fatal: the caller is already on an
		// error path, and the session teardown that follows is what the peer
		// ultimately observes. Return false so Kotlin can log it.
		return C.JNI_FALSE
	}
	return C.JNI_TRUE
}

//export Java_dev_phonebridge_bridge_GoBridge_nativeMediaStop
func Java_dev_phonebridge_bridge_GoBridge_nativeMediaStop(env *C.JNIEnv, clazz C.jclass) {
	_ = env
	_ = clazz
	defer mediaRecover(env, "mediaStop")
	currentTransport().MediaStop()
}

//export Java_dev_phonebridge_bridge_GoBridge_nativeMediaRelease
func Java_dev_phonebridge_bridge_GoBridge_nativeMediaRelease(env *C.JNIEnv, clazz C.jclass) {
	_ = env
	_ = clazz
	defer mediaRecover(env, "mediaRelease")
	currentTransport().MediaRelease()
}

//export Java_dev_phonebridge_bridge_GoBridge_nativeMediaStats
func Java_dev_phonebridge_bridge_GoBridge_nativeMediaStats(env *C.JNIEnv, clazz C.jclass) C.jbyteArray {
	_ = clazz
	defer mediaRecover(env, "mediaStats")
	return goBytesToJava(env, currentTransport().MediaStatsJSON())
}

//export Java_dev_phonebridge_bridge_GoBridge_nativeMediaOnFrame
func Java_dev_phonebridge_bridge_GoBridge_nativeMediaOnFrame(env *C.JNIEnv, clazz C.jclass, jPtsUs C.jlong, jAU C.jbyteArray, jKeyframe C.jboolean) C.jboolean {
	_ = clazz
	defer mediaRecover(env, "mediaOnFrame")
	au := cBytes(env, jAU)
	if au == nil {
		return C.JNI_FALSE
	}
	key := jKeyframe == C.JNI_TRUE
	if !initialized.Load() || engineState.Load() == StateStopped {
		// This gate short-circuits before the transport, so MediaOnFrame — the
		// normal PSI learning point — is never reached. Learn here instead:
		// the once-per-codec CSD AU can arrive while the engine is not running
		// yet, and losing it leaves every later transport re-injecting nothing
		// (live E2E: bare IDRs on the wire, receiver reports
		// PARAM_SETS_MISSING). LearnPSI only caches bytes; the frame is still
		// refused. The normal path is unchanged and learns exactly once, so no
		// AU is ever split twice on the hot path.
		currentTransport().LearnPSI(au)
		return C.JNI_FALSE
	}
	if currentTransport().MediaOnFrame(au, int64(jPtsUs), key) {
		return C.JNI_TRUE
	}
	return C.JNI_FALSE
}

//export Java_dev_phonebridge_bridge_GoBridge_nativeTrimMemory
func Java_dev_phonebridge_bridge_GoBridge_nativeTrimMemory(env *C.JNIEnv, clazz C.jclass, level C.jint) {
	_ = env
	_ = clazz
	engineState.Store(StateTrimmed)
	runtime.GC()
	debug.FreeOSMemory()
}

//export Java_dev_phonebridge_bridge_GoBridge_nativeStop
func Java_dev_phonebridge_bridge_GoBridge_nativeStop(env *C.JNIEnv, clazz C.jclass) C.jboolean {
	_ = clazz
	mu.Lock()
	defer mu.Unlock()

	if !initialized.Load() {
		return C.JNI_TRUE
	}

	// Defense in depth: tear the media transport down first (the Kotlin
	// service normally calls mediaStop/mediaRelease explicitly before
	// stopping the engine; this guarantees no Pion goroutines survive
	// engine stop even if it does not).
	if t := mediaTransport.Load(); t != nil {
		t.MediaStop()
		t.MediaRelease()
		mediaTransport.Store(nil)
	}

	// Defense in depth: tear down clipboard bridge
	currentClipboardBridge().Stop()

	// Defense in depth: tear down the LAN browse session (multicast sockets
	// and the browse/sweep/refresh goroutines must not outlive the engine).
	currentDiscoveryBridge().Stop()
	clipboardHostMu.Lock()
	if currentJniHost != nil && C.isNull(currentJniHost.callbackObj) == 0 {
		C.deleteGlobalRef(env, currentJniHost.callbackObj)
		currentJniHost.callbackObj = C.nullObject()
	}
	currentJniHost = nil
	clipboardHostMu.Unlock()

	if streamCancel != nil {
		streamCancel()
	}
	streamWg.Wait()

	engineState.Store(StateStopped)
	initialized.Store(false)
	return C.JNI_TRUE
}

var (
	clipboardHostMu sync.Mutex
	currentJniHost  *jniClipboardHost
)

type jniClipboardHost struct {
	callbackObj  C.jobject
	writeMid     C.jmethodID
	sendMid      C.jmethodID
	oversizedMid C.jmethodID
}

func (h *jniClipboardHost) WritePlatformClipboard(mimeType string, payload []byte) bool {
	if jvm == nil || C.isNull(h.callbackObj) == 1 {
		return false
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var env *C.JNIEnv
	var didAttach C.int
	if C.getOrAttachEnv(jvm, &env, &didAttach) != 0 || env == nil {
		return false
	}
	defer C.releaseEnv(jvm, didAttach)

	cMime := C.CString(mimeType)
	defer C.free(unsafe.Pointer(cMime))

	var cPayload *C.char
	if len(payload) > 0 {
		cPayload = (*C.char)(unsafe.Pointer(&payload[0]))
	}

	ok := C.callClipboardWrite(env, h.callbackObj, h.writeMid, cMime, cPayload, C.int(len(payload)))
	return ok == 1
}

func (h *jniClipboardHost) SendClipboardUpdate(wireBytes []byte) bool {
	if jvm == nil || C.isNull(h.callbackObj) == 1 {
		return false
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var env *C.JNIEnv
	var didAttach C.int
	if C.getOrAttachEnv(jvm, &env, &didAttach) != 0 || env == nil {
		return false
	}
	defer C.releaseEnv(jvm, didAttach)

	var cBytes *C.char
	if len(wireBytes) > 0 {
		cBytes = (*C.char)(unsafe.Pointer(&wireBytes[0]))
	}

	ok := C.callClipboardSend(env, h.callbackObj, h.sendMid, cBytes, C.int(len(wireBytes)))
	return ok == 1
}

func (h *jniClipboardHost) OnOversizedPayload(size int) {
	if jvm == nil || C.isNull(h.callbackObj) == 1 {
		return
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var env *C.JNIEnv
	var didAttach C.int
	if C.getOrAttachEnv(jvm, &env, &didAttach) != 0 || env == nil {
		return
	}
	defer C.releaseEnv(jvm, didAttach)

	C.callClipboardOversized(env, h.callbackObj, h.oversizedMid, C.int(size))
}

func clipboardRecover(env *C.JNIEnv, what string) {
	if r := recover(); r != nil {
		panicsCaught.Add(1)
		errStr := fmt.Sprintf("%s: Go panic recovered: %v", what, r)
		cErr := C.CString(errStr)
		defer C.free(unsafe.Pointer(cErr))
		C.throwIllegalState(env, cErr)
	}
}

//export Java_dev_phonebridge_bridge_GoBridge_nativeClipboardInit
func Java_dev_phonebridge_bridge_GoBridge_nativeClipboardInit(env *C.JNIEnv, clazz C.jclass, jCallback C.jobject) C.jboolean {
	_ = clazz
	defer clipboardRecover(env, "clipboardInit")

	if C.isNull(jCallback) == 1 {
		C.throwIllegalState(env, C.CString("callback cannot be null"))
		return C.JNI_FALSE
	}

	if jvm == nil {
		if C.getJavaVM(env, &jvm) != 0 || jvm == nil {
			C.throwIllegalState(env, C.CString("Failed to obtain JavaVM reference"))
			return C.JNI_FALSE
		}
	}

	writeMid := C.resolveClipboardWriteMethod(env, jCallback)
	sendMid := C.resolveClipboardSendMethod(env, jCallback)
	oversizedMid := C.resolveClipboardOversizedMethod(env, jCallback)

	if C.isNullMethod(writeMid) == 1 || C.isNullMethod(sendMid) == 1 || C.isNullMethod(oversizedMid) == 1 {
		C.throwIllegalState(env, C.CString("Failed to resolve ClipboardHostCallback methods"))
		return C.JNI_FALSE
	}

	gRef := C.makeGlobalRef(env, jCallback)
	if C.isNull(gRef) == 1 {
		C.throwIllegalState(env, C.CString("Failed to create global ref for callback"))
		return C.JNI_FALSE
	}

	clipboardHostMu.Lock()
	if currentJniHost != nil && C.isNull(currentJniHost.callbackObj) == 0 {
		C.deleteGlobalRef(env, currentJniHost.callbackObj)
	}
	currentJniHost = &jniClipboardHost{
		callbackObj:  gRef,
		writeMid:     writeMid,
		sendMid:      sendMid,
		oversizedMid: oversizedMid,
	}
	clipboardHostMu.Unlock()

	if err := currentClipboardBridge().Init(currentJniHost); err != nil {
		cErr := C.CString(err.Error())
		defer C.free(unsafe.Pointer(cErr))
		C.throwIllegalState(env, cErr)
		return C.JNI_FALSE
	}

	return C.JNI_TRUE
}

//export Java_dev_phonebridge_bridge_GoBridge_nativeClipboardStop
func Java_dev_phonebridge_bridge_GoBridge_nativeClipboardStop(env *C.JNIEnv, clazz C.jclass) {
	_ = clazz
	defer clipboardRecover(env, "clipboardStop")

	currentClipboardBridge().Stop()

	clipboardHostMu.Lock()
	if currentJniHost != nil && C.isNull(currentJniHost.callbackObj) == 0 {
		C.deleteGlobalRef(env, currentJniHost.callbackObj)
		currentJniHost.callbackObj = C.nullObject()
	}
	currentJniHost = nil
	clipboardHostMu.Unlock()
}

//export Java_dev_phonebridge_bridge_GoBridge_nativeClipboardOnLocalCopy
func Java_dev_phonebridge_bridge_GoBridge_nativeClipboardOnLocalCopy(env *C.JNIEnv, clazz C.jclass, jMime C.jstring, jPayload C.jbyteArray, jCopiedAtMs C.jlong) C.jboolean {
	_ = clazz
	defer clipboardRecover(env, "clipboardOnLocalCopy")

	mime := goString(env, jMime)
	payload := cBytes(env, jPayload)
	if mime == "" || payload == nil {
		return C.JNI_FALSE
	}

	err := currentClipboardBridge().OnLocalCopy(mime, payload, int64(jCopiedAtMs))
	if err != nil {
		return C.JNI_FALSE
	}
	return C.JNI_TRUE
}

//export Java_dev_phonebridge_bridge_GoBridge_nativeClipboardOnRemoteBytes
func Java_dev_phonebridge_bridge_GoBridge_nativeClipboardOnRemoteBytes(env *C.JNIEnv, clazz C.jclass, jPayload C.jbyteArray) C.jboolean {
	_ = clazz
	defer clipboardRecover(env, "clipboardOnRemoteBytes")

	bytes := cBytes(env, jPayload)
	if bytes == nil {
		return C.JNI_FALSE
	}

	err := currentClipboardBridge().OnRemoteBytes(bytes)
	if err != nil {
		return C.JNI_FALSE
	}
	return C.JNI_TRUE
}

//export Java_dev_phonebridge_bridge_GoBridge_nativeClipboardStats
func Java_dev_phonebridge_bridge_GoBridge_nativeClipboardStats(env *C.JNIEnv, clazz C.jclass) C.jbyteArray {
	_ = clazz
	defer clipboardRecover(env, "clipboardStats")
	return goBytesToJava(env, currentClipboardBridge().StatsJSON())
}
