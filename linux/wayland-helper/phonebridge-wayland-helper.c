/*
 * phonebridge-wayland-helper.c - Production Wayland Clipboard Helper for PhoneBridge
 *
 * Implements zwlr_data_control_unstable_v1 v2 client for windowless, focus-independent
 * clipboard read, write, and change observation under COSMIC and compatible Wayland compositors.
 *
 * Communicates with the PhoneBridge pure-Go supervisor over stdin/stdout using
 * deterministic length-prefixed IPC framing.
 *
 * Security & Privacy:
 *   - Zero Logging Rule: Raw clipboard payload bytes are NEVER logged to stdout or stderr.
 *   - Payload ceiling: Enforces maximum payload read/write bounds (768 KiB / 786,432 bytes).
 *   - Subprocess boundary: Process isolation protects the Go core from compositor crashes.
 */

#ifndef _GNU_SOURCE
#define _GNU_SOURCE
#endif
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdint.h>
#include <stdbool.h>
#include <unistd.h>
#include <fcntl.h>
#include <poll.h>
#include <errno.h>
#include <time.h>
#include <signal.h>
#include <sys/types.h>
#include <sys/socket.h>
#include <wayland-client.h>
#include "wlr-data-control-client-protocol.h"

#define MAX_PAYLOAD_SIZE 786432 /* 768 KiB application ceiling */
#define MAX_READ_BUFFER (MAX_PAYLOAD_SIZE + 4096)
#define MAX_MIMES 64
#define MAX_MIME_LEN 128
#define READ_TIMEOUT_MS 1500

struct helper_context {
    struct wl_display *display;
    struct wl_registry *registry;
    struct wl_seat *seat;
    struct zwlr_data_control_manager_v1 *manager;
    struct zwlr_data_control_device_v1 *device;
    struct zwlr_data_control_source_v1 *source;
    struct zwlr_data_control_offer_v1 *current_offer;

    /* Current selection held by this helper */
    char *selection_payload;
    size_t selection_len;
    char selection_mime[MAX_MIME_LEN];

    /* Incoming offer state */
    char offer_mimes[MAX_MIMES][MAX_MIME_LEN];
    int offer_mime_count;

    /* Runtime flags */
    bool running;
    bool source_active;
    const char *compositor_name;
};

/* --- Data Offer Listener --- */
static void offer_offer(void *data, struct zwlr_data_control_offer_v1 *offer, const char *mime_type) {
    (void)offer;
    struct helper_context *ctx = (struct helper_context *)data;
    if (ctx->offer_mime_count < MAX_MIMES && mime_type != NULL) {
        strncpy(ctx->offer_mimes[ctx->offer_mime_count], mime_type, MAX_MIME_LEN - 1);
        ctx->offer_mimes[ctx->offer_mime_count][MAX_MIME_LEN - 1] = '\0';
        ctx->offer_mime_count++;
    }
}

static const struct zwlr_data_control_offer_v1_listener offer_listener = {
    .offer = offer_offer,
};

/* --- Helper forward declarations --- */
static void read_offer_payload(struct helper_context *ctx, struct zwlr_data_control_offer_v1 *offer, const char *mime);

/* --- Data Device Listener --- */
static void device_data_offer(void *data, struct zwlr_data_control_device_v1 *device,
                              struct zwlr_data_control_offer_v1 *offer) {
    (void)device;
    struct helper_context *ctx = (struct helper_context *)data;
    if (ctx->current_offer != NULL) {
        zwlr_data_control_offer_v1_destroy(ctx->current_offer);
        ctx->current_offer = NULL;
    }
    ctx->current_offer = offer;
    ctx->offer_mime_count = 0;
    zwlr_data_control_offer_v1_add_listener(offer, &offer_listener, ctx);
}

