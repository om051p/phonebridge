# Spike 03 — Android MediaProjection → Surface → MediaCodec (hardware H.264)
#
# Isolated prototype. Requires: Android SDK + JDK 17 + a connected device
# (physical device is strongly preferred; encoders differ wildly on emulators).
#
#   make build install     # build + install the prototype APK
#   make device            # dump device/SoC/Android/thermal context
#   make caps              # enumerate encoder capabilities (no consent needed)
#   make session           # one capture session at W/H/FPS/BITRATE/DURATION
#   make lifecycle         # capture + encoder lifecycle/restart/FLAG_SECURE probes
#   make matrix            # resolution/fps sweep (one consent per config)
#   make screenoff         # capture with the screen switched off mid-session
#   make frhint            # capture with Surface.setFrameRate() hint
#   make clean
#
# Override anything on the command line, e.g.
#   make session W=1280 H=720 FPS=60 DURATION=15

SHELL := /bin/bash
DRIVER := tools/spike03.sh

W ?= 1080
H ?= 2400
FPS ?= 30
BITRATE ?= 6000000
BITRATE_MODE ?= cbr
DURATION ?= 10
CYCLES ?= 20
LABEL ?=
SCOPE ?= Entire screen
export W H FPS BITRATE BITRATE_MODE DURATION CYCLES LABEL SCOPE

.PHONY: all build build34 install device caps baseline session vbr lifecycle matrix perf screenoff frhint adapt landscape fgsfirst pull logs clean

all: build install caps

build:
	$(DRIVER) build

build34:
	$(DRIVER) build34

baseline:
	$(DRIVER) baseline

vbr:
	$(DRIVER) vbr

fgsfirst:
	$(DRIVER) fgsfirst

install:
	$(DRIVER) install

device:
	$(DRIVER) device

caps:
	$(DRIVER) caps

session:
	$(DRIVER) session

lifecycle:
	$(DRIVER) lifecycle

matrix perf:
	$(DRIVER) matrix

screenoff:
	$(DRIVER) screenoff

frhint:
	$(DRIVER) frhint

frhint-always:
	$(DRIVER) frhint-always

adapt:
	$(DRIVER) adapt

landscape:
	$(DRIVER) landscape

pull:
	$(DRIVER) pull

logs:
	$(DRIVER) logs

clean:
	$(DRIVER) clean
