/*
 * phonebridge-mutter-helper.c - Isolated GNOME/Mutter Clipboard Helper for PhoneBridge
 *
 * Implements org.gnome.Mutter.RemoteDesktop.Session D-Bus client for windowless,
 * focus-independent clipboard read, write, and change observation under GNOME Shell / Mutter.
 *
 * Communicates with the PhoneBridge pure-Go supervisor over stdin/stdout using
 * deterministic length-prefixed IPC framing (DEC-023).
 *
 * Security & Privacy:
 *   - Zero Logging Rule: Raw clipboard payload bytes are NEVER logged to stdout or stderr.
 *   - Payload ceiling: Enforces maximum payload read/write bounds (768 KiB / 786,432 bytes).
 *   - Subprocess boundary: Process isolation protects the Go core from compositor/D-Bus crashes.
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
#include <signal.h>
#include <gio/gio.h>
#include <gio/gunixfdlist.h>

#define MAX_PAYLOAD_SIZE 786432 /* 768 KiB application ceiling */
#define MAX_READ_BUFFER (MAX_PAYLOAD_SIZE + 4096)
#define READ_TIMEOUT_MS 1500
#define MAX_MIME_LEN 128

struct helper_context {
    GMainLoop *loop;
    GDBusConnection *conn;
    char *session_path;
    guint transfer_sub_id;
    guint owner_changed_sub_id;

    /* Current selection held by this helper */
    char *selection_payload;
    size_t selection_len;
    char selection_mime[MAX_MIME_LEN];

    /* Runtime flags */
    bool running;
    bool session_active;
    bool probe_only;
};

/* --- Signal Listeners --- */

