//go:build android || jni

package main

/*
#include <jni.h>
#include <stdlib.h>
#include <string.h>

// ---------------------------------------------------------------------------
// Transfer storage-host JNI glue (DEC-024). These are the ONLY non-static
// definitions in this package's preambles, and they live here — not in
// main.go — because cgo recompiles each file's preamble into its own
// translation unit and also copies non-static definitions into the generated
// _cgo_export.h. A definition in main.go would therefore be compiled twice
// (main.cgo2.c and _cgo_export.c) and fail to link. Defining them here keeps
// exactly one copy, and main.go reaches them through the prototypes below.
//
// Kotlin contract (AndroidTransferHost, see android/app/src/main/kotlin/
// dev/phonebridge/transfer/AndroidTransferHost.kt):
//
//	onBeginDownload(String filename, String mime, long size) -> String handle
//	onOpenPendingFd(String handle)                           -> int fd
//	onCommitDownload(String handle)                          -> String name
//	onAbortDownload(String handle)                           -> void
//	onFreeSpaceBytes()                                       -> long
//	onOversizedFrame(int size)                               -> void
// Every helper below is declared through PB_LINKAGE: cgo re-emits non-static
// preamble definitions into _cgo_export.c, which would duplicate them. Making
// them file-local `static` keeps one compiled copy and still satisfies every
// C.<name> reference in this file (all of them live here).
#define PB_LINKAGE static
// ---------------------------------------------------------------------------
static jobject nullObject();
static jobject makeGlobalRef(JNIEnv *env, jobject obj);
static void deleteGlobalRef(JNIEnv *env, jobject obj);
static jint getJavaVM(JNIEnv *env, JavaVM **jvm);
static jint getOrAttachEnv(JavaVM *jvm, JNIEnv **env, int *didAttach);
static void releaseEnv(JavaVM *jvm, int didAttach);
static void throwIllegalState(JNIEnv *env, const char *msg);
static int isNull(jobject obj);
static int isNullMethod(jmethodID mid);

static void pbDeleteLocalRef(JNIEnv *env, jobject obj) {
    if (env && obj) (*env)->DeleteLocalRef(env, obj);
}

// copyJavaString copies a Java String into a caller buffer (NUL terminated)
// and returns its length, or -1 when null/unavailable. The JVM chars are
// released immediately: only the copy escapes.
static int copyJavaString(JNIEnv *env, jstring str, char *out, int outLen) {
    if (!env || !str || !out || outLen <= 1) return -1;
    const char *chars = (*env)->GetStringUTFChars(env, str, NULL);
    if (!chars) return -1;
    int n = (int)strlen(chars);
    if (n > outLen - 1) n = outLen - 1;
    memcpy(out, chars, (size_t)n);
    out[n] = '\0';
    (*env)->ReleaseStringUTFChars(env, str, chars);
    return n;
}

PB_LINKAGE jmethodID resolveTransferBeginMethod(JNIEnv *env, jobject host) {
    if (!env || !host) return NULL;
    jclass cls = (*env)->GetObjectClass(env, host);
    if (!cls) return NULL;
    jmethodID mid = (*env)->GetMethodID(env, cls, "onBeginDownload", "(Ljava/lang/String;Ljava/lang/String;J)Ljava/lang/String;");
    (*env)->DeleteLocalRef(env, cls);
    return mid;
}

PB_LINKAGE jmethodID resolveTransferOpenFdMethod(JNIEnv *env, jobject host) {
    if (!env || !host) return NULL;
    jclass cls = (*env)->GetObjectClass(env, host);
    if (!cls) return NULL;
    jmethodID mid = (*env)->GetMethodID(env, cls, "onOpenPendingFd", "(Ljava/lang/String;)I");
    (*env)->DeleteLocalRef(env, cls);
    return mid;
}

PB_LINKAGE jmethodID resolveTransferCommitMethod(JNIEnv *env, jobject host) {
    if (!env || !host) return NULL;
    jclass cls = (*env)->GetObjectClass(env, host);
    if (!cls) return NULL;
    jmethodID mid = (*env)->GetMethodID(env, cls, "onCommitDownload", "(Ljava/lang/String;)Ljava/lang/String;");
    (*env)->DeleteLocalRef(env, cls);
    return mid;
}

PB_LINKAGE jmethodID resolveTransferAbortMethod(JNIEnv *env, jobject host) {
    if (!env || !host) return NULL;
    jclass cls = (*env)->GetObjectClass(env, host);
    if (!cls) return NULL;
    jmethodID mid = (*env)->GetMethodID(env, cls, "onAbortDownload", "(Ljava/lang/String;)V");
    (*env)->DeleteLocalRef(env, cls);
    return mid;
}

PB_LINKAGE jmethodID resolveTransferFreeSpaceMethod(JNIEnv *env, jobject host) {
    if (!env || !host) return NULL;
    jclass cls = (*env)->GetObjectClass(env, host);
    if (!cls) return NULL;
    jmethodID mid = (*env)->GetMethodID(env, cls, "onFreeSpaceBytes", "()J");
    (*env)->DeleteLocalRef(env, cls);
    return mid;
}

PB_LINKAGE jmethodID resolveTransferOversizedMethod(JNIEnv *env, jobject host) {
    if (!env || !host) return NULL;
    jclass cls = (*env)->GetObjectClass(env, host);
    if (!cls) return NULL;
    jmethodID mid = (*env)->GetMethodID(env, cls, "onOversizedFrame", "(I)V");
    (*env)->DeleteLocalRef(env, cls);
    return mid;
}

PB_LINKAGE int callTransferBegin(JNIEnv *env, jobject host, jmethodID mid, const char *filename, const char *mime, jlong size, char *out, int outLen) {
    if (!env || !host || !mid) return -1;
    jstring jFile = (*env)->NewStringUTF(env, filename);
    jstring jMime = (*env)->NewStringUTF(env, mime);
    jstring res = (jstring)(*env)->CallObjectMethod(env, host, mid, jFile, jMime, size);
    pbDeleteLocalRef(env, (jobject)jFile);
    pbDeleteLocalRef(env, (jobject)jMime);
    if (!res) return -1;
    int n = copyJavaString(env, res, out, outLen);
    pbDeleteLocalRef(env, (jobject)res);
    return n;
}

PB_LINKAGE int callTransferOpenFd(JNIEnv *env, jobject host, jmethodID mid, const char *handle) {
    if (!env || !host || !mid) return -1;
    jstring jHandle = (*env)->NewStringUTF(env, handle);
    jint fd = (*env)->CallIntMethod(env, host, mid, jHandle);
    pbDeleteLocalRef(env, (jobject)jHandle);
    return (int)fd;
}

PB_LINKAGE int callTransferCommit(JNIEnv *env, jobject host, jmethodID mid, const char *handle, char *out, int outLen) {
    if (!env || !host || !mid) return -1;
    jstring jHandle = (*env)->NewStringUTF(env, handle);
    jstring res = (jstring)(*env)->CallObjectMethod(env, host, mid, jHandle);
    pbDeleteLocalRef(env, (jobject)jHandle);
    if (!res) return -1;
    int n = copyJavaString(env, res, out, outLen);
    pbDeleteLocalRef(env, (jobject)res);
    return n;
}

PB_LINKAGE void callTransferAbort(JNIEnv *env, jobject host, jmethodID mid, const char *handle) {
    if (!env || !host || !mid) return;
    jstring jHandle = (*env)->NewStringUTF(env, handle);
    (*env)->CallVoidMethod(env, host, mid, jHandle);
    pbDeleteLocalRef(env, (jobject)jHandle);
}

PB_LINKAGE jlong callTransferFreeSpace(JNIEnv *env, jobject host, jmethodID mid) {
    if (!env || !host || !mid) return -1;
    return (*env)->CallLongMethod(env, host, mid);
}

PB_LINKAGE void callTransferOversized(JNIEnv *env, jobject host, jmethodID mid, int size) {
    if (!env || !host || !mid) return;
    (*env)->CallVoidMethod(env, host, mid, (jint)size);
}
*/
import "C"

