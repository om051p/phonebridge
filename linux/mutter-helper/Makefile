CC ?= gcc
CFLAGS ?= -O2 -Wall -Wextra -Werror -std=c11 -pedantic -D_GNU_SOURCE
GIO_FLAGS ?= $(shell pkg-config --cflags --libs gio-2.0 gio-unix-2.0)

TARGET = phonebridge-mutter-helper

SRCS = phonebridge-mutter-helper.c

.PHONY: all clean

all: $(TARGET)

$(TARGET): $(SRCS)
	$(CC) $(CFLAGS) -I. $(SRCS) -o $@ $(GIO_FLAGS)

clean:
	rm -f $(TARGET)
