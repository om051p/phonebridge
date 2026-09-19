package main

/*
#cgo pkg-config: wayland-client
#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdint.h>
#include <unistd.h>
#include <fcntl.h>
#include <wayland-client.h>
#include "wlr-data-control-client-protocol.h"

extern void goOnSelectionOffer(char *mimes);
extern void goOnSelectionCleared();

static void c_offer_offer(void *data, struct zwlr_data_control_offer_v1 *offer, const char *mime_type) {
    (void)data; (void)offer; (void)mime_type;
}

static const struct zwlr_data_control_offer_v1_listener c_offer_listener = {
    .offer = c_offer_offer,
};

static void c_device_data_offer(void *data, struct zwlr_data_control_device_v1 *device,
                               struct zwlr_data_control_offer_v1 *offer) {
    (void)data; (void)device;
    zwlr_data_control_offer_v1_add_listener(offer, &c_offer_listener, NULL);
}

static void c_device_selection(void *data, struct zwlr_data_control_device_v1 *device,
                              struct zwlr_data_control_offer_v1 *offer) {
    (void)data; (void)device;
    if (!offer) {
        goOnSelectionCleared();
        return;
    }
    goOnSelectionOffer("text/plain;charset=utf-8");
}

static void c_device_primary_selection(void *data, struct zwlr_data_control_device_v1 *device,
                                      struct zwlr_data_control_offer_v1 *offer) {
    (void)data; (void)device; (void)offer;
}

static const struct zwlr_data_control_device_v1_listener c_device_listener = {
    .data_offer = c_device_data_offer,
    .selection = c_device_selection,
    .primary_selection = c_device_primary_selection,
};

struct cgo_wayland_state {
    struct wl_display *display;
    struct wl_registry *registry;
    struct wl_seat *seat;
    struct zwlr_data_control_manager_v1 *manager;
    struct zwlr_data_control_device_v1 *device;
};

static void c_registry_global(void *data, struct wl_registry *registry,
                             uint32_t id, const char *interface, uint32_t version) {
    struct cgo_wayland_state *st = (struct cgo_wayland_state *)data;
    if (strcmp(interface, "wl_seat") == 0) {
        st->seat = wl_registry_bind(registry, id, &wl_seat_interface, 1);
    } else if (strcmp(interface, "zwlr_data_control_manager_v1") == 0) {
        st->manager = wl_registry_bind(registry, id, &zwlr_data_control_manager_v1_interface, 2);
    }
    (void)version;
}

static void c_registry_global_remove(void *data, struct wl_registry *registry, uint32_t id) {
    (void)data; (void)registry; (void)id;
}

static const struct wl_registry_listener c_registry_listener = {
    .global = c_registry_global,
    .global_remove = c_registry_global_remove,
};

static struct cgo_wayland_state *cgo_wayland_init(void) {
    struct cgo_wayland_state *st = calloc(1, sizeof(*st));
    st->display = wl_display_connect(NULL);
    if (!st->display) {
        free(st);
        return NULL;
    }

    st->registry = wl_display_get_registry(st->display);
    wl_registry_add_listener(st->registry, &c_registry_listener, st);
    wl_display_roundtrip(st->display);

    if (!st->seat || !st->manager) {
        wl_display_disconnect(st->display);
        free(st);
        return NULL;
    }

    st->device = zwlr_data_control_manager_v1_get_data_device(st->manager, st->seat);
    zwlr_data_control_device_v1_add_listener(st->device, &c_device_listener, st);
    wl_display_roundtrip(st->display);

    return st;
}

static int cgo_wayland_get_fd(struct cgo_wayland_state *st) {
    return wl_display_get_fd(st->display);
}

static int cgo_wayland_dispatch(struct cgo_wayland_state *st) {
    return wl_display_dispatch(st->display);
}

static void cgo_wayland_destroy(struct cgo_wayland_state *st) {
    if (!st) return;
    if (st->device) zwlr_data_control_device_v1_destroy(st->device);
    if (st->manager) zwlr_data_control_manager_v1_destroy(st->manager);
    if (st->seat) wl_seat_destroy(st->seat);
    if (st->registry) wl_registry_destroy(st->registry);
    if (st->display) wl_display_disconnect(st->display);
    free(st);
}
*/
import "C"
import (
	"fmt"
	"os"
	"runtime"
	"time"
)

var eventCh = make(chan string, 16)

//export goOnSelectionOffer
func goOnSelectionOffer(mimes *C.char) {
	eventCh <- fmt.Sprintf("OFFER:%s", C.GoString(mimes))
}

//export goOnSelectionCleared
func goOnSelectionCleared() {
	eventCh <- "CLEARED"
}

func main() {
	fmt.Println("=== CGO IN-PROCESS WAYLAND CLIENT PROBE ===")
	st := C.cgo_wayland_init()
	if st == nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize CGO Wayland state\n")
		os.Exit(1)
	}
	defer C.cgo_wayland_destroy(st)

	fd := int(C.cgo_wayland_get_fd(st))
	fmt.Printf("CGO Wayland display initialized. Wayland fd: %d\n", fd)

	stop := make(chan struct{})

	// Goroutine event loop
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				ret := C.cgo_wayland_dispatch(st)
				if ret < 0 {
					eventCh <- "DISCONNECTED"
					return
				}
			}
		}
	}()

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	fmt.Printf("Initial Go Heap Alloc: %d KB, TotalAlloc: %d KB, NumGC: %d\n",
		mem.Alloc/1024, mem.TotalAlloc/1024, mem.NumGC)

	timeout := time.After(3 * time.Second)
	eventsReceived := 0

loop:
	for {
		select {
		case ev := <-eventCh:
			eventsReceived++
			fmt.Printf("Go Channel Received Event #%d: %s\n", eventsReceived, ev)
			if eventsReceived >= 2 {
				break loop
			}
		case <-timeout:
			fmt.Printf("Timeout reached. Total events received: %d\n", eventsReceived)
			break loop
		}
	}

	close(stop)
	runtime.ReadMemStats(&mem)
	fmt.Printf("Final Go Heap Alloc: %d KB, TotalAlloc: %d KB\n",
		mem.Alloc/1024, mem.TotalAlloc/1024)
	fmt.Println("STATUS=cgo_probe_success")
}
