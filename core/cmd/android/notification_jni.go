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
*/
import "C"

import (
	"fmt"
	"unsafe"
)

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
func Java_dev_phonebridge_bridge_GoBridge_nativeNotificationInit(env *C.JNIEnv, clazz C.jclass) C.jboolean {
	_ = clazz
	defer notifRecover(env, "notificationInit")

	bridge := currentNotificationBridge()
	if err := bridge.Init(); err != nil {
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

	bridge := currentNotificationBridge()
	bridge.Stop()
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
