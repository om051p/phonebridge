//go:build android || jni

package main

/*
#include <jni.h>
#include <stdlib.h>
#include <string.h>

static inline int pbNotifIsNull(jobject obj) {
    return obj == NULL ? 1 : 0;
}

static inline const char* pbNotifGetUTFChars(JNIEnv *env, jstring str) {
    if (!env || !str) return NULL;
    return (*env)->GetStringUTFChars(env, str, NULL);
}

static inline void pbNotifReleaseUTFChars(JNIEnv *env, jstring str, const char *chars) {
    if (env && str && chars) {
        (*env)->ReleaseStringUTFChars(env, str, chars);
    }
}

static inline void pbNotifThrowIllegalState(JNIEnv *env, const char *msg) {
    jclass cls = (*env)->FindClass(env, "java/lang/IllegalStateException");
    if (cls != NULL) {
        (*env)->ThrowNew(env, cls, msg);
        (*env)->DeleteLocalRef(env, cls);
    }
}

static inline jstring pbNotifNewStringUTF(JNIEnv *env, const char *chars) {
    if (!env || !chars) return NULL;
    return (*env)->NewStringUTF(env, chars);
}

static inline int pbNotifIsNullMethod(jmethodID mid) {
    return mid == NULL ? 1 : 0;
}

static inline jobject pbNotifMakeGlobalRef(JNIEnv *env, jobject obj) {
    return (*env)->NewGlobalRef(env, obj);
}

static inline void pbNotifDeleteGlobalRef(JNIEnv *env, jobject obj) {
    if (env && obj) {
        (*env)->DeleteGlobalRef(env, obj);
    }
}

static inline jint pbNotifGetJavaVM(JNIEnv *env, JavaVM **jvm) {
    return (*env)->GetJavaVM(env, jvm);
}

static inline jint pbNotifGetOrAttachEnv(JavaVM *jvm, JNIEnv **env, int *didAttach) {
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

static inline void pbNotifReleaseEnv(JavaVM *jvm, int didAttach) {
    if (didAttach) {
        (*jvm)->DetachCurrentThread(jvm);
    }
}

static inline jmethodID resolveNotificationDismissMethod(JNIEnv *env, jobject host) {
    if (!env || !host) return NULL;
    jclass cls = (*env)->GetObjectClass(env, host);
    if (!cls) return NULL;
    jmethodID mid = (*env)->GetMethodID(env, cls, "onDismiss", "(Ljava/lang/String;)Z");
    (*env)->DeleteLocalRef(env, cls);
    return mid;
}

static inline jboolean callNotificationDismiss(JNIEnv *env, jobject host, jmethodID mid, const char *key) {
    if (!env || !host || !mid) return JNI_FALSE;
    jstring jKey = (*env)->NewStringUTF(env, key ? key : "");
    jboolean res = (*env)->CallBooleanMethod(env, host, mid, jKey);
    if (jKey) {
        (*env)->DeleteLocalRef(env, jKey);
    }
    return res;
}
*/
import "C"

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"
)

var (
	notifJvm            *C.JavaVM
	notifHostMu         sync.Mutex
	currentJniNotifHost *jniNotificationHost
)

type jniNotificationHost struct {
	callbackObj C.jobject
	dismissMid  C.jmethodID
}

func (h *jniNotificationHost) OnDismiss(key string) bool {
	if notifJvm == nil || C.pbNotifIsNull(h.callbackObj) == 1 {
		return false
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var env *C.JNIEnv
	var didAttach C.int
	if C.pbNotifGetOrAttachEnv(notifJvm, &env, &didAttach) != 0 || env == nil {
		return false
	}
	defer C.pbNotifReleaseEnv(notifJvm, didAttach)

	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))

	ok := C.callNotificationDismiss(env, h.callbackObj, h.dismissMid, cKey)
	return ok == 1
}

func notifGoString(env *C.JNIEnv, s C.jstring) string {
	if C.pbNotifIsNull(C.jobject(s)) == 1 {
		return ""
	}
	chars := C.pbNotifGetUTFChars(env, s)
	if chars == nil {
		return ""
	}
	out := C.GoString(chars)
	C.pbNotifReleaseUTFChars(env, s, chars)
	return out
}

func notifRecover(env *C.JNIEnv, what string) {
	if r := recover(); r != nil {
		errStr := fmt.Sprintf("%s: Go panic recovered: %v", what, r)
		cErr := C.CString(errStr)
		defer C.free(unsafe.Pointer(cErr))
		C.pbNotifThrowIllegalState(env, cErr)
	}
}