static void device_selection(void *data, struct zwlr_data_control_device_v1 *device,
                             struct zwlr_data_control_offer_v1 *offer) {
    (void)device;
    struct helper_context *ctx = (struct helper_context *)data;

    if (offer == NULL) {
        fprintf(stdout, "EVENT=SELECTION_CLEARED\n");
        fflush(stdout);
        return;
    }

    /* Print offer event with available MIME types */
    fprintf(stdout, "EVENT=SELECTION_OFFER mime_count=%d mimes=", ctx->offer_mime_count);
    for (int i = 0; i < ctx->offer_mime_count; i++) {
        fprintf(stdout, "%s%s", ctx->offer_mimes[i], (i == ctx->offer_mime_count - 1) ? "" : ",");
    }
    fprintf(stdout, "\n");
    fflush(stdout);

    /* If our own source is active, this offer is an echo of our own SET_SELECTION.
     * Do not read it back, preventing self-read reentrancy deadlock. */
    if (ctx->source_active && ctx->source != NULL) {
        return;
    }

    /* Determine preferred text MIME type */
    const char *chosen = NULL;
    for (int i = 0; i < ctx->offer_mime_count; i++) {
        if (strcmp(ctx->offer_mimes[i], "text/plain;charset=utf-8") == 0) {
            chosen = ctx->offer_mimes[i];
            break;
        }
    }
    if (chosen == NULL) {
        for (int i = 0; i < ctx->offer_mime_count; i++) {
            if (strcmp(ctx->offer_mimes[i], "text/plain") == 0) {
                chosen = ctx->offer_mimes[i];
                break;
            }
        }
    }
    if (chosen == NULL) {
        for (int i = 0; i < ctx->offer_mime_count; i++) {
            if (strcmp(ctx->offer_mimes[i], "UTF8_STRING") == 0) {
                chosen = ctx->offer_mimes[i];
                break;
            }
        }
    }
    if (chosen == NULL) {
        for (int i = 0; i < ctx->offer_mime_count; i++) {
            if (strcmp(ctx->offer_mimes[i], "STRING") == 0 || strcmp(ctx->offer_mimes[i], "TEXT") == 0) {
                chosen = ctx->offer_mimes[i];
                break;
            }
        }
    }

    if (chosen != NULL) {
        read_offer_payload(ctx, offer, chosen);
    } else {
        fprintf(stdout, "EVENT=UNSUPPORTED_OFFER mime_count=%d mimes=", ctx->offer_mime_count);
        for (int i = 0; i < ctx->offer_mime_count; i++) {
            fprintf(stdout, "%s%s", ctx->offer_mimes[i], (i == ctx->offer_mime_count - 1) ? "" : ",");
        }
        fprintf(stdout, "\n");
        fflush(stdout);
    }
}

static void device_primary_selection(void *data, struct zwlr_data_control_device_v1 *device,
                                     struct zwlr_data_control_offer_v1 *offer) {
    (void)data;
    (void)device;
    (void)offer;
    /* Primary selection ignored for PhoneBridge V1 */
}

static const struct zwlr_data_control_device_v1_listener device_listener = {
    .data_offer = device_data_offer,
    .selection = device_selection,
    .primary_selection = device_primary_selection,
};

