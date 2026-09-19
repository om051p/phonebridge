/*
 * focus_helper.c - Simple Wayland GUI client to acquire and track window focus
 * Part of PhoneBridge Spike 06
 */

#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <sys/mman.h>
#include <fcntl.h>
#include <wayland-client.h>
#include "xdg-shell-client-protocol.h"

struct app_state {
    struct wl_display *display;
    struct wl_registry *registry;
    struct wl_compositor *compositor;
    struct wl_shm *shm;
    struct xdg_wm_base *xdg_wm_base;
    struct wl_seat *seat;
    struct wl_keyboard *keyboard;

    struct wl_surface *surface;
    struct xdg_surface *xdg_surface;
    struct xdg_toplevel *xdg_toplevel;
    struct wl_buffer *buffer;

    int focused;
    int configured;
};

static void xdg_wm_base_ping(void *data, struct xdg_wm_base *xdg_wm_base, uint32_t serial) {
    (void)data;
    xdg_wm_base_pong(xdg_wm_base, serial);
}

static const struct xdg_wm_base_listener xdg_wm_base_listener = {
    .ping = xdg_wm_base_ping,
};

static void xdg_surface_configure(void *data, struct xdg_surface *xdg_surface, uint32_t serial) {
    struct app_state *state = data;
    xdg_surface_ack_configure(xdg_surface, serial);
    state->configured = 1;
}

static const struct xdg_surface_listener xdg_surface_listener = {
    .configure = xdg_surface_configure,
};

static void xdg_toplevel_configure(void *data, struct xdg_toplevel *xdg_toplevel,
                                   int32_t width, int32_t height, struct wl_array *states) {
    (void)data; (void)xdg_toplevel; (void)width; (void)height; (void)states;
}

static void xdg_toplevel_close(void *data, struct xdg_toplevel *xdg_toplevel) {
    (void)data; (void)xdg_toplevel;
}

static const struct xdg_toplevel_listener xdg_toplevel_listener = {
    .configure = xdg_toplevel_configure,
    .close = xdg_toplevel_close,
};

static void keyboard_keymap(void *data, struct wl_keyboard *keyboard, uint32_t format, int32_t fd, uint32_t size) {
    (void)data; (void)keyboard; (void)format; (void)size;
    close(fd);
}

static void keyboard_enter(void *data, struct wl_keyboard *keyboard, uint32_t serial,
                           struct wl_surface *surface, struct wl_array *keys) {
    (void)keyboard; (void)serial; (void)surface; (void)keys;
    struct app_state *state = data;
    state->focused = 1;
    printf("EVENT=focus_enter\n");
    fflush(stdout);
}

static void keyboard_leave(void *data, struct wl_keyboard *keyboard, uint32_t serial,
                           struct wl_surface *surface) {
    (void)keyboard; (void)serial; (void)surface;
    struct app_state *state = data;
    state->focused = 0;
    printf("EVENT=focus_leave\n");
    fflush(stdout);
}

static void keyboard_key(void *data, struct wl_keyboard *keyboard, uint32_t serial,
                         uint32_t time, uint32_t key, uint32_t state) {
    (void)data; (void)keyboard; (void)serial; (void)time; (void)key; (void)state;
}

static void keyboard_modifiers(void *data, struct wl_keyboard *keyboard, uint32_t serial,
                               uint32_t mods_depressed, uint32_t mods_latched,
                               uint32_t mods_locked, uint32_t group) {
    (void)data; (void)keyboard; (void)serial; (void)mods_depressed; (void)mods_latched;
    (void)mods_locked; (void)group;
}

static void keyboard_repeat_info(void *data, struct wl_keyboard *keyboard, int32_t rate, int32_t delay) {
    (void)data; (void)keyboard; (void)rate; (void)delay;
}

static const struct wl_keyboard_listener keyboard_listener = {
    .keymap = keyboard_keymap,
    .enter = keyboard_enter,
    .leave = keyboard_leave,
    .key = keyboard_key,
    .modifiers = keyboard_modifiers,
    .repeat_info = keyboard_repeat_info,
};

static void seat_capabilities(void *data, struct wl_seat *seat, uint32_t capabilities) {
    struct app_state *state = data;
    if ((capabilities & WL_SEAT_CAPABILITY_KEYBOARD) && !state->keyboard) {
        state->keyboard = wl_seat_get_keyboard(seat);
        wl_keyboard_add_listener(state->keyboard, &keyboard_listener, state);
    }
}

