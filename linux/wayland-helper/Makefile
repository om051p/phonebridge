CC ?= gcc
CFLAGS ?= -O2 -Wall -Wextra -Werror -std=c11 -pedantic -D_GNU_SOURCE
WAYLAND_FLAGS ?= $(shell pkg-config --cflags --libs wayland-client)

TARGET = phonebridge-wayland-helper

SRCS = phonebridge-wayland-helper.c wlr-data-control-protocol.c

.PHONY: all clean

all: $(TARGET)

$(TARGET): $(SRCS)
	$(CC) $(CFLAGS) -I. $(SRCS) -o $@ $(WAYLAND_FLAGS)

clean:
	rm -f $(TARGET)
