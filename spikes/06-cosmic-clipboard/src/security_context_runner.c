/*
 * security_context_runner.c - Flatpak / wp_security_context_v1 Sandbox Evaluation
 * Part of PhoneBridge Spike 06
 */

#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <sys/socket.h>
#include <sys/un.h>
#include <sys/wait.h>
#include <fcntl.h>
#include <errno.h>
#include <wayland-client.h>
#include "security-context-client-protocol.h"
#include "wlr-data-control-client-protocol.h"

struct host_ctx {
    struct wl_display *display;
    struct wl_registry *registry;
    struct wp_security_context_manager_v1 *sec_manager;
};

static void host_registry_global(void *data, struct wl_registry *registry,
                                uint32_t id, const char *interface, uint32_t version) {
    struct host_ctx *ctx = data;
    if (strcmp(interface, "wp_security_context_manager_v1") == 0) {
        ctx->sec_manager = wl_registry_bind(registry, id, &wp_security_context_manager_v1_interface, 1);
    }
    (void)version;
}

static void host_registry_global_remove(void *data, struct wl_registry *registry, uint32_t id) {
    (void)data; (void)registry; (void)id;
}

static const struct wl_registry_listener host_registry_listener = {
    .global = host_registry_global,
    .global_remove = host_registry_global_remove,
};

/* Sandboxed Child Inspection */
struct sandboxed_ctx {
    struct wl_display *display;
    struct wl_registry *registry;
    int data_control_found;
    uint32_t data_control_id;
    uint32_t data_control_version;
    int global_count;
    char globals[128][64];
};

static void sandboxed_registry_global(void *data, struct wl_registry *registry,
                                     uint32_t id, const char *interface, uint32_t version) {
    struct sandboxed_ctx *ctx = data;
    if (ctx->global_count < 128) {
        snprintf(ctx->globals[ctx->global_count], 64, "%s (v%u)", interface, version);
        ctx->global_count++;
    }
    if (strcmp(interface, "zwlr_data_control_manager_v1") == 0) {
        ctx->data_control_found = 1;
        ctx->data_control_id = id;
        ctx->data_control_version = version;
    }
    (void)registry;
}

static void sandboxed_registry_global_remove(void *data, struct wl_registry *registry, uint32_t id) {
    (void)data; (void)registry; (void)id;
}

static const struct wl_registry_listener sandboxed_registry_listener = {
    .global = sandboxed_registry_global,
    .global_remove = sandboxed_registry_global_remove,
};

static int run_sandboxed_probe(const char *sock_path) {
    printf("[SANDBOX_CHILD] Connecting to sandboxed socket: %s\n", sock_path);
    int fd = socket(AF_UNIX, SOCK_STREAM, 0);
    if (fd < 0) {
        perror("socket");
        return 1;
    }

    struct sockaddr_un addr;
    memset(&addr, 0, sizeof(addr));
    addr.sun_family = AF_UNIX;
    strncpy(addr.sun_path, sock_path, sizeof(addr.sun_path) - 1);

    if (connect(fd, (struct sockaddr *)&addr, sizeof(addr)) < 0) {
        perror("connect to sandboxed socket");
        close(fd);
        return 1;
    }

    struct sandboxed_ctx s_ctx;
    memset(&s_ctx, 0, sizeof(s_ctx));

    s_ctx.display = wl_display_connect_to_fd(fd);
    if (!s_ctx.display) {
        fprintf(stderr, "[SANDBOX_CHILD] ERROR: wl_display_connect_to_fd failed\n");
        return 1;
    }

    s_ctx.registry = wl_display_get_registry(s_ctx.display);
    wl_registry_add_listener(s_ctx.registry, &sandboxed_registry_listener, &s_ctx);
    wl_display_roundtrip(s_ctx.display);

    printf("[SANDBOX_CHILD] Connected under Flatpak security context!\n");
    printf("[SANDBOX_CHILD] Total advertised globals visible inside sandbox: %d\n", s_ctx.global_count);

    printf("[SANDBOX_CHILD] Checking for zwlr_data_control_manager_v1...\n");
    if (s_ctx.data_control_found) {
        printf("[SANDBOX_CHILD] RESULT: zwlr_data_control_manager_v1 IS ADVERTISED inside sandbox (id=%u, v=%u)\n",
               s_ctx.data_control_id, s_ctx.data_control_version);

        /* Attempt to bind */
        printf("[SANDBOX_CHILD] Attempting to bind zwlr_data_control_manager_v1...\n");
        struct zwlr_data_control_manager_v1 *mgr = wl_registry_bind(s_ctx.registry,
            s_ctx.data_control_id, &zwlr_data_control_manager_v1_interface, 1);
        wl_display_flush(s_ctx.display);
        int err = wl_display_roundtrip(s_ctx.display);
        if (err < 0 || wl_display_get_error(s_ctx.display) != 0) {
            printf("[SANDBOX_CHILD] BIND_RESULT: Compositor KILLED sandboxed client with protocol error upon binding (access restricted!)\n");
        } else {
            printf("[SANDBOX_CHILD] BIND_RESULT: Bind SUCCEEDED without error!\n");
            zwlr_data_control_manager_v1_destroy(mgr);
        }
    } else {
        printf("[SANDBOX_CHILD] RESULT: zwlr_data_control_manager_v1 is HIDDEN / FILTERED OUT from sandboxed clients!\n");
    }

    printf("\n[SANDBOX_CHILD] Visible globals list:\n");
    for (int i = 0; i < s_ctx.global_count; i++) {
        printf("  - %s\n", s_ctx.globals[i]);
    }

    wl_registry_destroy(s_ctx.registry);
    wl_display_disconnect(s_ctx.display);
    return 0;
}