// JNI wiring of the transfer plane (DEC-024). The storage host that Kotlin
// registers through nativeTransferInit implements TransferHost by calling back
// into the JVM with the flat method set declared in the C preamble above.
//
// This file is glue only: every transfer decision (limits, staging,
// verification, buffering) belongs to core/pkg/transfer, and every storage
// decision belongs to Kotlin's AndroidTransferHost. The host holds no state of
// its own beyond the global JVM reference, so a service restart can
// re-register it freely.

import (
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"github.com/om051p/phonebridge/core/pkg/transfer"
)

// transferHostMu guards currentJniTransferHost, mirroring clipboardHostMu.
var (
	transferHostMu         sync.Mutex
	currentJniTransferHost *jniTransferHost
)

// jniTransferHost holds the global ref and pre-resolved method IDs for the
// Kotlin storage host. Method IDs belong to the object's class and stay valid
// for the JVM's lifetime; the global ref keeps the host itself alive across JNI
// calls from arbitrary Go goroutines.
type jniTransferHost struct {
	callbackObj C.jobject
	beginMid    C.jmethodID
	openFdMid   C.jmethodID
	commitMid   C.jmethodID
	abortMid    C.jmethodID
	freeMid     C.jmethodID
	oversizeMid C.jmethodID
}

// attachEnv is the common JVM entry for host callbacks: lock the OS thread
// (JNI envs are thread-bound), attach if needed, and detach on return.
func attachEnv() (*C.JNIEnv, func()) {
	if jvm == nil {
		return nil, func() {}
	}
	runtime.LockOSThread()
	var env *C.JNIEnv
	var didAttach C.int
	if C.getOrAttachEnv(jvm, &env, &didAttach) != 0 || env == nil {
		runtime.UnlockOSThread()
		return nil, func() { runtime.UnlockOSThread() }
	}
	release := func() { C.releaseEnv(jvm, didAttach); runtime.UnlockOSThread() }
	return env, release
}