//export Java_dev_phonebridge_bridge_GoBridge_nativeNotificationInit
func Java_dev_phonebridge_bridge_GoBridge_nativeNotificationInit(env *C.JNIEnv, clazz C.jclass, jCallback C.jobject) C.jboolean {
	_ = clazz
	defer notifRecover(env, "notificationInit")

	var host NotificationHost
	if C.pbNotifIsNull(jCallback) == 0 {
		if notifJvm == nil {
			if C.pbNotifGetJavaVM(env, &notifJvm) != 0 || notifJvm == nil {
				C.pbNotifThrowIllegalState(env, C.CString("Failed to obtain JavaVM reference"))
				return C.JNI_FALSE
			}
		}

		dismissMid := C.resolveNotificationDismissMethod(env, jCallback)
		if C.pbNotifIsNullMethod(dismissMid) == 1 {
			C.pbNotifThrowIllegalState(env, C.CString("Failed to resolve NotificationHostCallback.onDismiss"))
			return C.JNI_FALSE
		}

		gRef := C.pbNotifMakeGlobalRef(env, jCallback)
		if C.pbNotifIsNull(gRef) == 1 {
			C.pbNotifThrowIllegalState(env, C.CString("Failed to create global ref for notification host"))
			return C.JNI_FALSE
		}

		notifHostMu.Lock()
		if currentJniNotifHost != nil && C.pbNotifIsNull(currentJniNotifHost.callbackObj) == 0 {
			C.pbNotifDeleteGlobalRef(env, currentJniNotifHost.callbackObj)
		}
		currentJniNotifHost = &jniNotificationHost{
			callbackObj: gRef,
			dismissMid:  dismissMid,
		}
		host = currentJniNotifHost
		notifHostMu.Unlock()
	}

	bridge := currentNotificationBridge()
	if err := bridge.Init(host); err != nil {
		cErr := C.CString(err.Error())
		defer C.free(unsafe.Pointer(cErr))
		C.pbNotifThrowIllegalState(env, cErr)
		return C.JNI_FALSE
	}
	return C.JNI_TRUE
}

//export Java_dev_phonebridge_bridge_GoBridge_nativeNotificationStop
func Java_dev_phonebridge_bridge_GoBridge_nativeNotificationStop(env *C.JNIEnv, clazz C.jclass) {
	_ = clazz
	defer notifRecover(env, "notificationStop")

	currentNotificationBridge().Stop()

	notifHostMu.Lock()
	if currentJniNotifHost != nil && C.pbNotifIsNull(currentJniNotifHost.callbackObj) == 0 {
		C.pbNotifDeleteGlobalRef(env, currentJniNotifHost.callbackObj)
		currentJniNotifHost = nil
	}
	notifHostMu.Unlock()
}

//export Java_dev_phonebridge_bridge_GoBridge_nativeNotificationPost
func Java_dev_phonebridge_bridge_GoBridge_nativeNotificationPost(
	env *C.JNIEnv, clazz C.jclass,
	jKey C.jstring,
	jPackageName C.jstring,
	jAppName C.jstring,
	jTitle C.jstring,
	jText C.jstring,
	jSubText C.jstring,
	jPostTimeMs C.jlong,
	jIsOngoing C.jboolean,
	jIsClearable C.jboolean,
	jCategory C.jstring,
) C.jboolean {
	_ = clazz
	defer notifRecover(env, "notificationPost")

	key := notifGoString(env, jKey)
	packageName := notifGoString(env, jPackageName)
	appName := notifGoString(env, jAppName)
	title := notifGoString(env, jTitle)
	text := notifGoString(env, jText)
	subText := notifGoString(env, jSubText)
	postTimeMs := int64(jPostTimeMs)
	isOngoing := jIsOngoing == C.JNI_TRUE
	isClearable := jIsClearable == C.JNI_TRUE
	category := notifGoString(env, jCategory)

	bridge := currentNotificationBridge()
	if err := bridge.PostNotification(
		key, packageName, appName, title, text, subText,
		postTimeMs, isOngoing, isClearable, category,
	); err != nil {
		return C.JNI_FALSE
	}
	return C.JNI_TRUE
}

//export Java_dev_phonebridge_bridge_GoBridge_nativeNotificationRemove
func Java_dev_phonebridge_bridge_GoBridge_nativeNotificationRemove(
	env *C.JNIEnv, clazz C.jclass,
	jKey C.jstring,
	jPackageName C.jstring,
	jReason C.jint,
) C.jboolean {
	_ = clazz
	defer notifRecover(env, "notificationRemove")

	key := notifGoString(env, jKey)
	packageName := notifGoString(env, jPackageName)
	reason := int32(jReason)

	bridge := currentNotificationBridge()
	if err := bridge.RemoveNotification(key, packageName, reason); err != nil {
		return C.JNI_FALSE
	}
	return C.JNI_TRUE
}

//export Java_dev_phonebridge_bridge_GoBridge_nativeNotificationStats
func Java_dev_phonebridge_bridge_GoBridge_nativeNotificationStats(env *C.JNIEnv, clazz C.jclass) C.jstring {
	_ = clazz
	defer notifRecover(env, "notificationStats")

	bridge := currentNotificationBridge()
	b := bridge.NotificationStatsJSON()
	cs := C.CString(string(b))
	defer C.free(unsafe.Pointer(cs))
	return C.pbNotifNewStringUTF(env, cs)
}
