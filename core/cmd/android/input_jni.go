//go:build android || jni

package main

/*
#include <jni.h>
#include <stdlib.h>
#include <string.h>

static inline int pbInputIsNull(jobject obj) {
    return obj == NULL ? 1 : 0;
}

static inline int pbInputIsNullMethod(jmethodID mid) {
    return mid == NULL ? 1 : 0;
}

static inline jobject pbInputMakeGlobalRef(JNIEnv *env, jobject obj) {
    return (*env)->NewGlobalRef(env, obj);
}

static inline void pbInputDeleteGlobalRef(JNIEnv *env, jobject obj) {
    if (env && obj) {
        (*env)->DeleteGlobalRef(env, obj);
    }
}

static inline jint pbInputGetJavaVM(JNIEnv *env, JavaVM **jvm) {
    return (*env)->GetJavaVM(env, jvm);
}

static inline jint pbInputGetOrAttachEnv(JavaVM *jvm, JNIEnv **env, int *didAttach) {
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

static inline void pbInputReleaseEnv(JavaVM *jvm, int didAttach) {
    if (didAttach) {
        (*jvm)->DetachCurrentThread(jvm);
    }
}

static inline void pbInputThrowIllegalState(JNIEnv *env, const char *msg) {
    jclass cls = (*env)->FindClass(env, "java/lang/IllegalStateException");
    if (cls != NULL) {
        (*env)->ThrowNew(env, cls, msg);
        (*env)->DeleteLocalRef(env, cls);
    }
}

static inline jmethodID resolveInputTouchMethod(JNIEnv *env, jobject host) {
    if (!env || !host) return NULL;
    jclass cls = (*env)->GetObjectClass(env, host);
    if (!cls) return NULL;
    jmethodID mid = (*env)->GetMethodID(env, cls, "onTouch", "(IIFFF)Z");
    (*env)->DeleteLocalRef(env, cls);
    return mid;
}

static inline jmethodID resolveInputKeyMethod(JNIEnv *env, jobject host) {
    if (!env || !host) return NULL;
    jclass cls = (*env)->GetObjectClass(env, host);
    if (!cls) return NULL;
    jmethodID mid = (*env)->GetMethodID(env, cls, "onKey", "(III)Z");
    (*env)->DeleteLocalRef(env, cls);
    return mid;
}

static inline jmethodID resolveInputTextMethod(JNIEnv *env, jobject host) {
    if (!env || !host) return NULL;
    jclass cls = (*env)->GetObjectClass(env, host);
    if (!cls) return NULL;
    jmethodID mid = (*env)->GetMethodID(env, cls, "onText", "(Ljava/lang/String;)Z");
    (*env)->DeleteLocalRef(env, cls);
    return mid;
}

static inline jmethodID resolveInputScrollMethod(JNIEnv *env, jobject host) {
    if (!env || !host) return NULL;
    jclass cls = (*env)->GetObjectClass(env, host);
    if (!cls) return NULL;
    jmethodID mid = (*env)->GetMethodID(env, cls, "onScroll", "(FFFF)Z");
    (*env)->DeleteLocalRef(env, cls);
    return mid;
}

static inline jmethodID resolveInputGlobalActionMethod(JNIEnv *env, jobject host) {
    if (!env || !host) return NULL;
    jclass cls = (*env)->GetObjectClass(env, host);
    if (!cls) return NULL;
    jmethodID mid = (*env)->GetMethodID(env, cls, "onGlobalAction", "(I)Z");
    (*env)->DeleteLocalRef(env, cls);
    return mid;
}

static inline int callInputTouch(JNIEnv *env, jobject host, jmethodID mid, jint action, jint pointerId, jfloat normX, jfloat normY, jfloat pressure) {
    if (!env || !host || !mid) return 0;
    jboolean res = (*env)->CallBooleanMethod(env, host, mid, action, pointerId, normX, normY, pressure);
    return res == JNI_TRUE ? 1 : 0;
}

static inline int callInputKey(JNIEnv *env, jobject host, jmethodID mid, jint action, jint keyCode, jint metaState) {
    if (!env || !host || !mid) return 0;
    jboolean res = (*env)->CallBooleanMethod(env, host, mid, action, keyCode, metaState);
    return res == JNI_TRUE ? 1 : 0;
}

static inline int callInputText(JNIEnv *env, jobject host, jmethodID mid, const char *text) {
    if (!env || !host || !mid) return 0;
    jstring jStr = (*env)->NewStringUTF(env, text);
    jboolean res = (*env)->CallBooleanMethod(env, host, mid, jStr);
    if (jStr) (*env)->DeleteLocalRef(env, jStr);
    return res == JNI_TRUE ? 1 : 0;
}

static inline int callInputScroll(JNIEnv *env, jobject host, jmethodID mid, jfloat normX, jfloat normY, jfloat deltaX, jfloat deltaY) {
    if (!env || !host || !mid) return 0;
    jboolean res = (*env)->CallBooleanMethod(env, host, mid, normX, normY, deltaX, deltaY);
    return res == JNI_TRUE ? 1 : 0;
}

static inline int callInputGlobalAction(JNIEnv *env, jobject host, jmethodID mid, jint actionType) {
    if (!env || !host || !mid) return 0;
    jboolean res = (*env)->CallBooleanMethod(env, host, mid, actionType);
    return res == JNI_TRUE ? 1 : 0;
}
*/
import "C"

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"
)

