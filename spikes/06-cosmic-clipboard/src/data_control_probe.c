/*
 * data_control_probe.c - Wayland zwlr_data_control_unstable_v1 Probe Harness
 * Part of PhoneBridge Spike 06 (COSMIC Clipboard Validation)
 */

#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdint.h>
#include <unistd.h>
#include <fcntl.h>
#include <poll.h>
#include <errno.h>
#include <time.h>
#include <sys/types.h>
#include <sys/socket.h>
#include <wayland-client.h>
#include "wlr-data-control-client-protocol.h"

static uint64_t now_us(void) {
    struct timespec ts;
    clock_gettime(CLOCK_MONOTONIC, &ts);
    return (uint64_t)ts.tv_sec * 1000000ULL + (uint64_t)ts.tv_nsec / 1000ULL;
}

struct probe_context {
    struct wl_display *display;
    struct wl_registry *registry;
    struct wl_seat *seat;
    struct zwlr_data_control_manager_v1 *manager;
    struct zwlr_data_control_device_v1 *device;
    struct zwlr_data_control_source_v1 *source;
    struct zwlr_data_control_offer_v1 *current_offer;

    char *mode;
    char *mime_type;
    char *custom_mime;
    char *payload_text;
    size_t payload_len;
    char *html_text;
    size_t html_len;
    char *uri_text;
    size_t uri_len;
    int binary_mode;
    int primary_selection;
    int auto_read;
    int max_events;
    int event_count;
    int max_offers;
    int offer_event_count;
    int serve_count;
    int cancelled;
    int ready;
    int target_size_mb;
    int burst_count;
    int burst_interval_ms;

    /* Current offer mime types */
    char offer_mimes[64][128];
    int offer_mime_count;
};

/* Data Offer Listener */
static void offer_offer(void *data, struct zwlr_data_control_offer_v1 *offer, const char *mime_type) {
    struct probe_context *ctx = data;
    if (ctx->offer_mime_count < 64) {
        strncpy(ctx->offer_mimes[ctx->offer_mime_count], mime_type, 127);
        ctx->offer_mimes[ctx->offer_mime_count][127] = '\0';
        ctx->offer_mime_count++;
    }
}

static const struct zwlr_data_control_offer_v1_listener offer_listener = {
    .offer = offer_offer,
};

/* Data Device Listener */
static void device_data_offer(void *data, struct zwlr_data_control_device_v1 *device,
                              struct zwlr_data_control_offer_v1 *offer) {
    struct probe_context *ctx = data;
    if (ctx->current_offer) {
        zwlr_data_control_offer_v1_destroy(ctx->current_offer);
    }
    ctx->current_offer = offer;
    ctx->offer_mime_count = 0;
    zwlr_data_control_offer_v1_add_listener(offer, &offer_listener, ctx);
}

static void read_offer_data(struct probe_context *ctx, struct zwlr_data_control_offer_v1 *offer, const char *mime) {
    int pfd[2];
    if (pipe2(pfd, O_CLOEXEC) < 0) {
        perror("pipe2");
        return;
    }

    uint64_t t0 = now_us();
    zwlr_data_control_offer_v1_receive(offer, mime, pfd[1]);
    wl_display_flush(ctx->display);
    close(pfd[1]);

    /* Read from pfd[0] */
    size_t cap = 65536;
    size_t total = 0;
    char *buf = malloc(cap);
    if (!buf) {
        close(pfd[0]);
        return;
    }

    while (1) {
        if (total + 4096 > cap) {
            cap *= 2;
            char *nb = realloc(buf, cap);
            if (!nb) break;
            buf = nb;
        }
        ssize_t n = read(pfd[0], buf + total, cap - total - 1);
        if (n > 0) {
            total += n;
        } else if (n == 0) {
            break;
        } else {
            if (errno == EINTR) continue;
            break;
        }
    }
    close(pfd[0]);
    uint64_t t1 = now_us();
    buf[total] = '\0';

    uint32_t digest = 0;
    for (size_t i = 0; i < total; i++) {
        digest = ((digest << 5) + digest) + (unsigned char)buf[i];
    }

    char preview[128];
    size_t prev_len = total < 100 ? total : 100;
    for (size_t i = 0; i < prev_len; i++) {
        char c = buf[i];
        preview[i] = (c >= 32 && c <= 126) ? c : '.';
    }
    preview[prev_len] = '\0';

    printf("EVENT=read_done mime=%s bytes=%zu duration_us=%llu digest=%08x preview=\"%s\"\n",
           mime, total, (unsigned long long)(t1 - t0), digest, preview);
    fflush(stdout);
    free(buf);
}