/* --- Read Offer Data from Pipe --- */
static void read_offer_payload(struct helper_context *ctx, struct zwlr_data_control_offer_v1 *offer, const char *mime) {
    int pfd[2];
    if (pipe2(pfd, O_CLOEXEC) < 0) {
        fprintf(stderr, "ERROR: pipe2 failed: %s\n", strerror(errno));
        return;
    }

    zwlr_data_control_offer_v1_receive(offer, mime, pfd[1]);
    wl_display_flush(ctx->display);
    close(pfd[1]);

    /* Read with poll timeout to prevent hanging on unresponsive clipboard sources */
    char *buf = malloc(MAX_READ_BUFFER);
    if (!buf) {
        close(pfd[0]);
        fprintf(stderr, "ERROR: malloc failed for clipboard read\n");
        return;
    }

    size_t total = 0;
    bool oversized = false;

    while (total < MAX_READ_BUFFER) {
        struct pollfd poll_item;
        poll_item.fd = pfd[0];
        poll_item.events = POLLIN;

        int ret = poll(&poll_item, 1, READ_TIMEOUT_MS);
        if (ret <= 0) {
            /* Timeout or error */
            break;
        }

        ssize_t n = read(pfd[0], buf + total, MAX_READ_BUFFER - total);
        if (n > 0) {
            total += (size_t)n;
            if (total > MAX_PAYLOAD_SIZE) {
                oversized = true;
                break;
            }
        } else if (n == 0) {
            /* EOF */
            break;
        } else {
            if (errno == EINTR) continue;
            break;
        }
    }

    /* If oversized, drain remaining bytes to discover exact size if possible */
    if (oversized) {
        char drain[4096];
        while (1) {
            struct pollfd poll_item;
            poll_item.fd = pfd[0];
            poll_item.events = POLLIN;
            if (poll(&poll_item, 1, 100) <= 0) break;
            ssize_t n = read(pfd[0], drain, sizeof(drain));
            if (n <= 0) break;
            total += (size_t)n;
        }
    }
    close(pfd[0]);

    if (oversized) {
        fprintf(stdout, "EVENT=READ_OVERSIZED mime=%s size=%zu\n", mime, total);
        fflush(stdout);
        free(buf);
        return;
    }

    /* Emit deterministic length-prefixed payload */
    fprintf(stdout, "EVENT=READ_DATA mime=%s len=%zu\n", mime, total);
    fflush(stdout);
    if (total > 0) {
        size_t written = 0;
        while (written < total) {
            ssize_t nw = write(STDOUT_FILENO, buf + written, total - written);
            if (nw > 0) {
                written += (size_t)nw;
            } else if (nw < 0) {
                if (errno == EINTR) continue;
                break;
            } else {
                break;
            }
        }
    }
    fprintf(stdout, "\n");
    fflush(stdout);
    free(buf);
}

/* --- Data Source Listener --- */
static void source_send(void *data, struct zwlr_data_control_source_v1 *source,
                        const char *mime_type, int32_t fd) {
    (void)source;
    struct helper_context *ctx = (struct helper_context *)data;

    /* Zero Logging: Log MIME and length, never raw payload content */
    fprintf(stdout, "EVENT=SOURCE_SEND mime=%s bytes=%zu\n", mime_type, ctx->selection_len);
    fflush(stdout);

    if (ctx->selection_payload != NULL && ctx->selection_len > 0) {
        size_t written = 0;
        while (written < ctx->selection_len) {
            ssize_t n = write(fd, ctx->selection_payload + written, ctx->selection_len - written);
            if (n > 0) {
                written += (size_t)n;
            } else if (n < 0) {
                if (errno == EINTR) continue;
                break; /* e.g. EPIPE if reader closed */
            } else {
                break;
            }
        }
    }
    close(fd);
}

static void source_cancelled(void *data, struct zwlr_data_control_source_v1 *source) {
    struct helper_context *ctx = (struct helper_context *)data;
    ctx->source_active = false;

    if (ctx->source != NULL && ctx->source == source) {
        zwlr_data_control_source_v1_destroy(ctx->source);
        ctx->source = NULL;
    }

    if (ctx->selection_payload != NULL) {
        free(ctx->selection_payload);
        ctx->selection_payload = NULL;
        ctx->selection_len = 0;
    }

    fprintf(stdout, "EVENT=SOURCE_CANCELLED\n");
    fflush(stdout);
}

static const struct zwlr_data_control_source_v1_listener source_listener = {
    .send = source_send,
    .cancelled = source_cancelled,
};

/* --- Registry Listener --- */
static void registry_global(void *data, struct wl_registry *registry,
                            uint32_t id, const char *interface, uint32_t version) {
    struct helper_context *ctx = (struct helper_context *)data;
    if (strcmp(interface, "wl_seat") == 0) {
        ctx->seat = wl_registry_bind(registry, id, &wl_seat_interface, 1);
    } else if (strcmp(interface, "zwlr_data_control_manager_v1") == 0) {
        uint32_t bind_ver = (version < 2) ? version : 2;
        ctx->manager = wl_registry_bind(registry, id, &zwlr_data_control_manager_v1_interface, bind_ver);
    }
}

static void registry_global_remove(void *data, struct wl_registry *registry, uint32_t id) {
    (void)data;
    (void)registry;
    (void)id;
}

