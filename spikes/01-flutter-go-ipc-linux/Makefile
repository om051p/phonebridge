# Spike 01 — Flutter ↔ Go local IPC on Linux (EXPERIMENTAL, throwaway).
#
# Isolated from the product: its own proto module, its own pinned plugins, its
# own go.mod, and its own Flutter test harness. Nothing here feeds CI codegen.
#
# Toolchain note: this spike was developed with a staged user-local toolchain
# (Go, buf). Put them on PATH before invoking make, e.g.:
#   export PATH=$HOME/.local/spike01-toolchain/go/bin:$HOME/.local/spike01-toolchain/bin:$HOME/.pub-cache/bin:$PATH
#   export GOTOOLCHAIN=local

SPIKE_DIR := $(patsubst %/,%,$(dir $(abspath $(lastword $(MAKEFILE_LIST)))))
HARNESS := $(SPIKE_DIR)/harness

.PHONY: help proto build test perf analyze clean all
.DEFAULT_GOAL := help

help:
	@echo 'Spike 01 targets:'
	@echo '  proto   - buf lint + generate spike-local Go/Dart stubs'
	@echo '  build   - compile spiked (daemon) and libphonebridge_spike.so'
	@echo '  analyze - flutter analyze on the harness'
	@echo '  test    - functional suites (uds_grpc, security, lifecycle)'
	@echo '  perf    - perf suites; writes JSON evidence to results/'
	@echo '  all     - proto, build, analyze, test, perf'
	@echo '  clean   - remove binaries and codegen output'

proto:
	cd $(SPIKE_DIR)/proto && buf lint && buf generate

build:
	cd $(SPIKE_DIR)/go && \
	  CGO_ENABLED=1 go build -trimpath -o bin/phonebridge-spike-daemon ./cmd/spiked && \
	  CGO_ENABLED=1 go build -trimpath -buildmode=c-shared \
	    -o bin/libphonebridge_spike.so ./cshared && \
	  chmod 700 bin && chmod 700 bin/phonebridge-spike-daemon && \
	  chmod 600 bin/libphonebridge_spike.so bin/libphonebridge_spike.h

analyze:
	cd $(HARNESS) && flutter analyze

test:
	cd $(HARNESS) && flutter test test/uds_grpc_test.dart test/security_test.dart test/lifecycle_test.dart

perf:
	cd $(HARNESS) && flutter test test/perf_uds_grpc_test.dart --concurrency=1 && \
	  flutter test test/perf_ffi_test.dart --concurrency=1

all: proto build analyze test perf

clean:
	rm -rf $(SPIKE_DIR)/go/bin $(SPIKE_DIR)/results/*.json
	rm -rf $(SPIKE_DIR)/harness/lib/generated
	find $(SPIKE_DIR)/proto -type f -name '*.pb.go' -delete
	find $(SPIKE_DIR)/proto -type f -name '*.pb.dart' -delete
	find $(SPIKE_DIR)/proto -type f -name '*.pbenum.dart' -delete
	find $(SPIKE_DIR)/proto -type f -name '*.pbgrpc.dart' -delete
	find $(SPIKE_DIR)/proto -type f -name '*.pbjson.dart' -delete