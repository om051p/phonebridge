CC ?= gcc
CFLAGS ?= -O2 -Wall -Wextra
WAYLAND_FLAGS = $(shell pkg-config --cflags --libs wayland-client)

all: bin/data_control_probe bin/focus_helper bin/security_context_runner

bin/data_control_probe: src/data_control_probe.c src/wlr-data-control-protocol.c
	@mkdir -p bin
	$(CC) $(CFLAGS) -Isrc src/data_control_probe.c src/wlr-data-control-protocol.c -o $@ $(WAYLAND_FLAGS)

bin/focus_helper: src/focus_helper.c src/xdg-shell-protocol.c
	@mkdir -p bin
	$(CC) $(CFLAGS) -Isrc src/focus_helper.c src/xdg-shell-protocol.c -o $@ $(WAYLAND_FLAGS)

bin/security_context_runner: src/security_context_runner.c src/security-context-protocol.c src/wlr-data-control-protocol.c
	@mkdir -p bin
	$(CC) $(CFLAGS) -Isrc src/security_context_runner.c src/security-context-protocol.c src/wlr-data-control-protocol.c -o $@ $(WAYLAND_FLAGS)

clean:
	rm -rf bin/