static void seat_name(void *data, struct wl_seat *seat, const char *name) {
    (void)data; (void)seat; (void)name;
}

static const struct wl_seat_listener seat_listener = {
    .capabilities = seat_capabilities,
    .name = seat_name,
};

static void registry_global(void *data, struct wl_registry *registry,
                            uint32_t id, const char *interface, uint32_t version) {
    struct app_state *state = data;
    if (strcmp(interface, "wl_compositor") == 0) {
        state->compositor = wl_registry_bind(registry, id, &wl_compositor_interface, 4);
    } else if (strcmp(interface, "wl_shm") == 0) {
        state->shm = wl_registry_bind(registry, id, &wl_shm_interface, 1);
    } else if (strcmp(interface, "xdg_wm_base") == 0) {
        state->xdg_wm_base = wl_registry_bind(registry, id, &xdg_wm_base_interface, 1);
        xdg_wm_base_add_listener(state->xdg_wm_base, &xdg_wm_base_listener, state);
    } else if (strcmp(interface, "wl_seat") == 0) {
        state->seat = wl_registry_bind(registry, id, &wl_seat_interface, 1);
        wl_seat_add_listener(state->seat, &seat_listener, state);
    }
    (void)version;
}

static void registry_global_remove(void *data, struct wl_registry *registry, uint32_t id) {
    (void)data; (void)registry; (void)id;
}

static const struct wl_registry_listener registry_listener = {
    .global = registry_global,
    .global_remove = registry_global_remove,
};

static struct wl_buffer *create_buffer(struct wl_shm *shm, int width, int height) {
    int stride = width * 4;
    int size = stride * height;

    int fd = memfd_create("focus_helper_shm", MFD_CLOEXEC);
    if (fd < 0) return NULL;
    if (ftruncate(fd, size) < 0) { close(fd); return NULL; }

    uint32_t *data = mmap(NULL, size, PROT_READ | PROT_WRITE, MAP_SHARED, fd, 0);
    if (data == MAP_FAILED) { close(fd); return NULL; }

    /* Fill with blue-ish color */
    for (int i = 0; i < width * height; i++) {
        data[i] = 0xFF2A52BE;
    }
    munmap(data, size);

    struct wl_shm_pool *pool = wl_shm_create_pool(shm, fd, size);
    struct wl_buffer *buffer = wl_shm_pool_create_buffer(pool, 0, width, height, stride, WL_SHM_FORMAT_XRGB8888);
    wl_shm_pool_destroy(pool);
    close(fd);
    return buffer;
}

int main(int argc, char **argv) {
    const char *title = (argc > 1) ? argv[1] : "FocusHelperWindow";
    struct app_state state;
    memset(&state, 0, sizeof(state));

    state.display = wl_display_connect(NULL);
    if (!state.display) {
        fprintf(stderr, "Failed to connect to wayland\n");
        return 1;
    }

    state.registry = wl_display_get_registry(state.display);
    wl_registry_add_listener(state.registry, &registry_listener, &state);
    wl_display_roundtrip(state.display);
    wl_display_roundtrip(state.display);

    if (!state.compositor || !state.xdg_wm_base || !state.shm) {
        fprintf(stderr, "Missing required wayland interfaces\n");
        return 1;
    }

    state.surface = wl_compositor_create_surface(state.compositor);
    state.xdg_surface = xdg_wm_base_get_xdg_surface(state.xdg_wm_base, state.surface);
    xdg_surface_add_listener(state.xdg_surface, &xdg_surface_listener, &state);
    state.xdg_toplevel = xdg_surface_get_toplevel(state.xdg_surface);
    xdg_toplevel_add_listener(state.xdg_toplevel, &xdg_toplevel_listener, &state);
    xdg_toplevel_set_title(state.xdg_toplevel, title);
    xdg_toplevel_set_app_id(state.xdg_toplevel, "dev.phonebridge.focus_helper");

    wl_surface_commit(state.surface);
    wl_display_roundtrip(state.display);

    state.buffer = create_buffer(state.shm, 200, 200);
    wl_surface_attach(state.surface, state.buffer, 0, 0);
    wl_surface_damage(state.surface, 0, 0, 200, 200);
    wl_surface_commit(state.surface);
    wl_display_roundtrip(state.display);

    printf("STATUS=window_mapped title=%s\n", title);
    fflush(stdout);

    while (1) {
        if (wl_display_dispatch(state.display) < 0) break;
    }

    return 0;
}