int main(int argc, char **argv) {
    (void)argc; (void)argv;
    struct host_ctx h_ctx;
    memset(&h_ctx, 0, sizeof(h_ctx));

    h_ctx.display = wl_display_connect(NULL);
    if (!h_ctx.display) {
        fprintf(stderr, "Failed to connect to host Wayland display\n");
        return 1;
    }

    h_ctx.registry = wl_display_get_registry(h_ctx.display);
    wl_registry_add_listener(h_ctx.registry, &host_registry_listener, &h_ctx);
    wl_display_roundtrip(h_ctx.display);

    if (!h_ctx.sec_manager) {
        fprintf(stderr, "Compositor does NOT advertise wp_security_context_manager_v1\n");
        return 1;
    }
    printf("[HOST] Connected to host Wayland. wp_security_context_manager_v1 bound.\n");

    /* Create UNIX listening socket for sandboxed client */
    const char *sock_path = "/tmp/phonebridge_sandboxed_wayland.sock";
    unlink(sock_path);

    int listen_fd = socket(AF_UNIX, SOCK_STREAM, 0);
    if (listen_fd < 0) {
        perror("listen socket");
        return 1;
    }

    struct sockaddr_un saddr;
    memset(&saddr, 0, sizeof(saddr));
    saddr.sun_family = AF_UNIX;
    strncpy(saddr.sun_path, sock_path, sizeof(saddr.sun_path) - 1);

    if (bind(listen_fd, (struct sockaddr *)&saddr, sizeof(saddr)) < 0) {
        perror("bind listen_fd");
        close(listen_fd);
        return 1;
    }

    if (listen(listen_fd, 5) < 0) {
        perror("listen");
        close(listen_fd);
        return 1;
    }

    int close_pipe[2];
    if (pipe(close_pipe) < 0) {
        perror("pipe");
        return 1;
    }

    /* Create security context with listener */
    struct wp_security_context_v1 *sec_ctx =
        wp_security_context_manager_v1_create_listener(h_ctx.sec_manager, listen_fd, close_pipe[0]);

    wp_security_context_v1_set_app_id(sec_ctx, "dev.phonebridge.sandboxed");
    wp_security_context_v1_set_sandbox_engine(sec_ctx, "flatpak");
    wp_security_context_v1_commit(sec_ctx);

    /* Close host end of listen_fd & close_pipe[0] as required by protocol */
    close(listen_fd);
    close(close_pipe[0]);

    wl_display_roundtrip(h_ctx.display);
    printf("[HOST] Security context created with listen_fd, app_id=dev.phonebridge.sandboxed, engine=flatpak.\n");

    /* Fork child to connect to the sandboxed socket */
    pid_t pid = fork();
    if (pid == 0) {
        /* Child process */
        int ret = run_sandboxed_probe(sock_path);
        exit(ret);
    }

    /* Host process waits for child */
    int status = 0;
    waitpid(pid, &status, 0);

    /* Signal hangup to compositor and clean up */
    close(close_pipe[1]);
    wp_security_context_v1_destroy(sec_ctx);
    wp_security_context_manager_v1_destroy(h_ctx.sec_manager);
    wl_registry_destroy(h_ctx.registry);
    wl_display_disconnect(h_ctx.display);
    unlink(sock_path);

    printf("[HOST] Sandbox evaluation complete. Child exit status: %d\n", WEXITSTATUS(status));
    return WEXITSTATUS(status);
}