static void device_selection(void *data, struct zwlr_data_control_device_v1 *device,
                             struct zwlr_data_control_offer_v1 *offer) {
    struct probe_context *ctx = data;
    uint64_t ts = now_us();
    ctx->event_count++;

    if (!offer) {
        printf("EVENT=selection_cleared time_us=%llu count=%d\n",
               (unsigned long long)ts, ctx->event_count);
        fflush(stdout);
        if (ctx->max_events > 0 && ctx->event_count >= ctx->max_events) {
            ctx->ready = 1;
        }
        return;
    }

    ctx->offer_event_count++;

    /* Print offered MIME types */
    printf("EVENT=selection_offer time_us=%llu count=%d mime_count=%d mimes=",
           (unsigned long long)ts, ctx->event_count, ctx->offer_mime_count);
    for (int i = 0; i < ctx->offer_mime_count; i++) {
        printf("%s%s", ctx->offer_mimes[i], (i == ctx->offer_mime_count - 1) ? "" : ",");
    }
    printf("\n");
    fflush(stdout);

    if (ctx->auto_read && ctx->offer_mime_count > 0) {
        /* Find matching mime type */
        const char *chosen = NULL;
        for (int i = 0; i < ctx->offer_mime_count; i++) {
            if (ctx->mime_type && strcmp(ctx->offer_mimes[i], ctx->mime_type) == 0) {
                chosen = ctx->offer_mimes[i];
                break;
            }
        }
        if (!chosen) {
            /* Fallback to text/plain;charset=utf-8 or text/plain or first */
            for (int i = 0; i < ctx->offer_mime_count; i++) {
                if (strcmp(ctx->offer_mimes[i], "text/plain;charset=utf-8") == 0) {
                    chosen = ctx->offer_mimes[i];
                    break;
                }
            }
        }
        if (!chosen) {
            for (int i = 0; i < ctx->offer_mime_count; i++) {
                if (strcmp(ctx->offer_mimes[i], "text/plain") == 0) {
                    chosen = ctx->offer_mimes[i];
                    break;
                }
            }
        }
        if (!chosen) {
            chosen = ctx->offer_mimes[0];
        }

        read_offer_data(ctx, offer, chosen);
    }

    if (ctx->max_events > 0 && ctx->event_count >= ctx->max_events) {
        ctx->ready = 1;
    }
    if (ctx->max_offers > 0 && ctx->offer_event_count >= ctx->max_offers) {
        ctx->ready = 1;
    }
}

static void device_primary_selection(void *data, struct zwlr_data_control_device_v1 *device,
                                     struct zwlr_data_control_offer_v1 *offer) {
    /* Handle primary selection if needed */
}

static const struct zwlr_data_control_device_v1_listener device_listener = {
    .data_offer = device_data_offer,
    .selection = device_selection,
    .primary_selection = device_primary_selection,
};

/* Data Source Listener */
static void source_send(void *data, struct zwlr_data_control_source_v1 *source,
                        const char *mime_type, int32_t fd) {
    struct probe_context *ctx = data;
    uint64_t t0 = now_us();
    ctx->serve_count++;

    const char *out_data = ctx->payload_text;
    size_t out_len = ctx->payload_len;

    if (ctx->html_text && (strcmp(mime_type, "text/html") == 0 || strcmp(mime_type, "text/html;charset=utf-8") == 0)) {
        out_data = ctx->html_text;
        out_len = ctx->html_len;
    } else if (ctx->uri_text && strcmp(mime_type, "text/uri-list") == 0) {
        out_data = ctx->uri_text;
        out_len = ctx->uri_len;
    }

    size_t written = 0;
    while (written < out_len) {
        ssize_t n = write(fd, out_data + written, out_len - written);
        if (n > 0) {
            written += n;
        } else if (n < 0) {
            if (errno == EINTR) continue;
            break;
        } else {
            break;
        }
    }
    close(fd);
    uint64_t t1 = now_us();

    printf("EVENT=source_send mime=%s bytes=%zu duration_us=%llu serve_count=%d\n",
           mime_type, written, (unsigned long long)(t1 - t0), ctx->serve_count);
    fflush(stdout);
}

static void source_cancelled(void *data, struct zwlr_data_control_source_v1 *source) {
    struct probe_context *ctx = data;
    ctx->cancelled = 1;
    printf("EVENT=source_cancelled time_us=%llu serve_count=%d\n",
           (unsigned long long)now_us(), ctx->serve_count);
    fflush(stdout);
}

static const struct zwlr_data_control_source_v1_listener source_listener = {
    .send = source_send,
    .cancelled = source_cancelled,
};