static void on_selection_owner_changed(GDBusConnection *conn,
                                       const gchar *sender_name,
                                       const gchar *object_path,
                                       const gchar *interface_name,
                                       const gchar *signal_name,
                                       GVariant *parameters,
                                       gpointer user_data) {
    (void)conn; (void)sender_name; (void)object_path; (void)interface_name; (void)signal_name;
    struct helper_context *ctx = (struct helper_context *)user_data;

    GVariant *options = NULL;
    g_variant_get(parameters, "(@a{sv})", &options);
    if (!options) return;

    gboolean session_is_owner = FALSE;
    (void)g_variant_lookup(options, "session-is-owner", "b", &session_is_owner);
    if (session_is_owner) {
        /* Loop suppression: do not echo our own selection back to supervisor */
        g_variant_unref(options);
        return;
    }

    GVariant *mime_types_var = g_variant_lookup_value(options, "mime-types", NULL);
    if (!mime_types_var) {
        g_variant_unref(options);
        return;
    }

    GVariant *array_var = NULL;
    if (g_variant_is_of_type(mime_types_var, G_VARIANT_TYPE("(as)"))) {
        array_var = g_variant_get_child_value(mime_types_var, 0);
    } else if (g_variant_is_of_type(mime_types_var, G_VARIANT_TYPE("as"))) {
        array_var = g_variant_ref(mime_types_var);
    }

    if (!array_var) {
        g_variant_unref(mime_types_var);
        g_variant_unref(options);
        return;
    }

    /* Check offered MIME types for preferred text format */
    const gchar **mimes = g_variant_get_strv(array_var, NULL);
    g_variant_unref(array_var);
    const gchar *chosen_mime = NULL;

    if (mimes) {
        for (gsize i = 0; mimes[i] != NULL; i++) {
            if (strcmp(mimes[i], "text/plain;charset=utf-8") == 0) {
                chosen_mime = "text/plain;charset=utf-8";
                break;
            }
        }
        if (!chosen_mime) {
            for (gsize i = 0; mimes[i] != NULL; i++) {
                if (strcmp(mimes[i], "text/plain") == 0) {
                    chosen_mime = "text/plain";
                    break;
                } else if (strcmp(mimes[i], "UTF8_STRING") == 0) {
                    chosen_mime = "UTF8_STRING";
                    break;
                }
            }
        }
    }

    if (!chosen_mime) {
        /* Offer contains no supported text MIME */
        fprintf(stdout, "EVENT=UNSUPPORTED_OFFER\n");
        fflush(stdout);
        g_free(mimes);
        g_variant_unref(mime_types_var);
        g_variant_unref(options);
        return;
    }

    /* Request data via SelectionRead */
    GError *error = NULL;
    GUnixFDList *out_fds = NULL;
    GVariant *read_res = g_dbus_connection_call_with_unix_fd_list_sync(
        ctx->conn,
        "org.gnome.Mutter.RemoteDesktop",
        ctx->session_path,
        "org.gnome.Mutter.RemoteDesktop.Session",
        "SelectionRead",
        g_variant_new("(s)", chosen_mime),
        G_VARIANT_TYPE("(h)"),
        G_DBUS_CALL_FLAGS_NONE,
        2000,
        NULL,
        &out_fds,
        NULL,
        &error
    );

    g_free(mimes);
    g_variant_unref(mime_types_var);
    g_variant_unref(options);

    if (!read_res) {
        if (error) g_error_free(error);
        return;
    }

    gint32 handle = -1;
    g_variant_get(read_res, "(h)", &handle);
    int fd = g_unix_fd_list_get(out_fds, handle, &error);
    g_variant_unref(read_res);
    g_object_unref(out_fds);

    if (fd < 0) {
        if (error) g_error_free(error);
        return;
    }

    /* Read from Unix FD with poll timeout */
    char *buf = malloc(MAX_READ_BUFFER);
    if (!buf) {
        close(fd);
        return;
    }

    size_t total = 0;
    bool oversized = false;

    while (total < MAX_READ_BUFFER) {
        struct pollfd pfd = { .fd = fd, .events = POLLIN, .revents = 0 };
        int ret = poll(&pfd, 1, READ_TIMEOUT_MS);
        if (ret <= 0) break;

        ssize_t n = read(fd, buf + total, MAX_READ_BUFFER - total);
        if (n > 0) {
            total += (size_t)n;
            if (total > MAX_PAYLOAD_SIZE) {
                oversized = true;
                break;
            }
        } else if (n == 0) {
            break;
        } else {
            if (errno == EINTR) continue;
            break;
        }
    }
    close(fd);

    if (oversized) {
        fprintf(stdout, "EVENT=READ_OVERSIZED size=%zu\n", total);
        fflush(stdout);
        free(buf);
        return;
    }

    if (total == 0) {
        fprintf(stdout, "EVENT=SELECTION_CLEARED\n");
        fflush(stdout);
        free(buf);
        return;
    }

    /* Zero Logging Rule: NEVER log clipboard payload */
    fprintf(stdout, "EVENT=READ_DATA mime=%s len=%zu\n", chosen_mime, total);
    fwrite(buf, 1, total, stdout);
    fputc('\n', stdout);
    fflush(stdout);
    free(buf);
}