// BeginDownload asks Kotlin to create the pending destination entry and open a
// writable descriptor for it.
func (h *jniTransferHost) BeginDownload(filename, mimeType string, sizeBytes int64) (string, int, error) {
	if jvm == nil || C.isNull(h.callbackObj) == 1 {
		return "", -1, errors.New("transfer host not registered")
	}
	env, release := attachEnv()
	defer release()

	cFilename := C.CString(filename)
	defer C.free(unsafe.Pointer(cFilename))
	cMime := C.CString(mimeType)
	defer C.free(unsafe.Pointer(cMime))

	const handleBufLen = 512
	handleBuf := (*C.char)(C.malloc(handleBufLen))
	if handleBuf == nil {
		return "", -1, errors.New("out of memory")
	}
	defer C.free(unsafe.Pointer(handleBuf))

	n := C.callTransferBegin(env, h.callbackObj, h.beginMid, cFilename, cMime, C.jlong(sizeBytes), handleBuf, handleBufLen)
	if n < 0 {
		return "", -1, errors.New("Kotlin refused to create the pending download")
	}
	handle := C.GoStringN(handleBuf, n)
	if handle == "" {
		return "", -1, errors.New("Kotlin returned an empty transfer handle")
	}

	// The descriptor is opened in a second call: MediaStore row creation and
	// ContentResolver.openFileDescriptor are two separate steps on the Kotlin
	// side, and the handle is what ties them together.
	cHandle := C.CString(handle)
	defer C.free(unsafe.Pointer(cHandle))
	fd := C.callTransferOpenFd(env, h.callbackObj, h.openFdMid, cHandle)
	if fd < 0 {
		// The entry exists but cannot be written: delete it now so an aborted
		// begin never leaves a pending row behind.
		C.callTransferAbort(env, h.callbackObj, h.abortMid, cHandle)
		return handle, int(fd), errors.New("Kotlin could not open a descriptor for the pending download")
	}
	return handle, int(fd), nil
}

// CommitDownload asks Kotlin to publish (IS_PENDING=0) the finished file.
func (h *jniTransferHost) CommitDownload(handle string) (string, bool) {
	if jvm == nil || C.isNull(h.callbackObj) == 1 {
		return "", false
	}
	env, release := attachEnv()
	defer release()

	cHandle := C.CString(handle)
	defer C.free(unsafe.Pointer(cHandle))

	const nameBufLen = 512
	nameBuf := (*C.char)(C.malloc(nameBufLen))
	if nameBuf == nil {
		return "", false
	}
	defer C.free(unsafe.Pointer(nameBuf))

	n := C.callTransferCommit(env, h.callbackObj, h.commitMid, cHandle, nameBuf, nameBufLen)
	if n < 0 {
		return "", false
	}
	return C.GoStringN(nameBuf, n), true
}