static const struct wl_registry_listener registry_listener = {
    .global = registry_global,
    .global_remove = registry_global_remove,
};

/* --- Command Processing from Go Supervisor --- */
static void handle_set_selection(struct helper_context *ctx, const char *args) {
    char mime[MAX_MIME_LEN];
    size_t len = 0;
    mime[0] = '\0';

    /* Parse args: mime=<mime> len=<len> */
    const char *p_mime = strstr(args, "mime=");
    const char *p_len = strstr(args, "len=");

    if (!p_mime || !p_len) {
        fprintf(stdout, "STATUS=ERROR cmd=SET_SELECTION detail=malformed_arguments\n");
        fflush(stdout);
        return;
    }

    if (sscanf(p_mime, "mime=%127s", mime) != 1 || sscanf(p_len, "len=%zu", &len) != 1) {
        fprintf(stdout, "STATUS=ERROR cmd=SET_SELECTION detail=invalid_parameters\n");
        fflush(stdout);
        return;
    }

    if (len > MAX_PAYLOAD_SIZE) {
        fprintf(stdout, "STATUS=ERROR cmd=SET_SELECTION detail=payload_exceeds_max_size\n");
        fflush(stdout);
        return;
    }

    /* Read exact length payload from stdin */
    char *payload = malloc(len + 1);
    if (!payload) {
        fprintf(stdout, "STATUS=ERROR cmd=SET_SELECTION detail=out_of_memory\n");
        fflush(stdout);
        return;
    }

    size_t read_bytes = 0;
    while (read_bytes < len) {
        ssize_t nr = read(STDIN_FILENO, payload + read_bytes, len - read_bytes);
        if (nr > 0) {
            read_bytes += (size_t)nr;
        } else if (nr < 0) {
            if (errno == EINTR) continue;
            free(payload);
            fprintf(stdout, "STATUS=ERROR cmd=SET_SELECTION detail=stdin_read_failed\n");
            fflush(stdout);
            return;
        } else {
            free(payload);
            fprintf(stdout, "STATUS=ERROR cmd=SET_SELECTION detail=stdin_unexpected_eof\n");
            fflush(stdout);
            return;
        }
    }
    payload[len] = '\0';

    /* Consume trailing newline delimiter */
    char trailing;
    while (1) {
        ssize_t nr = read(STDIN_FILENO, &trailing, 1);
        if (nr > 0) break;
        if (nr < 0 && errno == EINTR) continue;
        break;
    }

    /* Destroy existing source if present */
    if (ctx->source != NULL) {
        zwlr_data_control_source_v1_destroy(ctx->source);
        ctx->source = NULL;
    }
    if (ctx->selection_payload != NULL) {
        free(ctx->selection_payload);
        ctx->selection_payload = NULL;
        ctx->selection_len = 0;
    }

    ctx->selection_payload = payload;
    ctx->selection_len = len;
    strncpy(ctx->selection_mime, mime, MAX_MIME_LEN - 1);
    ctx->selection_mime[MAX_MIME_LEN - 1] = '\0';

    /* Create new data source and register offers */
    ctx->source = zwlr_data_control_manager_v1_create_data_source(ctx->manager);
    zwlr_data_control_source_v1_add_listener(ctx->source, &source_listener, ctx);

    /* Offer standard text MIME types */
    zwlr_data_control_source_v1_offer(ctx->source, "text/plain;charset=utf-8");
    zwlr_data_control_source_v1_offer(ctx->source, "text/plain");
    zwlr_data_control_source_v1_offer(ctx->source, "UTF8_STRING");
    zwlr_data_control_source_v1_offer(ctx->source, "STRING");
    zwlr_data_control_source_v1_offer(ctx->source, "TEXT");

    /* If custom MIME was provided and differs from text/plain, offer it too */
    if (strcmp(mime, "text/plain;charset=utf-8") != 0 &&
        strcmp(mime, "text/plain") != 0 &&
        strcmp(mime, "UTF8_STRING") != 0) {
        zwlr_data_control_source_v1_offer(ctx->source, mime);
    }

    zwlr_data_control_device_v1_set_selection(ctx->device, ctx->source);
    wl_display_flush(ctx->display);
    ctx->source_active = true;

    fprintf(stdout, "STATUS=OK cmd=SET_SELECTION len=%zu\n", len);
    fflush(stdout);
}