static void on_selection_transfer(GDBusConnection *conn,
                                  const gchar *sender_name,
                                  const gchar *object_path,
                                  const gchar *interface_name,
                                  const gchar *signal_name,
                                  GVariant *parameters,
                                  gpointer user_data) {
    (void)sender_name; (void)object_path; (void)interface_name; (void)signal_name;
    struct helper_context *ctx = (struct helper_context *)user_data;

    const gchar *mime = NULL;
    guint32 serial = 0;
    g_variant_get(parameters, "(&su)", &mime, &serial);

    if (!ctx->selection_payload || ctx->selection_len == 0) {
        /* No payload held */
        GVariant *done_res = g_dbus_connection_call_sync(
            conn,
            "org.gnome.Mutter.RemoteDesktop",
            ctx->session_path,
            "org.gnome.Mutter.RemoteDesktop.Session",
            "SelectionWriteDone",
            g_variant_new("(ub)", serial, FALSE),
            G_VARIANT_TYPE("()"),
            G_DBUS_CALL_FLAGS_NONE,
            1000,
            NULL,
            NULL
        );
        if (done_res) g_variant_unref(done_res);
        return;
    }

    GError *error = NULL;
    GUnixFDList *out_fds = NULL;
    GVariant *w_res = g_dbus_connection_call_with_unix_fd_list_sync(
        conn,
        "org.gnome.Mutter.RemoteDesktop",
        ctx->session_path,
        "org.gnome.Mutter.RemoteDesktop.Session",
        "SelectionWrite",
        g_variant_new("(u)", serial),
        G_VARIANT_TYPE("(h)"),
        G_DBUS_CALL_FLAGS_NONE,
        2000,
        NULL,
        &out_fds,
        NULL,
        &error
    );

    if (!w_res) {
        if (error) g_error_free(error);
        return;
    }

    gint32 handle = -1;
    g_variant_get(w_res, "(h)", &handle);
    int fd = g_unix_fd_list_get(out_fds, handle, &error);
    g_variant_unref(w_res);
    g_object_unref(out_fds);

    if (fd < 0) {
        if (error) g_error_free(error);
        return;
    }

    /* Write selection payload to fd */
    size_t written = 0;
    bool write_ok = true;
    while (written < ctx->selection_len) {
        struct pollfd pfd = { .fd = fd, .events = POLLOUT, .revents = 0 };
        int ret = poll(&pfd, 1, 1000);
        if (ret <= 0) { write_ok = false; break; }
        ssize_t nw = write(fd, ctx->selection_payload + written, ctx->selection_len - written);
        if (nw > 0) {
            written += (size_t)nw;
        } else if (nw < 0) {
            if (errno == EINTR) continue;
            write_ok = false;
            break;
        }
    }
    close(fd);

    GVariant *done_res = g_dbus_connection_call_sync(
        conn,
        "org.gnome.Mutter.RemoteDesktop",
        ctx->session_path,
        "org.gnome.Mutter.RemoteDesktop.Session",
        "SelectionWriteDone",
        g_variant_new("(ub)", serial, (gboolean)write_ok),
        G_VARIANT_TYPE("()"),
        G_DBUS_CALL_FLAGS_NONE,
        1000,
        NULL,
        NULL
    );
    if (done_res) g_variant_unref(done_res);
}

/* --- Supervisor Command Processing --- */