// AbortDownload deletes the pending entry and every byte written to it.
func (h *jniTransferHost) AbortDownload(handle string) {
	if jvm == nil || C.isNull(h.callbackObj) == 1 {
		return
	}
	env, release := attachEnv()
	defer release()

	cHandle := C.CString(handle)
	defer C.free(unsafe.Pointer(cHandle))
	C.callTransferAbort(env, h.callbackObj, h.abortMid, cHandle)
}

// FreeSpaceBytes reports the destination volume's free space; negative means
// unknown, in which case the Go free-space policy is skipped rather than guessed.
func (h *jniTransferHost) FreeSpaceBytes() int64 {
	if jvm == nil || C.isNull(h.callbackObj) == 1 {
		return -1
	}
	env, release := attachEnv()
	defer release()
	return int64(C.callTransferFreeSpace(env, h.callbackObj, h.freeMid))
}

// OnOversizedFrame forwards a protocol-limit violation to the UI.
func (h *jniTransferHost) OnOversizedFrame(size int) {
	if jvm == nil || C.isNull(h.callbackObj) == 1 {
		return
	}
	env, release := attachEnv()
	defer release()
	C.callTransferOversized(env, h.callbackObj, h.oversizeMid, C.int(size))
}

func transferRecover(env *C.JNIEnv, what string) {
	if r := recover(); r != nil {
		panicsCaught.Add(1)
		errStr := fmt.Sprintf("%s: Go panic recovered: %v", what, r)
		cErr := C.CString(errStr)
		defer C.free(unsafe.Pointer(cErr))
		C.throwIllegalState(env, cErr)
	}
}

//export Java_dev_phonebridge_bridge_GoBridge_nativeTransferInit
func Java_dev_phonebridge_bridge_GoBridge_nativeTransferInit(env *C.JNIEnv, clazz C.jclass, jCallback C.jobject, jPeerID C.jstring) C.jboolean {
	_ = clazz
	defer transferRecover(env, "transferInit")

	if C.isNull(jCallback) == 1 {
		C.throwIllegalState(env, C.CString("transfer host callback cannot be null"))
		return C.JNI_FALSE
	}
	if jvm == nil {
		if C.getJavaVM(env, &jvm) != 0 || jvm == nil {
			C.throwIllegalState(env, C.CString("Failed to obtain JavaVM reference"))
			return C.JNI_FALSE
		}
	}

	beginMid := C.resolveTransferBeginMethod(env, jCallback)
	openFdMid := C.resolveTransferOpenFdMethod(env, jCallback)
	commitMid := C.resolveTransferCommitMethod(env, jCallback)
	abortMid := C.resolveTransferAbortMethod(env, jCallback)
	freeMid := C.resolveTransferFreeSpaceMethod(env, jCallback)
	oversizeMid := C.resolveTransferOversizedMethod(env, jCallback)

	if C.isNullMethod(beginMid) == 1 || C.isNullMethod(openFdMid) == 1 ||
		C.isNullMethod(commitMid) == 1 || C.isNullMethod(abortMid) == 1 ||
		C.isNullMethod(freeMid) == 1 || C.isNullMethod(oversizeMid) == 1 {
		C.throwIllegalState(env, C.CString("Failed to resolve TransferHost methods"))
		return C.JNI_FALSE
	}

	gRef := C.makeGlobalRef(env, jCallback)
	if C.isNull(gRef) == 1 {
		C.throwIllegalState(env, C.CString("Failed to create global ref for transfer host"))
		return C.JNI_FALSE
	}

	transferHostMu.Lock()
	if currentJniTransferHost != nil && C.isNull(currentJniTransferHost.callbackObj) == 0 {
		C.deleteGlobalRef(env, currentJniTransferHost.callbackObj)
	}
	currentJniTransferHost = &jniTransferHost{
		callbackObj: gRef,
		beginMid:    beginMid,
		openFdMid:   openFdMid,
		commitMid:   commitMid,
		abortMid:    abortMid,
		freeMid:     freeMid,
		oversizeMid: oversizeMid,
	}
	transferHostMu.Unlock()

	peerID := goString(env, jPeerID)
	if err := currentTransferBridge().Init(currentJniTransferHost, peerID); err != nil {
		cErr := C.CString(err.Error())
		defer C.free(unsafe.Pointer(cErr))
		C.throwIllegalState(env, cErr)
		return C.JNI_FALSE
	}
	return C.JNI_TRUE
}

