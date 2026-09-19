# Spike 05 — Android clipboard/background behaviour (isolated prototype)
#
# All artifact paths are inside this spike. Nothing here touches production.
#
#   make apk       — build the spike + peer APKs (Gradle, isolated project)
#   make install   — adb install both APKs
#   make matrix    — foreground + background + FGS clipboard matrix
#   make listener  — clipboard-listener reachability per app state
#   make lifecycle — focus-transition behaviour
#   make autoclear — clipboard auto-clear timing
#   make all       — matrix + listener + lifecycle
#   make clean

S5_DIR  := $(patsubst %/,%,$(dir $(abspath $(lastword $(MAKEFILE_LIST)))))
ANDROID := $(S5_DIR)/android
OUT     ?= $(S5_DIR)/results
APP_APK := $(ANDROID)/app/build/outputs/apk/debug/app-debug.apk
SET_APK := $(ANDROID)/setter/build/outputs/apk/debug/setter-debug.apk

.PHONY: all apk install matrix listener lifecycle autoclear clean

all: matrix listener lifecycle

apk:
	cd "$(ANDROID)" && ./gradlew --no-daemon :app:assembleDebug :setter:assembleDebug

install: apk
	@# HyperOS refuses `adb install` with INSTALL_FAILED_USER_RESTRICTED unless
	@# "Install via USB" is enabled in Developer options. Pushing and using
	@# `pm install` as the shell user is not subject to that gate, so it is the
	@# reliable path on this device class.
	adb push "$(APP_APK)" /data/local/tmp/spike05.apk >/dev/null
	adb shell pm install -r -t /data/local/tmp/spike05.apk
	adb push "$(SET_APK)" /data/local/tmp/spike05setter.apk >/dev/null
	adb shell pm install -r -t /data/local/tmp/spike05setter.apk

matrix:    ; S5_DIR="$(S5_DIR)" OUT="$(OUT)" bash "$(S5_DIR)/tools/spike05.sh" matrix
listener:  ; S5_DIR="$(S5_DIR)" OUT="$(OUT)" bash "$(S5_DIR)/tools/spike05.sh" listener
lifecycle: ; S5_DIR="$(S5_DIR)" OUT="$(OUT)" bash "$(S5_DIR)/tools/spike05.sh" lifecycle
autoclear: ; S5_DIR="$(S5_DIR)" OUT="$(OUT)" bash "$(S5_DIR)/tools/spike05.sh" autoclear

clean:
	cd "$(ANDROID)" && ./gradlew --no-daemon clean || true
	rm -rf "$(OUT)/device"