static void handle_set_selection(struct helper_context *ctx, const char *args) {
    char mime[MAX_MIME_LEN];
    size_t len = 0;
    mime[0] = '\0';

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

    char *payload = malloc(len + 1);
    if (!payload) {
        fprintf(stdout, "STATUS=ERROR cmd=SET_SELECTION detail=out_of_memory\n");
        fflush(stdout);
        return;
    }

    size_t read_bytes = 0;
    while (read_bytes < len) {
        ssize_t n = read(STDIN_FILENO, payload + read_bytes, len - read_bytes);
        if (n > 0) {
            read_bytes += (size_t)n;
        } else if (n < 0 && errno == EINTR) {
            continue;
        } else {
            free(payload);
            fprintf(stdout, "STATUS=ERROR cmd=SET_SELECTION detail=payload_read_truncated\n");
            fflush(stdout);
            return;
        }
    }
    payload[len] = '\0';

    /* Consume trailing newline */
    char trailing;
    while (1) {
        ssize_t nr = read(STDIN_FILENO, &trailing, 1);
        if (nr > 0) break;
        if (nr < 0 && errno == EINTR) continue;
        break;
    }

    if (ctx->selection_payload) {
        free(ctx->selection_payload);
        ctx->selection_payload = NULL;
        ctx->selection_len = 0;
    }

    ctx->selection_payload = payload;
    ctx->selection_len = len;
    strncpy(ctx->selection_mime, mime, MAX_MIME_LEN - 1);
    ctx->selection_mime[MAX_MIME_LEN - 1] = '\0';

    /* Build MIME types array */
    GVariantBuilder mimes_builder;
    g_variant_builder_init(&mimes_builder, G_VARIANT_TYPE("as"));
    g_variant_builder_add(&mimes_builder, "s", "text/plain;charset=utf-8");
    g_variant_builder_add(&mimes_builder, "s", "text/plain");
    g_variant_builder_add(&mimes_builder, "s", "UTF8_STRING");
    g_variant_builder_add(&mimes_builder, "s", "STRING");
    g_variant_builder_add(&mimes_builder, "s", "TEXT");

    if (strcmp(mime, "text/plain;charset=utf-8") != 0 &&
        strcmp(mime, "text/plain") != 0 &&
        strcmp(mime, "UTF8_STRING") != 0) {
        g_variant_builder_add(&mimes_builder, "s", mime);
    }

    GVariantBuilder opt_builder;
    g_variant_builder_init(&opt_builder, G_VARIANT_TYPE("a{sv}"));
    g_variant_builder_add(&opt_builder, "{sv}", "mime-types", g_variant_builder_end(&mimes_builder));

    GError *error = NULL;
    GVariant *set_res = g_dbus_connection_call_sync(
        ctx->conn,
        "org.gnome.Mutter.RemoteDesktop",
        ctx->session_path,
        "org.gnome.Mutter.RemoteDesktop.Session",
        "SetSelection",
        g_variant_new("(a{sv})", &opt_builder),
        G_VARIANT_TYPE("()"),
        G_DBUS_CALL_FLAGS_NONE,
        1500,
        NULL,
        &error
    );

    if (!set_res) {
        fprintf(stdout, "STATUS=ERROR cmd=SET_SELECTION detail=%s\n", error ? error->message : "unknown");
        fflush(stdout);
        if (error) g_error_free(error);
        return;
    }

    g_variant_unref(set_res);
    fprintf(stdout, "STATUS=OK cmd=SET_SELECTION len=%zu\n", len);
    fflush(stdout);
}

static void handle_clear_selection(struct helper_context *ctx) {
    if (ctx->selection_payload) {
        free(ctx->selection_payload);
        ctx->selection_payload = NULL;
        ctx->selection_len = 0;
    }

    GVariantBuilder mimes_builder;
    g_variant_builder_init(&mimes_builder, G_VARIANT_TYPE("as"));

    GVariantBuilder opt_builder;
    g_variant_builder_init(&opt_builder, G_VARIANT_TYPE("a{sv}"));
    g_variant_builder_add(&opt_builder, "{sv}", "mime-types", g_variant_builder_end(&mimes_builder));

    GVariant *clear_res = g_dbus_connection_call_sync(
        ctx->conn,
        "org.gnome.Mutter.RemoteDesktop",
        ctx->session_path,
        "org.gnome.Mutter.RemoteDesktop.Session",
        "SetSelection",
        g_variant_new("(a{sv})", &opt_builder),
        G_VARIANT_TYPE("()"),
        G_DBUS_CALL_FLAGS_NONE,
        1500,
        NULL,
        NULL
    );
    if (clear_res) g_variant_unref(clear_res);

    fprintf(stdout, "STATUS=OK cmd=CLEAR_SELECTION\n");
    fflush(stdout);
}

static gboolean on_stdin_readable(GIOChannel *source, GIOCondition cond, gpointer user_data) {
    (void)source;
    struct helper_context *ctx = (struct helper_context *)user_data;
    if (cond & (G_IO_HUP | G_IO_ERR)) {
        /* Pipe closed by supervisor */
        ctx->running = false;
        if (ctx->loop) g_main_loop_quit(ctx->loop);
        return FALSE;
    }

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
            return TRUE;
        } else {
            /* EOF */
            ctx->running = false;
            if (ctx->loop) g_main_loop_quit(ctx->loop);
            return FALSE;
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
        if (ctx->loop) g_main_loop_quit(ctx->loop);
        return FALSE;
    } else if (idx > 0) {
        fprintf(stdout, "STATUS=ERROR cmd=UNKNOWN detail=unrecognized_command\n");
        fflush(stdout);
    }

    return TRUE;
}