//export Java_dev_phonebridge_bridge_GoBridge_nativeTransferStop
func Java_dev_phonebridge_bridge_GoBridge_nativeTransferStop(env *C.JNIEnv, clazz C.jclass) {
	_ = clazz
	defer transferRecover(env, "transferStop")

	currentTransferBridge().Stop()

	transferHostMu.Lock()
	if currentJniTransferHost != nil && C.isNull(currentJniTransferHost.callbackObj) == 0 {
		C.deleteGlobalRef(env, currentJniTransferHost.callbackObj)
		currentJniTransferHost.callbackObj = C.nullObject()
	}
	currentJniTransferHost = nil
	transferHostMu.Unlock()
}

//export Java_dev_phonebridge_bridge_GoBridge_nativeTransferSetPeer
func Java_dev_phonebridge_bridge_GoBridge_nativeTransferSetPeer(env *C.JNIEnv, clazz C.jclass, jPeerID C.jstring) C.jboolean {
	_ = clazz
	defer transferRecover(env, "transferSetPeer")
	peerID := goString(env, jPeerID)
	currentTransport().SetPeerDeviceID(peerID)
	return C.JNI_TRUE
}

// invokeTransfer routes the generic invoke("transfer:*") JSON surface. Keeping
// list/cancel/stats here (rather than as dedicated natives) follows the repo's
// control plane rule: low-frequency request/response goes through nativeInvoke;
// only high-throughput or callback-bearing paths get their own export.
func invokeTransfer(method string, payload []byte) ([]byte, bool) {
	const prefix = "transfer:"
	if len(method) <= len(prefix) || method[:len(prefix)] != prefix {
		return nil, false
	}
	bridge := currentTransferBridge()

	switch method[len(prefix):] {
	case "list":
		return bridge.ListJSON(), true
	case "stats":
		return bridge.StatsJSON(), true
	case "send":
		var req struct {
			Path     string `json:"path"`
			Filename string `json:"filename,omitempty"`
		}
		if err := json.Unmarshal(payload, &req); err != nil || req.Path == "" {
			return jsonError("path is required"), true
		}
		id, err := bridge.SendFile(req.Path, req.Filename)
		if err != nil {
			return jsonSendFailure(id, err), true
		}
		return mustJSON(map[string]any{"transfer_id": id, "state": "PENDING"}), true
	case "cancel":
		var req struct {
			TransferID string `json:"transfer_id"`
		}
		if err := json.Unmarshal(payload, &req); err != nil || req.TransferID == "" {
			return jsonError("transfer_id is required"), true
		}
		if err := bridge.Cancel(req.TransferID); err != nil {
			return jsonError(err.Error()), true
		}
		return mustJSON(map[string]any{"cancelled": true, "transfer_id": req.TransferID}), true
	default:
		return jsonError("unknown transfer method: " + method[len(prefix):]), true
	}
}

func mustJSON(v any) []byte {
	out, _ := json.Marshal(v)
	return out
}

func jsonError(message string) []byte {
	return mustJSON(map[string]any{"error": message})
}

// jsonSendFailure carries the typed reason taxonomy (DEC-024) in the JSON error,
// so the UI never has to parse a message string to classify a failure.
func jsonSendFailure(id string, err error) []byte {
	out := map[string]any{"error": err.Error()}
	if id != "" {
		out["transfer_id"] = id
	}
	if failure, ok := transfer.IsFailure(err); ok {
		out["reason"] = failure.Reason.String()
		out["code"] = failure.Code.String()
	}
	return mustJSON(out)
}