type jniInputHost struct {
	callbackObj     C.jobject
	touchMid        C.jmethodID
	keyMid          C.jmethodID
	textMid         C.jmethodID
	scrollMid       C.jmethodID
	globalActionMid C.jmethodID
}

var (
	inputHostMu        sync.Mutex
	currentJniInputHost *jniInputHost
)

func (h *jniInputHost) OnTouch(action int32, pointerID uint32, normX, normY, pressure float32) bool {
	if jvm == nil || C.pbInputIsNull(h.callbackObj) == 1 {
		return false
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var env *C.JNIEnv
	var didAttach C.int
	if C.pbInputGetOrAttachEnv(jvm, &env, &didAttach) != 0 || env == nil {
		return false
	}
	defer C.pbInputReleaseEnv(jvm, didAttach)

	ok := C.callInputTouch(env, h.callbackObj, h.touchMid, C.jint(action), C.jint(pointerID), C.jfloat(normX), C.jfloat(normY), C.jfloat(pressure))
	return ok == 1
}

func (h *jniInputHost) OnKey(action int32, keyCode int32, metaState uint32) bool {
	if jvm == nil || C.pbInputIsNull(h.callbackObj) == 1 {
		return false
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var env *C.JNIEnv
	var didAttach C.int
	if C.pbInputGetOrAttachEnv(jvm, &env, &didAttach) != 0 || env == nil {
		return false
	}
	defer C.pbInputReleaseEnv(jvm, didAttach)

	ok := C.callInputKey(env, h.callbackObj, h.keyMid, C.jint(action), C.jint(keyCode), C.jint(metaState))
	return ok == 1
}

func (h *jniInputHost) OnText(text string) bool {
	if jvm == nil || C.pbInputIsNull(h.callbackObj) == 1 {
		return false
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var env *C.JNIEnv
	var didAttach C.int
	if C.pbInputGetOrAttachEnv(jvm, &env, &didAttach) != 0 || env == nil {
		return false
	}
	defer C.pbInputReleaseEnv(jvm, didAttach)

	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))

	ok := C.callInputText(env, h.callbackObj, h.textMid, cText)
	return ok == 1
}

func (h *jniInputHost) OnScroll(normX, normY, deltaX, deltaY float32) bool {
	if jvm == nil || C.pbInputIsNull(h.callbackObj) == 1 {
		return false
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var env *C.JNIEnv
	var didAttach C.int
	if C.pbInputGetOrAttachEnv(jvm, &env, &didAttach) != 0 || env == nil {
		return false
	}
	defer C.pbInputReleaseEnv(jvm, didAttach)

	ok := C.callInputScroll(env, h.callbackObj, h.scrollMid, C.jfloat(normX), C.jfloat(normY), C.jfloat(deltaX), C.jfloat(deltaY))
	return ok == 1
}

func (h *jniInputHost) OnGlobalAction(actionType int32) bool {
	if jvm == nil || C.pbInputIsNull(h.callbackObj) == 1 {
		return false
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var env *C.JNIEnv
	var didAttach C.int
	if C.pbInputGetOrAttachEnv(jvm, &env, &didAttach) != 0 || env == nil {
		return false
	}
	defer C.pbInputReleaseEnv(jvm, didAttach)

	ok := C.callInputGlobalAction(env, h.callbackObj, h.globalActionMid, C.jint(actionType))
	return ok == 1
}

func inputRecover(env *C.JNIEnv, what string) {
	if r := recover(); r != nil {
		panicsCaught.Add(1)
		errStr := fmt.Sprintf("%s: Go panic recovered: %v", what, r)
		cErr := C.CString(errStr)
		defer C.free(unsafe.Pointer(cErr))
		C.pbInputThrowIllegalState(env, cErr)
	}
}

//export Java_dev_phonebridge_bridge_GoBridge_nativeInputInit
func Java_dev_phonebridge_bridge_GoBridge_nativeInputInit(env *C.JNIEnv, clazz C.jclass, jCallback C.jobject) C.jboolean {
	_ = clazz
	defer inputRecover(env, "inputInit")

	if C.pbInputIsNull(jCallback) == 1 {
		C.pbInputThrowIllegalState(env, C.CString("input host callback cannot be null"))
		return C.JNI_FALSE
	}

	if jvm == nil {
		if C.pbInputGetJavaVM(env, &jvm) != 0 || jvm == nil {
			C.pbInputThrowIllegalState(env, C.CString("Failed to obtain JavaVM reference"))
			return C.JNI_FALSE
		}
	}

	touchMid := C.resolveInputTouchMethod(env, jCallback)
	keyMid := C.resolveInputKeyMethod(env, jCallback)
	textMid := C.resolveInputTextMethod(env, jCallback)
	scrollMid := C.resolveInputScrollMethod(env, jCallback)
	globalActionMid := C.resolveInputGlobalActionMethod(env, jCallback)

	if C.pbInputIsNullMethod(touchMid) == 1 || C.pbInputIsNullMethod(keyMid) == 1 ||
		C.pbInputIsNullMethod(textMid) == 1 || C.pbInputIsNullMethod(scrollMid) == 1 ||
		C.pbInputIsNullMethod(globalActionMid) == 1 {
		C.pbInputThrowIllegalState(env, C.CString("Failed to resolve InputHostCallback methods"))
		return C.JNI_FALSE
	}

	gRef := C.pbInputMakeGlobalRef(env, jCallback)
	if C.pbInputIsNull(gRef) == 1 {
		C.pbInputThrowIllegalState(env, C.CString("Failed to create global ref for input host"))
		return C.JNI_FALSE
	}

	inputHostMu.Lock()
	if currentJniInputHost != nil && C.pbInputIsNull(currentJniInputHost.callbackObj) == 0 {
		C.pbInputDeleteGlobalRef(env, currentJniInputHost.callbackObj)
	}
	currentJniInputHost = &jniInputHost{
		callbackObj:     gRef,
		touchMid:        touchMid,
		keyMid:          keyMid,
		textMid:         textMid,
		scrollMid:       scrollMid,
		globalActionMid: globalActionMid,
	}
	inputHostMu.Unlock()

	if err := currentInputBridge().Init(currentJniInputHost); err != nil {
		cErr := C.CString(err.Error())
		defer C.free(unsafe.Pointer(cErr))
		C.pbInputThrowIllegalState(env, cErr)
		return C.JNI_FALSE
	}

	return C.JNI_TRUE
}

//export Java_dev_phonebridge_bridge_GoBridge_nativeInputStop
func Java_dev_phonebridge_bridge_GoBridge_nativeInputStop(env *C.JNIEnv, clazz C.jclass) {
	_ = clazz
	defer inputRecover(env, "inputStop")

	currentInputBridge().Stop()

	inputHostMu.Lock()
	if currentJniInputHost != nil && C.pbInputIsNull(currentJniInputHost.callbackObj) == 0 {
		C.pbInputDeleteGlobalRef(env, currentJniInputHost.callbackObj)
		currentJniInputHost = nil
	}
	inputHostMu.Unlock()
}