int main(int argc, char *argv[]) {
    signal(SIGPIPE, SIG_IGN);

    bool probe_only = (argc > 1 && strcmp(argv[1], "probe") == 0);

    struct helper_context ctx;
    memset(&ctx, 0, sizeof(ctx));
    ctx.running = true;
    ctx.probe_only = probe_only;

    GError *error = NULL;
    ctx.conn = g_bus_get_sync(G_BUS_TYPE_SESSION, NULL, &error);
    if (!ctx.conn) {
        fprintf(stdout, "STATUS=ERR_DBUS_CONNECT detail=%s\n", error ? error->message : "unknown");
        fflush(stdout);
        if (error) g_error_free(error);
        return 1;
    }

    GVariant *create_res = g_dbus_connection_call_sync(
        ctx.conn,
        "org.gnome.Mutter.RemoteDesktop",
        "/org/gnome/Mutter/RemoteDesktop",
        "org.gnome.Mutter.RemoteDesktop",
        "CreateSession",
        NULL,
        G_VARIANT_TYPE("(o)"),
        G_DBUS_CALL_FLAGS_NONE,
        2000,
        NULL,
        &error
    );

    if (!create_res) {
        fprintf(stdout, "STATUS=ERR_NO_MUTTER detail=%s\n", error ? error->message : "mutter_remote_desktop_unavailable");
        fflush(stdout);
        if (error) g_error_free(error);
        g_object_unref(ctx.conn);
        return 2;
    }

    const gchar *path = NULL;
    g_variant_get(create_res, "(&o)", &path);
    ctx.session_path = g_strdup(path);
    g_variant_unref(create_res);

    GVariant *start_res = g_dbus_connection_call_sync(
        ctx.conn,
        "org.gnome.Mutter.RemoteDesktop",
        ctx.session_path,
        "org.gnome.Mutter.RemoteDesktop.Session",
        "Start",
        NULL,
        G_VARIANT_TYPE("()"),
        G_DBUS_CALL_FLAGS_NONE,
        2000,
        NULL,
        &error
    );

    if (!start_res) {
        fprintf(stdout, "STATUS=ERR_MUTTER_START detail=%s\n", error ? error->message : "session_start_failed");
        fflush(stdout);
        if (error) g_error_free(error);
        g_free(ctx.session_path);
        g_object_unref(ctx.conn);
        return 2;
    }
    g_variant_unref(start_res);
    ctx.session_active = true;

    /* Subscribe to signals before EnableClipboard */
    ctx.owner_changed_sub_id = g_dbus_connection_signal_subscribe(
        ctx.conn,
        "org.gnome.Mutter.RemoteDesktop",
        "org.gnome.Mutter.RemoteDesktop.Session",
        "SelectionOwnerChanged",
        ctx.session_path,
        NULL,
        G_DBUS_SIGNAL_FLAGS_NONE,
        on_selection_owner_changed,
        &ctx,
        NULL
    );

    ctx.transfer_sub_id = g_dbus_connection_signal_subscribe(
        ctx.conn,
        "org.gnome.Mutter.RemoteDesktop",
        "org.gnome.Mutter.RemoteDesktop.Session",
        "SelectionTransfer",
        ctx.session_path,
        NULL,
        G_DBUS_SIGNAL_FLAGS_NONE,
        on_selection_transfer,
        &ctx,
        NULL
    );

    GVariantBuilder opt_builder;
    g_variant_builder_init(&opt_builder, G_VARIANT_TYPE("a{sv}"));
    g_variant_builder_add(&opt_builder, "{sv}", "mimetype-groups", g_variant_new_uint32(1));
    GVariant *en_res = g_dbus_connection_call_sync(
        ctx.conn,
        "org.gnome.Mutter.RemoteDesktop",
        ctx.session_path,
        "org.gnome.Mutter.RemoteDesktop.Session",
        "EnableClipboard",
        g_variant_new("(a{sv})", &opt_builder),
        G_VARIANT_TYPE("()"),
        G_DBUS_CALL_FLAGS_NONE,
        2000,
        NULL,
        &error
    );

    if (!en_res) {
        fprintf(stdout, "STATUS=ERR_MUTTER_CLIPBOARD detail=%s\n", error ? error->message : "enable_clipboard_failed");
        fflush(stdout);
        if (error) g_error_free(error);
        (void)g_dbus_connection_call_sync(ctx.conn, "org.gnome.Mutter.RemoteDesktop", ctx.session_path, "org.gnome.Mutter.RemoteDesktop.Session", "Stop", NULL, G_VARIANT_TYPE("()"), G_DBUS_CALL_FLAGS_NONE, 1000, NULL, NULL);
        g_free(ctx.session_path);
        g_object_unref(ctx.conn);
        return 2;
    }
    g_variant_unref(en_res);

    if (ctx.probe_only) {
        fprintf(stdout, "STATUS=READY compositor=GNOME backend=mutter\n");
        fflush(stdout);

        // Clean termination
        (void)g_dbus_connection_call_sync(ctx.conn, "org.gnome.Mutter.RemoteDesktop", ctx.session_path, "org.gnome.Mutter.RemoteDesktop.Session", "DisableClipboard", NULL, G_VARIANT_TYPE("()"), G_DBUS_CALL_FLAGS_NONE, 1000, NULL, NULL);
        (void)g_dbus_connection_call_sync(ctx.conn, "org.gnome.Mutter.RemoteDesktop", ctx.session_path, "org.gnome.Mutter.RemoteDesktop.Session", "Stop", NULL, G_VARIANT_TYPE("()"), G_DBUS_CALL_FLAGS_NONE, 1000, NULL, NULL);
        g_free(ctx.session_path);
        g_object_unref(ctx.conn);
        return 0;
    }

    /* Handshake: Ready signal emitted */
    fprintf(stdout, "STATUS=READY compositor=GNOME backend=mutter\n");
    fflush(stdout);

    /* Set up stdin channel */
    GIOChannel *stdin_ch = g_io_channel_unix_new(STDIN_FILENO);
    g_io_channel_set_encoding(stdin_ch, NULL, NULL);
    g_io_channel_set_buffered(stdin_ch, FALSE);
    guint stdin_watch = g_io_add_watch(stdin_ch, G_IO_IN | G_IO_HUP | G_IO_ERR, on_stdin_readable, &ctx);

    ctx.loop = g_main_loop_new(NULL, FALSE);
    g_main_loop_run(ctx.loop);

    /* Cleanup */
    g_source_remove(stdin_watch);
    g_io_channel_unref(stdin_ch);

    if (ctx.owner_changed_sub_id > 0)
        g_dbus_connection_signal_unsubscribe(ctx.conn, ctx.owner_changed_sub_id);
    if (ctx.transfer_sub_id > 0)
        g_dbus_connection_signal_unsubscribe(ctx.conn, ctx.transfer_sub_id);

    (void)g_dbus_connection_call_sync(ctx.conn, "org.gnome.Mutter.RemoteDesktop", ctx.session_path, "org.gnome.Mutter.RemoteDesktop.Session", "DisableClipboard", NULL, G_VARIANT_TYPE("()"), G_DBUS_CALL_FLAGS_NONE, 1000, NULL, NULL);
    (void)g_dbus_connection_call_sync(ctx.conn, "org.gnome.Mutter.RemoteDesktop", ctx.session_path, "org.gnome.Mutter.RemoteDesktop.Session", "Stop", NULL, G_VARIANT_TYPE("()"), G_DBUS_CALL_FLAGS_NONE, 1000, NULL, NULL);

    if (ctx.selection_payload) {
        free(ctx.selection_payload);
        ctx.selection_payload = NULL;
    }
    if (ctx.loop) g_main_loop_unref(ctx.loop);
    g_free(ctx.session_path);
    g_object_unref(ctx.conn);

    return 0;
}