/* Registry Listener */
static void registry_global(void *data, struct wl_registry *registry,
                            uint32_t id, const char *interface, uint32_t version) {
    struct probe_context *ctx = data;
    if (strcmp(interface, "wl_seat") == 0) {
        ctx->seat = wl_registry_bind(registry, id, &wl_seat_interface, 1);
    } else if (strcmp(interface, "zwlr_data_control_manager_v1") == 0) {
        uint32_t bind_ver = version < 2 ? version : 2;
        ctx->manager = wl_registry_bind(registry, id, &zwlr_data_control_manager_v1_interface, bind_ver);
    }
}

static void registry_global_remove(void *data, struct wl_registry *registry, uint32_t id) {
}

static const struct wl_registry_listener registry_listener = {
    .global = registry_global,
    .global_remove = registry_global_remove,
};

int main(int argc, char **argv) {
    struct probe_context ctx;
    memset(&ctx, 0, sizeof(ctx));
    ctx.mode = "listen";
    ctx.mime_type = "text/plain;charset=utf-8";
    ctx.auto_read = 1;
    ctx.max_events = 0;

    for (int i = 1; i < argc; i++) {
        if (strcmp(argv[i], "--mode") == 0 && i + 1 < argc) {
            ctx.mode = argv[++i];
        } else if (strcmp(argv[i], "--mime") == 0 && i + 1 < argc) {
            ctx.mime_type = argv[++i];
        } else if (strcmp(argv[i], "--text") == 0 && i + 1 < argc) {
            ctx.payload_text = argv[++i];
            ctx.payload_len = strlen(ctx.payload_text);
        } else if (strcmp(argv[i], "--html") == 0 && i + 1 < argc) {
            ctx.html_text = argv[++i];
            ctx.html_len = strlen(ctx.html_text);
        } else if (strcmp(argv[i], "--uri-list") == 0 && i + 1 < argc) {
            ctx.uri_text = argv[++i];
            ctx.uri_len = strlen(ctx.uri_text);
        } else if (strcmp(argv[i], "--custom-mime") == 0 && i + 1 < argc) {
            ctx.custom_mime = argv[++i];
        } else if (strcmp(argv[i], "--binary") == 0) {
            ctx.binary_mode = 1;
        } else if (strcmp(argv[i], "--max-events") == 0 && i + 1 < argc) {
            ctx.max_events = atoi(argv[++i]);
        } else if (strcmp(argv[i], "--max-offers") == 0 && i + 1 < argc) {
            ctx.max_offers = atoi(argv[++i]);
        } else if (strcmp(argv[i], "--no-read") == 0) {
            ctx.auto_read = 0;
        } else if (strcmp(argv[i], "--size-mb") == 0 && i + 1 < argc) {
            ctx.target_size_mb = atoi(argv[++i]);
        } else if (strcmp(argv[i], "--burst") == 0 && i + 1 < argc) {
            ctx.burst_count = atoi(argv[++i]);
        } else if (strcmp(argv[i], "--burst-interval-ms") == 0 && i + 1 < argc) {
            ctx.burst_interval_ms = atoi(argv[++i]);
        }
    }

    if (strcmp(ctx.mode, "write") == 0) {
        ctx.auto_read = 0;
    }

    if (ctx.target_size_mb > 0) {
        size_t sz = (size_t)ctx.target_size_mb * 1024 * 1024;
        ctx.payload_text = malloc(sz);
        if (!ctx.payload_text) {
            fprintf(stderr, "malloc %d MB failed\n", ctx.target_size_mb);
            return 1;
        }
        if (ctx.binary_mode) {
            for (size_t b = 0; b < sz; b++) {
                ctx.payload_text[b] = (char)(b & 0xFF);
            }
        } else {
            memset(ctx.payload_text, 'X', sz);
        }
        ctx.payload_len = sz;
    } else if (ctx.binary_mode && !ctx.payload_text) {
        size_t sz = 256;
        ctx.payload_text = malloc(sz);
        for (size_t b = 0; b < sz; b++) {
            ctx.payload_text[b] = (char)(b & 0xFF);
        }
        ctx.payload_len = sz;
    }

    ctx.display = wl_display_connect(NULL);
    if (!ctx.display) {
        fprintf(stderr, "ERROR: wl_display_connect failed\n");
        return 1;
    }

    ctx.registry = wl_display_get_registry(ctx.display);
    wl_registry_add_listener(ctx.registry, &registry_listener, &ctx);
    wl_display_roundtrip(ctx.display);

    if (!ctx.seat) {
        fprintf(stderr, "ERROR: wl_seat not found\n");
        return 1;
    }
    if (!ctx.manager) {
        fprintf(stderr, "ERROR: zwlr_data_control_manager_v1 not advertised by compositor\n");
        return 2;
    }

    ctx.device = zwlr_data_control_manager_v1_get_data_device(ctx.manager, ctx.seat);
    zwlr_data_control_device_v1_add_listener(ctx.device, &device_listener, &ctx);
    wl_display_roundtrip(ctx.display);

    printf("STATUS=ready compositor=COSMIC data_control=v2 mode=%s\n", ctx.mode);
    fflush(stdout);

    if (strcmp(ctx.mode, "write") == 0) {
        if (!ctx.payload_text) {
            ctx.payload_text = "PHONEBRIDGE_COSMIC_CLIPBOARD_VALIDATION";
            ctx.payload_len = strlen(ctx.payload_text);
        }

        if (ctx.burst_count > 1) {
            for (int b = 0; b < ctx.burst_count; b++) {
                struct zwlr_data_control_source_v1 *src = zwlr_data_control_manager_v1_create_data_source(ctx.manager);
                zwlr_data_control_source_v1_add_listener(src, &source_listener, &ctx);
                zwlr_data_control_source_v1_offer(src, "text/plain;charset=utf-8");
                zwlr_data_control_source_v1_offer(src, "text/plain");
                zwlr_data_control_device_v1_set_selection(ctx.device, src);
                wl_display_flush(ctx.display);
                printf("EVENT=burst_step index=%d total=%d time_us=%llu\n",
                       b + 1, ctx.burst_count, (unsigned long long)now_us());
                fflush(stdout);
                if (b < ctx.burst_count - 1 && ctx.burst_interval_ms > 0) {
                    usleep(ctx.burst_interval_ms * 1000);
                }
            }
            while (!ctx.cancelled) {
                if (wl_display_dispatch(ctx.display) < 0) break;
            }
            printf("STATUS=finished serve_count=%d cancelled=%d\n", ctx.serve_count, ctx.cancelled);
            fflush(stdout);
        } else {
            ctx.source = zwlr_data_control_manager_v1_create_data_source(ctx.manager);
            zwlr_data_control_source_v1_add_listener(ctx.source, &source_listener, &ctx);

            /* Offer standard mimes */
            zwlr_data_control_source_v1_offer(ctx.source, "text/plain;charset=utf-8");
            zwlr_data_control_source_v1_offer(ctx.source, "text/plain");
            zwlr_data_control_source_v1_offer(ctx.source, "UTF8_STRING");
            zwlr_data_control_source_v1_offer(ctx.source, "STRING");
            zwlr_data_control_source_v1_offer(ctx.source, "TEXT");

            if (ctx.html_text) {
                zwlr_data_control_source_v1_offer(ctx.source, "text/html;charset=utf-8");
                zwlr_data_control_source_v1_offer(ctx.source, "text/html");
            }

            if (ctx.uri_text) {
                zwlr_data_control_source_v1_offer(ctx.source, "text/uri-list");
            }

            if (ctx.custom_mime) {
                zwlr_data_control_source_v1_offer(ctx.source, ctx.custom_mime);
            }

            zwlr_data_control_device_v1_set_selection(ctx.device, ctx.source);
            wl_display_flush(ctx.display);

            printf("STATUS=selection_set time_us=%llu bytes=%zu html=%s\n",
                   (unsigned long long)now_us(), ctx.payload_len, ctx.html_text ? "yes" : "no");
            fflush(stdout);

            /* Main loop: serve paste requests until cancelled or signaled */
            while (!ctx.cancelled) {
                if (wl_display_dispatch(ctx.display) < 0) break;
            }
            printf("STATUS=finished serve_count=%d cancelled=%d\n", ctx.serve_count, ctx.cancelled);
            fflush(stdout);
        }

    } else if (strcmp(ctx.mode, "read_once") == 0) {
        /* Already roundtripped, device_selection was called on initial selection */
        printf("STATUS=read_once_complete\n");
        fflush(stdout);

    } else if (strcmp(ctx.mode, "listen") == 0) {
        while (!ctx.ready) {
            if (wl_display_dispatch(ctx.display) < 0) break;
        }
        printf("STATUS=listen_finished total_events=%d\n", ctx.event_count);
        fflush(stdout);
    }

    if (ctx.current_offer) {
        zwlr_data_control_offer_v1_destroy(ctx.current_offer);
    }
    if (ctx.source) {
        zwlr_data_control_source_v1_destroy(ctx.source);
    }
    if (ctx.device) {
        zwlr_data_control_device_v1_destroy(ctx.device);
    }
    if (ctx.manager) {
        zwlr_data_control_manager_v1_destroy(ctx.manager);
    }
    if (ctx.seat) {
        wl_seat_destroy(ctx.seat);
    }
    wl_registry_destroy(ctx.registry);
    wl_display_disconnect(ctx.display);

    return 0;
}
