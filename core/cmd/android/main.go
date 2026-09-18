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
	_ = env
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

	if streamCancel != nil {
		streamCancel()
	}
	streamWg.Wait()

	engineState.Store(StateStopped)
	initialized.Store(false)
	return C.JNI_TRUE
}