static void handle_clear_selection(struct helper_context *ctx) {
    if (ctx->source != NULL) {
        zwlr_data_control_source_v1_destroy(ctx->source);
        ctx->source = NULL;
    }
    if (ctx->selection_payload != NULL) {
        free(ctx->selection_payload);
        ctx->selection_payload = NULL;
        ctx->selection_len = 0;
    }
    ctx->source_active = false;

    zwlr_data_control_device_v1_set_selection(ctx->device, NULL);
    wl_display_flush(ctx->display);

    fprintf(stdout, "STATUS=OK cmd=CLEAR_SELECTION\n");
    fflush(stdout);
}

static void handle_stdin_command(struct helper_context *ctx) {
    char line[512];
    size_t idx = 0;

    while (idx < sizeof(line) - 1) {
        char ch;
        ssize_t n = read(STDIN_FILENO, &ch, 1);
        if (n > 0) {
            if (ch == '\n') break;
            line[idx++] = ch;
        } else if (n < 0) {
            if (errno == EINTR) continue;
            return;
        } else {
            /* EOF on stdin -> supervisor closed pipe */
            ctx->running = false;
            return;
        }
    }
    line[idx] = '\0';

    if (strncmp(line, "CMD=SET_SELECTION ", 18) == 0) {
        handle_set_selection(ctx, line + 18);
    } else if (strcmp(line, "CMD=CLEAR_SELECTION") == 0) {
        handle_clear_selection(ctx);
    } else if (strcmp(line, "CMD=SHUTDOWN") == 0) {
        fprintf(stdout, "STATUS=OK cmd=SHUTDOWN\n");
        fflush(stdout);
        ctx->running = false;
    } else if (idx > 0) {
        fprintf(stdout, "STATUS=ERROR cmd=UNKNOWN detail=unrecognized_command\n");
        fflush(stdout);
    }
}

/* --- Environment & Detection Helpers --- */
static bool detect_is_cosmic(void) {
    const char *desktop = getenv("XDG_CURRENT_DESKTOP");
    if (desktop && strcasestr(desktop, "cosmic") != NULL) return true;
    const char *session = getenv("DESKTOP_SESSION");
    if (session && strcasestr(session, "cosmic") != NULL) return true;
    return false;
}

int main(int argc, char *argv[]) {
    /* Ignore SIGPIPE so writing to closed client pipe does not kill helper */
    signal(SIGPIPE, SIG_IGN);

    bool probe_only = (argc > 1 && strcmp(argv[1], "probe") == 0);

    struct helper_context ctx;
    memset(&ctx, 0, sizeof(ctx));
    ctx.running = true;

    /* Detect compositor environment */
    bool is_cosmic = detect_is_cosmic();
    ctx.compositor_name = is_cosmic ? "COSMIC" : "Wayland";

    /* Check environment display variables */
    const char *wayland_display = getenv("WAYLAND_DISPLAY");
    const char *xdg_runtime = getenv("XDG_RUNTIME_DIR");
    if (wayland_display == NULL && xdg_runtime == NULL) {
        fprintf(stdout, "STATUS=ERR_WAYLAND_CONNECT detail=WAYLAND_DISPLAY and XDG_RUNTIME_DIR not set\n");
        fflush(stdout);
        return 1;
    }

    ctx.display = wl_display_connect(NULL);
    if (!ctx.display) {
        fprintf(stdout, "STATUS=ERR_WAYLAND_CONNECT detail=wl_display_connect failed (%s)\n", strerror(errno));
        fflush(stdout);
        return 1;
    }

    ctx.registry = wl_display_get_registry(ctx.display);
    wl_registry_add_listener(ctx.registry, &registry_listener, &ctx);
    wl_display_roundtrip(ctx.display);

    if (!ctx.seat) {
        fprintf(stdout, "STATUS=ERR_NO_SEAT detail=wl_seat global not advertised\n");
        fflush(stdout);
        wl_registry_destroy(ctx.registry);
        wl_display_disconnect(ctx.display);
        return 5;
    }

    if (!ctx.manager) {
        const char *cosmic_flag = getenv("COSMIC_DATA_CONTROL_ENABLED");
        if (is_cosmic && (cosmic_flag == NULL || strcmp(cosmic_flag, "1") != 0)) {
            fprintf(stdout, "STATUS=ERR_COSMIC_FLAG_REQUIRED detail=COSMIC_DATA_CONTROL_ENABLED=1 is required in cosmic-comp environment\n");
        } else {
            fprintf(stdout, "STATUS=ERR_NO_DATA_CONTROL detail=zwlr_data_control_manager_v1 not advertised by compositor\n");
        }
        fflush(stdout);
        wl_seat_destroy(ctx.seat);
        wl_registry_destroy(ctx.registry);
        wl_display_disconnect(ctx.display);
        return 2;
    }

    ctx.device = zwlr_data_control_manager_v1_get_data_device(ctx.manager, ctx.seat);
    zwlr_data_control_device_v1_add_listener(ctx.device, &device_listener, &ctx);

    /* Handshake: Ready signal emitted before event dispatch */
    fprintf(stdout, "STATUS=READY compositor=%s data_control=v2\n", ctx.compositor_name);
    fflush(stdout);

    if (probe_only) {
        zwlr_data_control_device_v1_destroy(ctx.device);
        zwlr_data_control_manager_v1_destroy(ctx.manager);
        wl_seat_destroy(ctx.seat);
        wl_registry_destroy(ctx.registry);
        wl_display_disconnect(ctx.display);
        return 0;
    }

    /* Dispatch initial selection events */
    wl_display_roundtrip(ctx.display);

    /* Event Loop: Multiplex Wayland display and STDIN */
    int wl_fd = wl_display_get_fd(ctx.display);

    while (ctx.running) {
        /* Prepare to read Wayland events */
        while (wl_display_prepare_read(ctx.display) != 0) {
            wl_display_dispatch_pending(ctx.display);
        }
        wl_display_flush(ctx.display);

        struct pollfd pfd[2];
        pfd[0].fd = wl_fd;
        pfd[0].events = POLLIN;
        pfd[1].fd = STDIN_FILENO;
        pfd[1].events = POLLIN;

        int ret = poll(pfd, 2, -1);
        if (ret < 0) {
            wl_display_cancel_read(ctx.display);
            if (errno == EINTR) continue;
            break;
        }

        /* Wayland display readable */
        if (pfd[0].revents & (POLLIN | POLLERR | POLLHUP)) {
            if (wl_display_read_events(ctx.display) < 0) {
                fprintf(stdout, "STATUS=ERR_COMPOSITOR_DISCONNECTED detail=compositor connection severed\n");
                fflush(stdout);
                break;
            }
        } else {
            wl_display_cancel_read(ctx.display);
        }
        wl_display_dispatch_pending(ctx.display);

        /* STDIN readable */
        if (pfd[1].revents & POLLIN) {
            handle_stdin_command(&ctx);
        }
        if (pfd[1].revents & (POLLHUP | POLLERR)) {
            /* Supervisor terminated */
            ctx.running = false;
        }
    }

    /* Clean shutdown */
    if (ctx.source != NULL) {
        zwlr_data_control_source_v1_destroy(ctx.source);
    }
    if (ctx.selection_payload != NULL) {
        free(ctx.selection_payload);
    }
    if (ctx.current_offer != NULL) {
        zwlr_data_control_offer_v1_destroy(ctx.current_offer);
    }
    if (ctx.device != NULL) {
        zwlr_data_control_device_v1_destroy(ctx.device);
    }
    if (ctx.manager != NULL) {
        zwlr_data_control_manager_v1_destroy(ctx.manager);
    }
    if (ctx.seat != NULL) {
        wl_seat_destroy(ctx.seat);
    }
    if (ctx.registry != NULL) {
        wl_registry_destroy(ctx.registry);
    }
    if (ctx.display != NULL) {
        wl_display_disconnect(ctx.display);
    }

    return 0;
}
