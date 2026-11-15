#define _GNU_SOURCE
#include <gtk/gtk.h>
#include <gdk/gdk.h>
#include <glib/gstdio.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <unistd.h>
#include <errno.h>
#include <string.h>
#include <stdlib.h>
#include <stdio.h>
#include <limits.h>

#define APP_ID "com.shoaib.scompress"
#define APP_VERSION "2.4.2"
#define MAX_FILES 512

typedef struct {
    GtkApplication *application;
    GtkWidget *window;
    GtkWidget *file_list;
    GtkWidget *list_scroller;
    GtkWidget *count_label;
    GtkWidget *toast_label;
    GtkWidget *preset_dropdown;
    GtkWidget *target_custom_box;
    GtkWidget *target_entry;
    GtkWidget *dimension_dropdown;
    GtkWidget *dimension_custom_box;
    GtkWidget *width_spin;
    GtkWidget *height_spin;
    GtkWidget *format_dropdown;
    GtkWidget *replace_check;
    GtkWidget *advanced_expander;
    GtkWidget *quality_scale;
    GtkWidget *quality_label;
    GtkWidget *metadata_dropdown;
    GtkWidget *first_frame_check;
    GtkWidget *output_entry;
    GtkWidget *output_browse_button;
    GtkWidget *run_button;
    GtkWidget *add_button;
    GtkWidget *clear_button;
    GtkWidget *settings_box;

    /* Floating Activity Monitor Modal Widgets */
    GtkWidget *activity_modal;
    GtkWidget *modal_spinner;
    GtkWidget *modal_status_badge;
    GtkWidget *modal_count_label;
    GtkWidget *modal_title_label;
    GtkWidget *modal_sub_label;
    GtkWidget *modal_result_view;
    GtkWidget *modal_close_btn;

    GPtrArray *files;
    gchar *cli_path;
    gboolean busy;
    guint completed_files;
    guint total_files;
} AppState;

typedef struct {
    AppState *state;
    gchar **files;
    guint file_count;
    gchar *cli_path;
    gint target_kb;
    gint quality;
    gint format_index;
    gint metadata_index;
    gint max_width;
    gint max_height;
    gboolean no_resize;
    gboolean first_frame;
    gboolean replace_original;
    gchar *output_dir;
} Job;

typedef struct {
    gboolean success;
    guint processed_count;
    gchar *text;
} JobResult;

typedef struct {
    AppState *state;
    guint completed_files;
    gchar *headline;
    gchar *detail;
} ProgressTick;

typedef struct {
    gchar *source;
    gchar *stage_dir;
    gchar *stage_file;
    gchar *backup_file;
    struct stat before;
    gboolean has_output;
    gboolean backup_created;
    gboolean committed;
} ReplaceItem;

static const char *format_labels[] = {
    "Default (Keep original)", "WebP", "JPEG", "PNG", "GIF", NULL
};
static const char *format_values[] = { "", "webp", "jpeg", "png", "gif" };

static const char *metadata_labels[] = {
    "Privacy mode — strip identifying EXIF",
    "Keep supported metadata (JPEG/PNG)",
    "Remove all metadata",
    NULL
};

static const char *preset_labels[] = {
    "100 KB", "200 KB", "500 KB", "Custom", NULL
};
static const gint preset_values[] = { 100, 200, 500, 0 };

static const char *dimension_labels[] = {
    "Default", "Custom", NULL
};

static void refresh_file_list(AppState *state);
static void reset_app_state(AppState *state);

static gboolean is_supported_image(const gchar *path) {
    if (!path) return FALSE;
    const gchar *dot = strrchr(path, '.');
    if (!dot) return FALSE;
    gchar *ext = g_ascii_strdown(dot + 1, -1);
    gboolean ok = g_str_equal(ext, "jpg") || g_str_equal(ext, "jpeg") ||
                  g_str_equal(ext, "png") || g_str_equal(ext, "gif") ||
                  g_str_equal(ext, "webp");
    g_free(ext);
    return ok;
}

static gchar *format_size(goffset bytes) {
    if (bytes < 1000) return g_strdup_printf("%" G_GOFFSET_FORMAT " B", bytes);
    if (bytes < 1000000) return g_strdup_printf("%.1f KB", (double)bytes / 1000.0);
    return g_strdup_printf("%.2f MB", (double)bytes / 1000000.0);
}

static void set_status(AppState *state, const gchar *message) {
    if (!state || !state->toast_label) return;
    if (message && *message) {
        gtk_label_set_text(GTK_LABEL(state->toast_label), message);
        gtk_widget_set_visible(state->toast_label, TRUE);
    } else {
        gtk_label_set_text(GTK_LABEL(state->toast_label), "");
        gtk_widget_set_visible(state->toast_label, FALSE);
    }
}

static void on_remove_single_file(GtkButton *button, gpointer user_data) {
    AppState *state = user_data;
    if (state->busy) return;
    const gchar *path = g_object_get_data(G_OBJECT(button), "file-path");
    if (!path) return;
    for (guint i = 0; i < state->files->len; i++) {
        if (g_str_equal(path, g_ptr_array_index(state->files, i))) {
            g_ptr_array_remove_index(state->files, i);
            break;
        }
    }
    refresh_file_list(state);
}

static void refresh_file_list(AppState *state) {
    GtkWidget *child;
    while ((child = gtk_widget_get_first_child(state->file_list)) != NULL)
        gtk_list_box_remove(GTK_LIST_BOX(state->file_list), child);

    if (state->files->len == 0) {
        GtkWidget *empty_box = gtk_box_new(GTK_ORIENTATION_VERTICAL, 5);
        gtk_widget_add_css_class(empty_box, "empty-state");
        gtk_widget_set_valign(empty_box, GTK_ALIGN_CENTER);
        gtk_widget_set_halign(empty_box, GTK_ALIGN_CENTER);

        GtkWidget *empty_icon = gtk_image_new_from_icon_name("folder-pictures-symbolic");
        gtk_image_set_pixel_size(GTK_IMAGE(empty_icon), 26);
        gtk_widget_add_css_class(empty_icon, "empty-icon");

        GtkWidget *empty_title = gtk_label_new("Drop images here or click “＋ Add Images”");
        gtk_widget_add_css_class(empty_title, "empty-title");

        GtkWidget *empty_sub = gtk_label_new("Supports JPEG, PNG, WebP, and GIF (up to 512 files)");
        gtk_widget_add_css_class(empty_sub, "dim-label");

        gtk_box_append(GTK_BOX(empty_box), empty_icon);
        gtk_box_append(GTK_BOX(empty_box), empty_title);
        gtk_box_append(GTK_BOX(empty_box), empty_sub);
        gtk_list_box_append(GTK_LIST_BOX(state->file_list), empty_box);
    } else {
        for (guint i = 0; i < state->files->len; i++) {
            const gchar *path = g_ptr_array_index(state->files, i);
            GFile *file = g_file_new_for_path(path);
            GFileInfo *info = g_file_query_info(file, G_FILE_ATTRIBUTE_STANDARD_SIZE,
                                                G_FILE_QUERY_INFO_NONE, NULL, NULL);
            goffset bytes = info ? g_file_info_get_size(info) : 0;
            gchar *size_text = format_size(bytes);
            gchar *name = g_path_get_basename(path);

            GtkWidget *row = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 10);
            gtk_widget_add_css_class(row, "file-row");
            gtk_widget_set_valign(row, GTK_ALIGN_CENTER);

            GtkWidget *icon = gtk_image_new_from_icon_name("image-x-generic-symbolic");
            gtk_image_set_pixel_size(GTK_IMAGE(icon), 18);
            gtk_widget_add_css_class(icon, "file-icon");

            GtkWidget *labels = gtk_box_new(GTK_ORIENTATION_VERTICAL, 1);
            gtk_widget_set_hexpand(labels, TRUE);
            gtk_widget_set_valign(labels, GTK_ALIGN_CENTER);

            GtkWidget *name_label = gtk_label_new(name);
            gtk_label_set_xalign(GTK_LABEL(name_label), 0.0f);
            gtk_label_set_ellipsize(GTK_LABEL(name_label), PANGO_ELLIPSIZE_MIDDLE);
            gtk_widget_add_css_class(name_label, "file-name");

            GtkWidget *path_label = gtk_label_new(path);
            gtk_label_set_xalign(GTK_LABEL(path_label), 0.0f);
            gtk_label_set_ellipsize(GTK_LABEL(path_label), PANGO_ELLIPSIZE_MIDDLE);
            gtk_widget_add_css_class(path_label, "dim-label");

            gtk_box_append(GTK_BOX(labels), name_label);
            gtk_box_append(GTK_BOX(labels), path_label);

            GtkWidget *size_label = gtk_label_new(size_text);
            gtk_widget_add_css_class(size_label, "size-badge");
            gtk_widget_set_valign(size_label, GTK_ALIGN_CENTER);

            GtkWidget *remove_btn = gtk_button_new_from_icon_name("window-close-symbolic");
            gtk_widget_add_css_class(remove_btn, "flat");
            gtk_widget_add_css_class(remove_btn, "remove-btn");
            gtk_widget_set_valign(remove_btn, GTK_ALIGN_CENTER);
            gtk_widget_set_tooltip_text(remove_btn, "Remove image");
            g_object_set_data_full(G_OBJECT(remove_btn), "file-path", g_strdup(path), g_free);
            g_signal_connect(remove_btn, "clicked", G_CALLBACK(on_remove_single_file), state);

            gtk_box_append(GTK_BOX(row), icon);
            gtk_box_append(GTK_BOX(row), labels);
            gtk_box_append(GTK_BOX(row), size_label);
            gtk_box_append(GTK_BOX(row), remove_btn);
            gtk_list_box_append(GTK_LIST_BOX(state->file_list), row);

            g_free(name);
            g_free(size_text);
            g_clear_object(&info);
            g_object_unref(file);
        }
    }

    gchar *count = NULL;
    if (state->files->len == 0)
        count = g_strdup("No images selected");
    else if (state->files->len == 1)
        count = g_strdup("One image selected");
    else
        count = g_strdup_printf("%u images selected", state->files->len);

    gtk_label_set_text(GTK_LABEL(state->count_label), count);
    g_free(count);
    gtk_widget_set_sensitive(state->run_button, state->files->len > 0 && !state->busy);
    if (state->clear_button)
        gtk_widget_set_sensitive(state->clear_button, state->files->len > 0 && !state->busy);
}

static gboolean add_file(AppState *state, const gchar *path) {
    if (!path || !is_supported_image(path) || !g_file_test(path, G_FILE_TEST_IS_REGULAR))
        return FALSE;
    gchar *canonical = g_canonicalize_filename(path, NULL);
    for (guint i = 0; i < state->files->len; i++) {
        if (g_str_equal(canonical, g_ptr_array_index(state->files, i))) {
            g_free(canonical);
            return FALSE;
        }
    }
    if (state->files->len >= MAX_FILES) {
        set_status(state, "Selection limit reached (512 files). Remove some files first.");
        g_free(canonical);
        return FALSE;
    }
    g_ptr_array_add(state->files, canonical);
    return TRUE;
}

/* Drag and Drop Handlers */
static GdkDragAction on_drop_enter(GtkDropTarget *target, gdouble x, gdouble y, gpointer user_data) {
    (void)target; (void)x; (void)y;
    AppState *state = user_data;
    if (state->list_scroller)
        gtk_widget_add_css_class(state->list_scroller, "drop-active");
    return GDK_ACTION_COPY;
}

static void on_drop_leave(GtkDropTarget *target, gpointer user_data) {
    (void)target;
    AppState *state = user_data;
    if (state->list_scroller)
        gtk_widget_remove_css_class(state->list_scroller, "drop-active");
}

static gboolean on_drop_files(GtkDropTarget *target, const GValue *value, gdouble x, gdouble y, gpointer user_data) {
    (void)target; (void)x; (void)y;
    AppState *state = user_data;
    if (state->list_scroller)
        gtk_widget_remove_css_class(state->list_scroller, "drop-active");
    if (state->busy) return FALSE;

    guint added = 0;
#if GTK_CHECK_VERSION(4, 6, 0)
    if (G_VALUE_HOLDS(value, GDK_TYPE_FILE_LIST)) {
        GdkFileList *file_list = g_value_get_boxed(value);
        GSList *list = gdk_file_list_get_files(file_list);
        for (GSList *l = list; l != NULL; l = l->next) {
            GFile *file = G_FILE(l->data);
            gchar *path = g_file_get_path(file);
            if (path) {
                if (add_file(state, path)) added++;
                g_free(path);
            }
        }
        g_slist_free(list);
    } else
#endif
    if (G_VALUE_HOLDS(value, G_TYPE_FILE)) {
        GFile *file = g_value_get_object(value);
        gchar *path = g_file_get_path(file);
        if (path) {
            if (add_file(state, path)) added++;
            g_free(path);
        }
    } else if (G_VALUE_HOLDS(value, G_TYPE_STRING)) {
        const gchar *text = g_value_get_string(value);
        if (text) {
            gchar **uris = g_uri_list_extract_uris(text);
            for (int i = 0; uris && uris[i] != NULL; i++) {
                gchar *path = g_filename_from_uri(uris[i], NULL, NULL);
                if (!path && g_file_test(uris[i], G_FILE_TEST_IS_REGULAR))
                    path = g_strdup(uris[i]);
                if (path) {
                    if (add_file(state, path)) added++;
                    g_free(path);
                }
            }
            g_strfreev(uris);
        }
    }

    if (added > 0) {
        set_status(state, NULL);
        refresh_file_list(state);
        return TRUE;
    } else {
        set_status(state, "No supported new images found in dropped items.");
        return FALSE;
    }
}

static void on_clear_files(GtkButton *button, gpointer user_data) {
    (void)button;
    AppState *state = user_data;
    g_ptr_array_set_size(state->files, 0);
    set_status(state, NULL);
    refresh_file_list(state);
}

static void add_files_from_model(AppState *state, GListModel *model) {
    if (!model) return;
    guint n = g_list_model_get_n_items(model);
    for (guint i = 0; i < n; i++) {
        GFile *file = g_list_model_get_item(model, i);
        gchar *path = g_file_get_path(file);
        if (path) add_file(state, path);
        g_free(path);
        g_object_unref(file);
    }
    set_status(state, NULL);
    refresh_file_list(state);
}

static void on_open_files_done(GObject *source_object, GAsyncResult *result, gpointer user_data) {
    AppState *state = user_data;
    GError *error = NULL;
    GListModel *model = gtk_file_dialog_open_multiple_finish(GTK_FILE_DIALOG(source_object), result, &error);
    if (model) {
        add_files_from_model(state, model);
        g_object_unref(model);
    } else if (error && !g_error_matches(error, GTK_DIALOG_ERROR, GTK_DIALOG_ERROR_DISMISSED)) {
        set_status(state, error->message);
    }
    g_clear_error(&error);
    g_object_unref(source_object);
}

static void on_add_files(GtkButton *button, gpointer user_data) {
    (void)button;
    AppState *state = user_data;
    GtkFileDialog *dialog = gtk_file_dialog_new();
    gtk_file_dialog_set_title(dialog, "Add images to scompress");
    gtk_file_dialog_set_accept_label(dialog, "Add Images");
    gtk_file_dialog_set_modal(dialog, TRUE);
    gtk_file_dialog_open_multiple(dialog, GTK_WINDOW(state->window), NULL,
                                  on_open_files_done, state);
}

static void on_output_folder_done(GObject *source_object, GAsyncResult *result, gpointer user_data) {
    AppState *state = user_data;
    GError *error = NULL;
    GFile *folder = gtk_file_dialog_select_folder_finish(GTK_FILE_DIALOG(source_object), result, &error);
    if (folder) {
        gchar *path = g_file_get_path(folder);
        if (path) gtk_editable_set_text(GTK_EDITABLE(state->output_entry), path);
        g_free(path);
        g_object_unref(folder);
    } else if (error && !g_error_matches(error, GTK_DIALOG_ERROR, GTK_DIALOG_ERROR_DISMISSED)) {
        set_status(state, error->message);
    }
    g_clear_error(&error);
    g_object_unref(source_object);
}

static void on_choose_output(GtkButton *button, gpointer user_data) {
    (void)button;
    AppState *state = user_data;
    GtkFileDialog *dialog = gtk_file_dialog_new();
    gtk_file_dialog_set_title(dialog, "Choose output folder");
    gtk_file_dialog_set_accept_label(dialog, "Select Folder");
    gtk_file_dialog_set_modal(dialog, TRUE);
    gtk_file_dialog_select_folder(dialog, GTK_WINDOW(state->window), NULL,
                                  on_output_folder_done, state);
}

static void on_preset_changed(GObject *object, GParamSpec *pspec, gpointer user_data) {
    (void)pspec;
    AppState *state = user_data;
    guint index = gtk_drop_down_get_selected(GTK_DROP_DOWN(object));
    if (index < 3) {
        gchar *value = g_strdup_printf("%d", preset_values[index]);
        gtk_editable_set_text(GTK_EDITABLE(state->target_entry), value);
        g_free(value);
        gtk_widget_set_visible(state->target_custom_box, FALSE);
    } else {
        gtk_widget_set_visible(state->target_custom_box, TRUE);
        gtk_widget_grab_focus(state->target_entry);
    }
}

static void on_dimension_changed(GObject *object, GParamSpec *pspec, gpointer user_data) {
    (void)pspec;
    AppState *state = user_data;
    guint index = gtk_drop_down_get_selected(GTK_DROP_DOWN(object));
    if (index == 0) {
        gtk_spin_button_set_value(GTK_SPIN_BUTTON(state->width_spin), 0);
        gtk_spin_button_set_value(GTK_SPIN_BUTTON(state->height_spin), 0);
        gtk_widget_set_visible(state->dimension_custom_box, FALSE);
    } else {
        gtk_widget_set_visible(state->dimension_custom_box, TRUE);
        gtk_widget_grab_focus(state->width_spin);
    }
}

static void on_quality_changed(GtkRange *range, gpointer user_data) {
    AppState *state = user_data;
    gint quality = (gint)gtk_range_get_value(range);
    gchar *label = g_strdup_printf("%d%%", quality);
    gtk_label_set_text(GTK_LABEL(state->quality_label), label);
    g_free(label);
}

static void on_replace_toggled(GtkCheckButton *button, gpointer user_data) {
    AppState *state = user_data;
    gboolean replace = gtk_check_button_get_active(button);
    gtk_widget_set_sensitive(state->format_dropdown, !replace);
    gtk_widget_set_sensitive(state->output_entry, !replace);
    gtk_widget_set_sensitive(state->output_browse_button, !replace);
    if (replace) {
        gtk_drop_down_set_selected(GTK_DROP_DOWN(state->format_dropdown), 0);
        gtk_editable_set_text(GTK_EDITABLE(state->output_entry), "");
    }
    set_status(state, NULL);
}

static void reset_app_state(AppState *state) {
    g_ptr_array_set_size(state->files, 0);

    gtk_drop_down_set_selected(GTK_DROP_DOWN(state->preset_dropdown), 2); /* 500 KB */
    gtk_editable_set_text(GTK_EDITABLE(state->target_entry), "500");
    gtk_widget_set_visible(state->target_custom_box, FALSE);

    gtk_drop_down_set_selected(GTK_DROP_DOWN(state->dimension_dropdown), 0); /* Default */
    gtk_spin_button_set_value(GTK_SPIN_BUTTON(state->width_spin), 0);
    gtk_spin_button_set_value(GTK_SPIN_BUTTON(state->height_spin), 0);
    gtk_widget_set_visible(state->dimension_custom_box, FALSE);

    gtk_drop_down_set_selected(GTK_DROP_DOWN(state->format_dropdown), 0); /* Default format */
    gtk_check_button_set_active(GTK_CHECK_BUTTON(state->replace_check), FALSE);

    gtk_range_set_value(GTK_RANGE(state->quality_scale), 95);
    gtk_drop_down_set_selected(GTK_DROP_DOWN(state->metadata_dropdown), 0);
    gtk_editable_set_text(GTK_EDITABLE(state->output_entry), "");
    gtk_check_button_set_active(GTK_CHECK_BUTTON(state->first_frame_check), FALSE);
    gtk_expander_set_expanded(GTK_EXPANDER(state->advanced_expander), FALSE);

    set_status(state, NULL);
    refresh_file_list(state);
}

static gchar *find_cli_binary(void) {
    gchar proc_path[PATH_MAX + 1];
    ssize_t length = readlink("/proc/self/exe", proc_path, PATH_MAX);
    if (length > 0) {
        proc_path[length] = '\0';
        gchar *directory = g_path_get_dirname(proc_path);
        gchar *sibling = g_build_filename(directory, "scompress", NULL);
        g_free(directory);
        if (g_file_test(sibling, G_FILE_TEST_IS_EXECUTABLE)) return sibling;
        g_free(sibling);
    }
    gchar *local = g_build_filename(g_get_home_dir(), ".local", "bin", "scompress", NULL);
    if (g_file_test(local, G_FILE_TEST_IS_EXECUTABLE)) return local;
    g_free(local);
    return g_find_program_in_path("scompress");
}

static void job_free(gpointer data) {
    Job *job = data;
    if (!job) return;
    g_strfreev(job->files);
    g_free(job->cli_path);
    g_free(job->output_dir);
    g_free(job);
}

static void job_result_free(gpointer data) {
    JobResult *result = data;
    if (!result) return;
    g_free(result->text);
    g_free(result);
}

/* Formats raw CLI output into an organized, aligned report block with separate Dimension field */
static void append_formatted_cli_block(GString *report, guint index, guint total,
                                       const gchar *filepath, const gchar *raw_output) {
    gchar *basename = g_path_get_basename(filepath);
    if (total > 1) {
        g_string_append_printf(report, "── Image %u of %u: %s ──\n", index, total, basename);
    }

    if (!raw_output || !*raw_output) {
        g_string_append_printf(report, "  File      : %s\n  Status    : Completed\n", basename);
        if (total > 1) g_string_append_c(report, '\n');
        g_free(basename);
        return;
    }

    gchar *size_val = NULL;
    gchar *saved_val = NULL;
    gchar *method_val = NULL;
    gchar *dim_val = NULL;
    GString *extra_lines = g_string_new(NULL);

    gchar **lines = g_strsplit(raw_output, "\n", -1);
    for (int i = 0; lines && lines[i] != NULL; i++) {
        gchar *line = g_strstrip(lines[i]);
        if (!*line) continue;

        /* Parse "✓ filename: 1.72 MB -> 162.0 KB (90.6% smaller)" */
        if (g_str_has_prefix(line, "✓") || g_str_has_prefix(line, "✔")) {
            const gchar *colon = strchr(line, ':');
            if (colon && *(colon + 1)) {
                g_free(size_val);
                size_val = g_strdup(g_strstrip((gchar *)(colon + 1)));
            } else {
                g_string_append_printf(extra_lines, "  %s\n", line);
            }
        } else if (g_str_has_prefix(line, "Saved:")) {
            g_free(saved_val);
            saved_val = g_strdup(g_strstrip(line + 6));
        } else if (g_str_has_prefix(line, "Method:")) {
            gchar *m_raw = g_strstrip(line + 7);
            gchar *dim_split = g_strstr_len(m_raw, -1, "; output dimensions");
            if (dim_split) {
                g_free(method_val);
                method_val = g_strndup(m_raw, dim_split - m_raw);
                g_strstrip(method_val);

                const gchar *d_start = dim_split + strlen("; output dimensions");
                while (*d_start == ' ' || *d_start == ':') d_start++;
                g_free(dim_val);
                dim_val = g_strdup(d_start);
                g_strstrip(dim_val);
            } else {
                dim_split = g_strstr_len(m_raw, -1, "; dimensions");
                if (dim_split) {
                    g_free(method_val);
                    method_val = g_strndup(m_raw, dim_split - m_raw);
                    g_strstrip(method_val);

                    const gchar *d_start = dim_split + strlen("; dimensions");
                    while (*d_start == ' ' || *d_start == ':') d_start++;
                    g_free(dim_val);
                    dim_val = g_strdup(d_start);
                    g_strstrip(dim_val);
                } else {
                    g_free(method_val);
                    method_val = g_strdup(m_raw);
                }
            }
        } else {
            g_string_append_printf(extra_lines, "  %s\n", line);
        }
    }

    g_string_append_printf(report, "  File      : %s\n", basename);
    if (size_val)
        g_string_append_printf(report, "  Size      : %s\n", size_val);
    if (dim_val && *dim_val)
        g_string_append_printf(report, "  Dimension : %s\n", dim_val);
    if (method_val)
        g_string_append_printf(report, "  Method    : %s\n", method_val);
    if (saved_val)
        g_string_append_printf(report, "  Saved     : %s\n", saved_val);
    if (extra_lines->len > 0)
        g_string_append(report, extra_lines->str);

    if (total > 1)
        g_string_append_c(report, '\n');

    g_free(size_val);
    g_free(saved_val);
    g_free(method_val);
    g_free(dim_val);
    g_string_free(extra_lines, TRUE);
    g_strfreev(lines);
    g_free(basename);
}

/* Progress is reported as completed file operations, never as an estimated percentage. */
static void update_progress_count(AppState *state) {
    if (!state || !state->modal_count_label) return;
    gchar *count = g_strdup_printf("%u / %u", MIN(state->completed_files, state->total_files),
                                   state->total_files);
    gtk_label_set_text(GTK_LABEL(state->modal_count_label), count);
    g_free(count);
}

static gboolean apply_progress_tick(gpointer user_data) {
    ProgressTick *tick = user_data;
    AppState *state = tick->state;
    if (state->busy && state->activity_modal) {
        state->completed_files = MAX(state->completed_files, MIN(tick->completed_files, state->total_files));
        update_progress_count(state);
        if (tick->headline)
            gtk_label_set_text(GTK_LABEL(state->modal_title_label), tick->headline);
        if (tick->detail)
            gtk_label_set_text(GTK_LABEL(state->modal_sub_label), tick->detail);
    }
    g_free(tick->headline);
    g_free(tick->detail);
    g_free(tick);
    return G_SOURCE_REMOVE;
}

static void emit_progress(AppState *state, guint completed_files,
                          const gchar *headline, const gchar *detail) {
    ProgressTick *tick = g_new0(ProgressTick, 1);
    tick->state = state;
    tick->completed_files = MIN(completed_files, state->total_files);
    tick->headline = g_strdup(headline);
    tick->detail = g_strdup(detail);
    g_idle_add(apply_progress_tick, tick);
}

static void argv_add(GPtrArray *args, const gchar *value) {
    g_ptr_array_add(args, g_strdup(value));
}

static gchar **build_cli_argv(const Job *job, const gchar *input, const gchar *stage_dir) {
    GPtrArray *args = g_ptr_array_new_with_free_func(g_free);
    argv_add(args, job->cli_path);
    if (input) argv_add(args, input);
    else {
        for (guint i = 0; i < job->file_count; i++) argv_add(args, job->files[i]);
    }

    gchar *option = g_strdup_printf("--target=%d", job->target_kb);
    argv_add(args, option); g_free(option);
    option = g_strdup_printf("--quality=%d", job->quality);
    argv_add(args, option); g_free(option);

    if (job->format_index > 0) {
        option = g_strdup_printf("--format=%s", format_values[job->format_index]);
        argv_add(args, option); g_free(option);
    }
    if (job->no_resize) argv_add(args, "--no-resize");
    else {
        if (job->max_width > 0) {
            option = g_strdup_printf("--max-width=%d", job->max_width);
            argv_add(args, option); g_free(option);
        }
        if (job->max_height > 0) {
            option = g_strdup_printf("--max-height=%d", job->max_height);
            argv_add(args, option); g_free(option);
        }
    }
    if (job->metadata_index == 1) argv_add(args, "--keep-metadata");
    if (job->metadata_index == 2) argv_add(args, "--no-metadata");
    if (job->first_frame) argv_add(args, "--first-frame");

    const gchar *out = stage_dir ? stage_dir : job->output_dir;
    if (out && *out) {
        option = g_strdup_printf("--output=%s", out);
        argv_add(args, option); g_free(option);
    }
    g_ptr_array_add(args, NULL);
    return (gchar **)g_ptr_array_free(args, FALSE);
}

static gboolean run_process(gchar **argv, gchar **combined_output, GError **error) {
    gchar *stdout_text = NULL;
    gchar *stderr_text = NULL;
    gint wait_status = 0;
    gboolean spawned = g_spawn_sync(NULL, argv, NULL, G_SPAWN_SEARCH_PATH,
                                    NULL, NULL, &stdout_text, &stderr_text,
                                    &wait_status, error);
    if (!spawned) {
        *combined_output = g_strdup(error && *error ? (*error)->message : "Unable to start scompress.");
        g_free(stdout_text);
        g_free(stderr_text);
        return FALSE;
    }
    GString *combined = g_string_new(NULL);
    if (stdout_text && *stdout_text) g_string_append(combined, stdout_text);
    if (stderr_text && *stderr_text) {
        if (combined->len && combined->str[combined->len - 1] != '\n') g_string_append_c(combined, '\n');
        g_string_append(combined, stderr_text);
    }
    if (!combined->len) g_string_append(combined, "scompress finished without additional output.");
    *combined_output = g_string_free(combined, FALSE);
    g_free(stdout_text);
    g_free(stderr_text);
    return g_spawn_check_wait_status(wait_status, error);
}

static gboolean same_source_file(const gchar *path, const struct stat *before) {
    struct stat now;
    if (g_lstat(path, &now) != 0 || !S_ISREG(now.st_mode)) return FALSE;
    return now.st_dev == before->st_dev && now.st_ino == before->st_ino &&
           now.st_size == before->st_size && now.st_mtim.tv_sec == before->st_mtim.tv_sec &&
           now.st_mtim.tv_nsec == before->st_mtim.tv_nsec &&
           now.st_ctim.tv_sec == before->st_ctim.tv_sec &&
           now.st_ctim.tv_nsec == before->st_ctim.tv_nsec;
}

static gchar *find_single_stage_file(const gchar *directory, GError **error) {
    GDir *dir = g_dir_open(directory, 0, error);
    if (!dir) return NULL;
    const gchar *name;
    gchar *found = NULL;
    while ((name = g_dir_read_name(dir)) != NULL) {
        if (name[0] == '.') continue;
        gchar *path = g_build_filename(directory, name, NULL);
        if (g_file_test(path, G_FILE_TEST_IS_REGULAR)) {
            if (found) {
                g_set_error(error, G_FILE_ERROR, G_FILE_ERROR_FAILED,
                            "More than one output appeared in the staging directory.");
                g_free(path);
                g_clear_pointer(&found, g_free);
                break;
            }
            found = path;
        } else {
            g_free(path);
        }
    }
    g_dir_close(dir);
    return found;
}

static void cleanup_replace_items(ReplaceItem *items, guint count, gboolean keep_backups) {
    for (guint i = 0; i < count; i++) {
        if (items[i].stage_file) g_unlink(items[i].stage_file);
        if (items[i].stage_dir) g_rmdir(items[i].stage_dir);
        if ((!keep_backups || !items[i].committed) && items[i].backup_file) g_unlink(items[i].backup_file);
        g_free(items[i].source);
        g_free(items[i].stage_dir);
        g_free(items[i].stage_file);
        g_free(items[i].backup_file);
    }
    g_free(items);
}

static gchar *create_backup_link(const gchar *source, const gchar *parent, GError **error) {
    gchar *template = g_build_filename(parent, ".scompress-backup-XXXXXX", NULL);
    gint fd = g_mkstemp(template);
    if (fd < 0) {
        g_set_error(error, G_FILE_ERROR, g_file_error_from_errno(errno),
                    "Could not reserve a backup filename: %s", g_strerror(errno));
        g_free(template);
        return NULL;
    }
    close(fd);
    if (g_unlink(template) != 0 || link(source, template) != 0) {
        gint saved_errno = errno;
        g_set_error(error, G_FILE_ERROR, g_file_error_from_errno(saved_errno),
                    "Could not create a recovery link for %s: %s", source, g_strerror(saved_errno));
        g_unlink(template);
        g_free(template);
        return NULL;
    }
    return template;
}

static JobResult *run_replace_job(const Job *job) {
    JobResult *result = g_new0(JobResult, 1);
    result->processed_count = 0;
    GString *log = g_string_new("SUMMARY REPORT (IN-PLACE REPLACEMENT)\n══════════════════════════════════════\n");
    ReplaceItem *items = g_new0(ReplaceItem, job->file_count);
    GError *error = NULL;
    gboolean failed = FALSE;
    guint staged = 0;
    guint skipped = 0;

    for (guint i = 0; i < job->file_count; i++) {
        ReplaceItem *item = &items[i];
        item->source = g_strdup(job->files[i]);
        gchar *basename = g_path_get_basename(item->source);
        gchar *headline = g_strdup_printf("Preparing image %u of %u…", i + 1, job->file_count);
        emit_progress(job->state, i, headline, basename);
        g_free(headline);
        g_free(basename);

        if (g_lstat(item->source, &item->before) != 0 || !S_ISREG(item->before.st_mode)) {
            g_string_append_printf(log, "  Error     : Cannot safely replace %s (not a regular file).\n", item->source);
            result->processed_count = i + 1;
            emit_progress(job->state, result->processed_count, "Unable to prepare selected file", item->source);
            failed = TRUE;
            break;
        }

        gchar *parent = g_path_get_dirname(item->source);
        gchar *stage_template = g_build_filename(parent, ".scompress-stage-XXXXXX", NULL);
        if (!g_mkdtemp(stage_template)) {
            g_string_append_printf(log, "  Error     : Could not create staging directory for %s: %s\n",
                                   item->source, g_strerror(errno));
            result->processed_count = i + 1;
            emit_progress(job->state, result->processed_count, "Unable to create a staging area", item->source);
            g_free(parent); g_free(stage_template); failed = TRUE; break;
        }
        item->stage_dir = stage_template;

        gchar **argv = build_cli_argv(job, item->source, item->stage_dir);
        gchar *output = NULL;
        gboolean ok = run_process(argv, &output, &error);
        g_strfreev(argv);
        result->processed_count = i + 1;
        emit_progress(job->state, result->processed_count,
                      ok ? "Image prepared" : "Image processing failed", item->source);
        append_formatted_cli_block(log, i + 1, job->file_count, item->source, output);
        g_free(output);
        if (!ok) {
            g_string_append_printf(log, "  Error     : %s\n", error ? error->message : "Compression failed");
            g_clear_error(&error);
            failed = TRUE;
            g_free(parent);
            break;
        }

        item->stage_file = find_single_stage_file(item->stage_dir, &error);
        if (error) {
            g_string_append_printf(log, "  Error     : Could not inspect staged output: %s\n", error->message);
            g_clear_error(&error);
            failed = TRUE;
            g_free(parent);
            break;
        }
        if (!item->stage_file) {
            skipped++;
            g_string_append(log, "  Note      : Already within target size; original left unchanged.\n");
            g_rmdir(item->stage_dir);
            g_clear_pointer(&item->stage_dir, g_free);
            g_free(parent);
            continue;
        }
        if (g_chmod(item->stage_file, item->before.st_mode & 07777) != 0) {
            g_string_append_printf(log, "  Error     : Could not preserve file permissions: %s\n", g_strerror(errno));
            failed = TRUE;
            g_free(parent);
            break;
        }
        item->has_output = TRUE;
        staged++;
        g_free(parent);
    }

    if (!failed) {
        emit_progress(job->state, result->processed_count, "Verifying original files…", "Checking that selected files have not changed");
        for (guint i = 0; i < job->file_count; i++) {
            if (!same_source_file(items[i].source, &items[i].before)) {
                g_string_append_printf(log, "  Error     : Source changed during compression; aborted: %s\n", items[i].source);
                failed = TRUE;
                break;
            }
        }
    }

    if (!failed && staged > 0) {
        for (guint i = 0; i < job->file_count; i++) {
            if (!items[i].has_output) continue;
            gchar *parent = g_path_get_dirname(items[i].source);
            items[i].backup_file = create_backup_link(items[i].source, parent, &error);
            g_free(parent);
            if (!items[i].backup_file) {
                g_string_append_printf(log, "  Error     : Could not prepare recovery backup: %s\n", error ? error->message : "unknown error");
                g_clear_error(&error);
                failed = TRUE;
                break;
            }
            items[i].backup_created = TRUE;
        }
    }

    if (!failed && staged > 0) {
        emit_progress(job->state, result->processed_count, "Applying replacements…", "Committing the verified output files");
        for (guint i = 0; i < job->file_count; i++) {
            if (!items[i].has_output) continue;
            if (g_rename(items[i].stage_file, items[i].source) != 0) {
                g_string_append_printf(log, "  Error     : Replacement failed for %s: %s\n",
                                       items[i].source, g_strerror(errno));
                failed = TRUE;
                break;
            }
            items[i].committed = TRUE;
        }
    }

    if (failed) {
        gboolean rollback_ok = TRUE;
        for (gint i = (gint)job->file_count - 1; i >= 0; i--) {
            if (items[i].committed && items[i].backup_file) {
                if (g_rename(items[i].backup_file, items[i].source) == 0) {
                    items[i].backup_created = FALSE;
                } else {
                    rollback_ok = FALSE;
                    g_string_append_printf(log, "  ALERT     : Original backup remains at %s\n", items[i].backup_file);
                }
            }
        }
        if (rollback_ok) g_string_append(log, "  Safety    : Rollback complete. No original files were modified.");
        result->success = FALSE;
        result->text = g_string_free(log, FALSE);
        cleanup_replace_items(items, job->file_count, !rollback_ok);
        return result;
    }

    for (guint i = 0; i < job->file_count; i++) {
        if (items[i].backup_file) g_unlink(items[i].backup_file);
    }
    g_string_append_printf(log, "──────────────────────────────────────\n"
                                "  Result    : %u replaced · %u unchanged",
                           staged, skipped);
    result->success = TRUE;
    result->text = g_string_free(log, FALSE);
    cleanup_replace_items(items, job->file_count, FALSE);
    return result;
}

static JobResult *run_standard_job(const Job *job) {
    JobResult *result = g_new0(JobResult, 1);
    result->processed_count = 0;
    GString *log = g_string_new("COMPRESSION SUMMARY\n══════════════════════════════════════\n");
    gboolean all_ok = TRUE;
    guint succeeded = 0;

    for (guint i = 0; i < job->file_count; i++) {
        gchar *basename = g_path_get_basename(job->files[i]);
        gchar *headline = job->file_count == 1
            ? g_strdup("Compressing image…")
            : g_strdup_printf("Compressing image %u of %u…", i + 1, job->file_count);
        emit_progress(job->state, i, headline, basename);
        g_free(headline);

        gchar **argv = build_cli_argv(job, job->files[i], NULL);
        gchar *output = NULL;
        GError *error = NULL;
        gboolean ok = run_process(argv, &output, &error);
        g_strfreev(argv);
        result->processed_count = i + 1;
        gchar *completed_headline = g_strdup_printf("Processed image %u of %u", i + 1, job->file_count);
        emit_progress(job->state, result->processed_count, completed_headline, basename);
        g_free(completed_headline);

        append_formatted_cli_block(log, i + 1, job->file_count, job->files[i], output);
        if (error) {
            g_string_append_printf(log, "  Error     : %s\n", error->message);
            g_clear_error(&error);
        }
        g_free(output);

        if (ok) succeeded++;
        else all_ok = FALSE;

        g_free(basename);
    }

    if (job->file_count > 1) {
        g_string_append_printf(log, "──────────────────────────────────────\n"
                                    "  Total     : %u of %u images compressed successfully",
                               succeeded, job->file_count);
    } else if (log->len > 0 && log->str[log->len - 1] == '\n') {
        g_string_truncate(log, log->len - 1);
    }

    result->success = all_ok;
    result->text = g_string_free(log, FALSE);
    return result;
}

static void compression_worker(GTask *task, gpointer source_object,
                               gpointer task_data, GCancellable *cancellable) {
    (void)source_object; (void)cancellable;
    Job *job = task_data;
    JobResult *result;
    if (job->replace_original) {
        result = run_replace_job(job);
    } else {
        result = run_standard_job(job);
    }
    g_task_return_pointer(task, result, job_result_free);
}

static void set_controls_sensitive(AppState *state, gboolean sensitive) {
    gtk_widget_set_sensitive(state->settings_box, sensitive);
    gtk_widget_set_sensitive(state->add_button, sensitive);
    gtk_widget_set_sensitive(state->clear_button, state->files->len > 0 && sensitive);
    gtk_widget_set_sensitive(state->run_button, sensitive && state->files->len > 0);
    gtk_widget_set_sensitive(state->file_list, sensitive);
}

static void on_activity_modal_close_clicked(GtkButton *button, gpointer user_data) {
    (void)button;
    AppState *state = user_data;
    if (state->busy) return;
    if (state->activity_modal)
        gtk_window_destroy(GTK_WINDOW(state->activity_modal));
}

static gboolean on_activity_modal_close_request(GtkWindow *window, gpointer user_data) {
    (void)window;
    AppState *state = user_data;
    if (state->busy) return TRUE;
    state->activity_modal = NULL;
    reset_app_state(state);
    return FALSE;
}

static void show_activity_modal(AppState *state, guint total_files) {
    GtkWidget *modal = gtk_window_new();
    state->activity_modal = modal;
    state->completed_files = 0;
    state->total_files = total_files;

    gtk_window_set_title(GTK_WINDOW(modal), "Activity Monitor");
    gtk_window_set_transient_for(GTK_WINDOW(modal), GTK_WINDOW(state->window));
    gtk_window_set_modal(GTK_WINDOW(modal), TRUE);
    gtk_window_set_resizable(GTK_WINDOW(modal), FALSE);
    /* Compact 500x330 modal height to eliminate blank bottom space */
    gtk_window_set_default_size(GTK_WINDOW(modal), 500, 330);
    gtk_widget_set_size_request(modal, 500, 330);
    g_signal_connect(modal, "close-request", G_CALLBACK(on_activity_modal_close_request), state);

    GtkWidget *header = gtk_header_bar_new();
    gtk_widget_add_css_class(header, "flat-header");
    gtk_header_bar_set_show_title_buttons(GTK_HEADER_BAR(header), FALSE);
    GtkWidget *title = gtk_label_new("Activity Monitor");
    gtk_widget_add_css_class(title, "section-title");
    gtk_header_bar_set_title_widget(GTK_HEADER_BAR(header), title);
    gtk_window_set_titlebar(GTK_WINDOW(modal), header);

    GtkWidget *box = gtk_box_new(GTK_ORIENTATION_VERTICAL, 12);
    gtk_widget_set_margin_start(box, 20);
    gtk_widget_set_margin_end(box, 20);
    gtk_widget_set_margin_top(box, 8);
    gtk_widget_set_margin_bottom(box, 14);
    gtk_window_set_child(GTK_WINDOW(modal), box);

    /* Top Telemetry Status Card */
    GtkWidget *status_card = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 14);
    gtk_widget_add_css_class(status_card, "monitor-card");
    gtk_widget_set_valign(status_card, GTK_ALIGN_CENTER);

    GtkWidget *indicator_box = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 10);
    gtk_widget_set_valign(indicator_box, GTK_ALIGN_CENTER);

    state->modal_spinner = gtk_spinner_new();
    gtk_widget_set_size_request(state->modal_spinner, 26, 26);
    gtk_spinner_start(GTK_SPINNER(state->modal_spinner));

    state->modal_status_badge = gtk_label_new("✓");
    gtk_widget_add_css_class(state->modal_status_badge, "status-badge-ok");
    gtk_widget_set_valign(state->modal_status_badge, GTK_ALIGN_CENTER);
    gtk_widget_set_visible(state->modal_status_badge, FALSE);

    gchar *initial_count = g_strdup_printf("0 / %u", total_files);
    state->modal_count_label = gtk_label_new(initial_count);
    g_free(initial_count);
    gtk_widget_add_css_class(state->modal_count_label, "progress-readout");

    gtk_box_append(GTK_BOX(indicator_box), state->modal_spinner);
    gtk_box_append(GTK_BOX(indicator_box), state->modal_status_badge);
    gtk_box_append(GTK_BOX(indicator_box), state->modal_count_label);

    GtkWidget *status_text_box = gtk_box_new(GTK_ORIENTATION_VERTICAL, 2);
    gtk_widget_set_hexpand(status_text_box, TRUE);
    gtk_widget_set_valign(status_text_box, GTK_ALIGN_CENTER);

    gchar *init_title = total_files == 1
        ? g_strdup("Compressing image…")
        : g_strdup_printf("Compressing %u images…", total_files);
    state->modal_title_label = gtk_label_new(init_title);
    g_free(init_title);
    gtk_label_set_xalign(GTK_LABEL(state->modal_title_label), 0.0f);
    gtk_widget_add_css_class(state->modal_title_label, "field-label");

    state->modal_sub_label = gtk_label_new("Optimizing locally…");
    gtk_label_set_xalign(GTK_LABEL(state->modal_sub_label), 0.0f);
    gtk_label_set_ellipsize(GTK_LABEL(state->modal_sub_label), PANGO_ELLIPSIZE_MIDDLE);
    gtk_widget_add_css_class(state->modal_sub_label, "dim-label");

    gtk_box_append(GTK_BOX(status_text_box), state->modal_title_label);
    gtk_box_append(GTK_BOX(status_text_box), state->modal_sub_label);

    gtk_box_append(GTK_BOX(status_card), indicator_box);
    gtk_box_append(GTK_BOX(status_card), status_text_box);
    gtk_box_append(GTK_BOX(box), status_card);

    /* Organized Summary View */
    GtkWidget *result_scroll = gtk_scrolled_window_new();
    gtk_widget_set_vexpand(result_scroll, TRUE);
    gtk_widget_add_css_class(result_scroll, "log-frame");

    state->modal_result_view = gtk_text_view_new();
    gtk_text_view_set_editable(GTK_TEXT_VIEW(state->modal_result_view), FALSE);
    gtk_text_view_set_cursor_visible(GTK_TEXT_VIEW(state->modal_result_view), FALSE);
    gtk_text_view_set_wrap_mode(GTK_TEXT_VIEW(state->modal_result_view), GTK_WRAP_WORD_CHAR);
    gtk_text_buffer_set_text(gtk_text_view_get_buffer(GTK_TEXT_VIEW(state->modal_result_view)),
                             "Running scompress engine…\n"
                             "Original files are protected while optimization is in progress.", -1);
    gtk_scrolled_window_set_child(GTK_SCROLLED_WINDOW(result_scroll), state->modal_result_view);
    gtk_box_append(GTK_BOX(box), result_scroll);

    /* Modal Action Bar */
    GtkWidget *action_box = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 10);
    GtkWidget *hint = gtk_label_new("Closing this monitor will reset the workspace.");
    gtk_widget_add_css_class(hint, "dim-label");
    gtk_widget_set_hexpand(hint, TRUE);
    gtk_label_set_xalign(GTK_LABEL(hint), 0.0f);

    state->modal_close_btn = gtk_button_new_with_label("Done");
    gtk_widget_add_css_class(state->modal_close_btn, "suggested-action");
    gtk_widget_set_sensitive(state->modal_close_btn, FALSE);
    g_signal_connect(state->modal_close_btn, "clicked", G_CALLBACK(on_activity_modal_close_clicked), state);

    gtk_box_append(GTK_BOX(action_box), hint);
    gtk_box_append(GTK_BOX(action_box), state->modal_close_btn);
    gtk_box_append(GTK_BOX(box), action_box);

    gtk_window_present(GTK_WINDOW(modal));
}

static void compression_complete(GObject *source_object, GAsyncResult *async_result, gpointer user_data) {
    (void)source_object;
    AppState *state = user_data;
    GError *error = NULL;
    JobResult *result = g_task_propagate_pointer(G_TASK(async_result), &error);
    state->busy = FALSE;
    set_controls_sensitive(state, TRUE);

    if (state->activity_modal) {
        gtk_spinner_stop(GTK_SPINNER(state->modal_spinner));
        gtk_widget_set_visible(state->modal_spinner, FALSE);
        gtk_widget_set_visible(state->modal_status_badge, TRUE);
        if (result)
            state->completed_files = MIN(result->processed_count, state->total_files);
        if (!error && result && result->success)
            state->completed_files = state->total_files;
        update_progress_count(state);
        gtk_widget_set_sensitive(state->modal_close_btn, TRUE);
        gtk_header_bar_set_show_title_buttons(
            GTK_HEADER_BAR(gtk_window_get_titlebar(GTK_WINDOW(state->activity_modal))), TRUE);

        if (error) {
            gtk_label_set_text(GTK_LABEL(state->modal_status_badge), "!");
            gtk_widget_remove_css_class(state->modal_status_badge, "status-badge-ok");
            gtk_widget_add_css_class(state->modal_status_badge, "status-badge-warn");
            gtk_label_set_text(GTK_LABEL(state->modal_title_label), "Compression failed");
            gtk_label_set_text(GTK_LABEL(state->modal_sub_label), "Review the summary below for details.");
            gtk_text_buffer_set_text(gtk_text_view_get_buffer(GTK_TEXT_VIEW(state->modal_result_view)),
                                     error->message, -1);
            g_clear_error(&error);
        } else if (result) {
            if (!result->success) {
                gtk_label_set_text(GTK_LABEL(state->modal_status_badge), "!");
                gtk_widget_remove_css_class(state->modal_status_badge, "status-badge-ok");
                gtk_widget_add_css_class(state->modal_status_badge, "status-badge-warn");
            }
            gtk_label_set_text(GTK_LABEL(state->modal_title_label),
                               result->success ? "Compression Complete" : "Completed with Warnings");
            gchar *sub = result->processed_count == 1
                ? g_strdup("One image processing attempt completed.")
                : g_strdup_printf("%u image processing attempts completed.", result->processed_count);
            gtk_label_set_text(GTK_LABEL(state->modal_sub_label), sub);
            g_free(sub);

            gtk_text_buffer_set_text(gtk_text_view_get_buffer(GTK_TEXT_VIEW(state->modal_result_view)),
                                     result->text ? result->text : "No output was returned.", -1);
            job_result_free(result);
        }
        gtk_widget_grab_focus(state->modal_close_btn);
    } else if (result) {
        job_result_free(result);
    }
}

static gboolean parse_positive_entry(GtkWidget *entry, gint *value) {
    const gchar *text = gtk_editable_get_text(GTK_EDITABLE(entry));
    gchar *end = NULL;
    errno = 0;
    gint64 parsed = g_ascii_strtoll(text, &end, 10);
    if (errno || end == text || *end != '\0' || parsed < 1 || parsed > G_MAXINT)
        return FALSE;
    *value = (gint)parsed;
    return TRUE;
}

static void launch_job(AppState *state, Job *job) {
    state->busy = TRUE;
    set_controls_sensitive(state, FALSE);
    set_status(state, NULL);
    show_activity_modal(state, job->file_count);

    GTask *task = g_task_new(NULL, NULL, compression_complete, state);
    g_task_set_task_data(task, job, job_free);
    g_task_run_in_thread(task, compression_worker);
    g_object_unref(task);
}

typedef struct {
    AppState *state;
    Job *job;
} ConfirmContext;

static void replace_confirmation_done(GObject *source_object, GAsyncResult *result, gpointer user_data) {
    ConfirmContext *context = user_data;
    GError *error = NULL;
    gint response = gtk_alert_dialog_choose_finish(GTK_ALERT_DIALOG(source_object), result, &error);
    if (!error && response == 1) launch_job(context->state, context->job);
    else job_free(context->job);
    g_clear_error(&error);
    g_object_unref(source_object);
    g_free(context);
}

static void on_start_compression(GtkButton *button, gpointer user_data) {
    (void)button;
    AppState *state = user_data;
    if (state->busy || state->files->len == 0) return;

    Job *job = g_new0(Job, 1);
    job->state = state;
    job->file_count = state->files->len;
    job->files = g_new0(gchar *, job->file_count + 1);
    for (guint i = 0; i < job->file_count; i++)
        job->files[i] = g_strdup(g_ptr_array_index(state->files, i));
    job->cli_path = g_strdup(state->cli_path);
    job->quality = (gint)gtk_range_get_value(GTK_RANGE(state->quality_scale));
    job->format_index = (gint)gtk_drop_down_get_selected(GTK_DROP_DOWN(state->format_dropdown));
    job->metadata_index = (gint)gtk_drop_down_get_selected(GTK_DROP_DOWN(state->metadata_dropdown));

    guint dim_idx = gtk_drop_down_get_selected(GTK_DROP_DOWN(state->dimension_dropdown));
    if (dim_idx == 0) {
        job->max_width = 0;
        job->max_height = 0;
        job->no_resize = FALSE;
    } else {
        job->max_width = (gint)gtk_spin_button_get_value_as_int(GTK_SPIN_BUTTON(state->width_spin));
        job->max_height = (gint)gtk_spin_button_get_value_as_int(GTK_SPIN_BUTTON(state->height_spin));
        job->no_resize = FALSE;
    }

    job->first_frame = gtk_check_button_get_active(GTK_CHECK_BUTTON(state->first_frame_check));
    job->replace_original = gtk_check_button_get_active(GTK_CHECK_BUTTON(state->replace_check));
    job->output_dir = g_strdup(gtk_editable_get_text(GTK_EDITABLE(state->output_entry)));

    guint preset_idx = gtk_drop_down_get_selected(GTK_DROP_DOWN(state->preset_dropdown));
    if (preset_idx < 3) {
        job->target_kb = preset_values[preset_idx];
    } else {
        if (!parse_positive_entry(state->target_entry, &job->target_kb)) {
            set_status(state, "Enter a valid custom target size in KB (for example, 350).");
            gtk_widget_grab_focus(state->target_entry);
            job_free(job);
            return;
        }
    }

    if (job->replace_original && job->format_index != 0) {
        set_status(state, "Keep Default format when replacing original files.");
        job_free(job);
        return;
    }
    if (job->replace_original && job->output_dir && *job->output_dir) {
        set_status(state, "Clear the custom output folder in Advanced Options when replacing original files.");
        job_free(job);
        return;
    }
    if (!job->replace_original && job->output_dir && *job->output_dir && !g_file_test(job->output_dir, G_FILE_TEST_IS_DIR)) {
        set_status(state, "The selected output folder does not exist or is not a directory.");
        job_free(job);
        return;
    }
    if (job->first_frame && job->format_index == 0) {
        set_status(state, "Choose a still-image output format (WebP, JPEG, PNG) when using the first GIF frame option.");
        job_free(job);
        return;
    }
    if (job->first_frame && job->format_index == 4) {
        set_status(state, "First-frame mode cannot be combined with GIF output.");
        job_free(job);
        return;
    }
    if (job->metadata_index == 1) {
        if (job->format_index != 0) {
            set_status(state, "Keep metadata is available only when keeping the Default format.");
            job_free(job);
            return;
        }
        for (guint i = 0; i < job->file_count; i++) {
            const gchar *ext = strrchr(job->files[i], '.');
            gchar *lower = ext ? g_ascii_strdown(ext + 1, -1) : g_strdup("");
            gboolean supported = g_str_equal(lower, "jpg") || g_str_equal(lower, "jpeg") || g_str_equal(lower, "png");
            g_free(lower);
            if (!supported) {
                set_status(state, "Keep metadata is supported for JPEG and PNG only. Switch metadata policy in Advanced Options.");
                job_free(job);
                return;
            }
        }
    }

    if (job->replace_original) {
        GtkAlertDialog *dialog = gtk_alert_dialog_new("Replace the selected original files?");
        const gchar *buttons[] = { "Cancel", "Replace Originals", NULL };
        gtk_alert_dialog_set_detail(dialog,
            "scompress will compress temporary copies first, verify that the source files have not changed, and create recovery links before replacement. This operation overwrites the selected files in place.");
        gtk_alert_dialog_set_buttons(dialog, buttons);
        gtk_alert_dialog_set_cancel_button(dialog, 0);
        gtk_alert_dialog_set_default_button(dialog, 0);
        ConfirmContext *context = g_new0(ConfirmContext, 1);
        context->state = state;
        context->job = job;
        gtk_alert_dialog_choose(dialog, GTK_WINDOW(state->window), NULL,
                                replace_confirmation_done, context);
        return;
    }
    launch_job(state, job);
}

static GtkWidget *make_card(const gchar *title, const gchar *subtitle) {
    GtkWidget *frame = gtk_box_new(GTK_ORIENTATION_VERTICAL, 10);
    gtk_widget_add_css_class(frame, "card");
    if (title || subtitle) {
        GtkWidget *header_box = gtk_box_new(GTK_ORIENTATION_VERTICAL, 2);
        if (title) {
            GtkWidget *heading = gtk_label_new(title);
            gtk_label_set_xalign(GTK_LABEL(heading), 0.0f);
            gtk_widget_add_css_class(heading, "section-title");
            gtk_box_append(GTK_BOX(header_box), heading);
        }
        if (subtitle) {
            GtkWidget *description = gtk_label_new(subtitle);
            gtk_label_set_xalign(GTK_LABEL(description), 0.0f);
            gtk_label_set_wrap(GTK_LABEL(description), TRUE);
            gtk_widget_add_css_class(description, "dim-label");
            gtk_box_append(GTK_BOX(header_box), description);
        }
        gtk_box_append(GTK_BOX(frame), header_box);
    }
    return frame;
}

static GtkWidget *field_label(const gchar *text) {
    GtkWidget *label = gtk_label_new(text);
    gtk_label_set_xalign(GTK_LABEL(label), 0.0f);
    gtk_widget_set_valign(label, GTK_ALIGN_CENTER);
    gtk_widget_add_css_class(label, "field-label");
    return label;
}

static GtkWidget *make_grid(void) {
    GtkWidget *grid = gtk_grid_new();
    gtk_grid_set_row_spacing(GTK_GRID(grid), 10);
    gtk_grid_set_column_spacing(GTK_GRID(grid), 16);
    gtk_widget_set_hexpand(grid, TRUE);
    return grid;
}

static gboolean on_window_close(GtkWindow *window, gpointer user_data) {
    (void)window;
    AppState *state = user_data;
    if (state->busy) {
        return TRUE;
    }
    return FALSE;
}

static void activate(GtkApplication *application, gpointer user_data) {
    AppState *state = user_data;
    if (state->window) {
        gtk_window_present(GTK_WINDOW(state->window));
        return;
    }

    GtkCssProvider *css = gtk_css_provider_new();
    const gchar *style =
        "window { background: @theme_bg_color; color: @theme_fg_color; }"
        "headerbar.flat-header { background: @theme_bg_color; border-bottom: none; box-shadow: none; padding: 6px 14px 0px 14px; }"
        ".app-heading { font-size: 20px; font-weight: 800; letter-spacing: -0.4px; color: @theme_fg_color; }"
        ".card { background: alpha(@theme_fg_color, 0.035); border: 1px solid alpha(@theme_fg_color, 0.09); border-radius: 14px; padding: 13px 16px; }"
        ".monitor-card { background: alpha(#6366f1, 0.08); border: 1px solid alpha(#6366f1, 0.22); border-radius: 12px; padding: 12px 16px; }"
        ".progress-readout { font-size: 18px; font-weight: 800; color: #6366f1; min-width: 58px; font-variant-numeric: tabular-nums; }"
        ".status-badge-ok { background: alpha(#10b981, 0.18); color: #10b981; border-radius: 99px; min-width: 26px; min-height: 26px; font-weight: 800; font-size: 14px; }"
        ".status-badge-warn { background: alpha(#f59e0b, 0.20); color: #fbbf24; border-radius: 99px; min-width: 26px; min-height: 26px; font-weight: 800; font-size: 14px; }"
        ".section-title { font-size: 13.5px; font-weight: 700; letter-spacing: -0.1px; }"
        ".field-label { font-size: 13px; font-weight: 600; color: alpha(@theme_fg_color, 0.88); }"
        ".dim-label { color: @insensitive_fg_color; font-size: 11.5px; }"
        ".toast-banner { background: alpha(#f59e0b, 0.12); color: #fbbf24; border: 1px solid alpha(#f59e0b, 0.28); border-radius: 9px; padding: 6px 12px; font-size: 12px; font-weight: 600; }"
        ".list-scroll { border: 1.5px dashed alpha(@theme_fg_color, 0.16); border-radius: 11px; background: alpha(@theme_bg_color, 0.55); transition: all 150ms ease; }"
        ".list-scroll.drop-active { border-color: #6366f1; background: alpha(#6366f1, 0.08); }"
        "listbox { background: transparent; }"
        ".empty-state { padding: 18px 12px; }"
        ".empty-icon { opacity: 0.45; }"
        ".empty-title { font-size: 13px; font-weight: 600; color: alpha(@theme_fg_color, 0.75); }"
        ".file-row { padding: 6px 10px; border-bottom: 1px solid alpha(@theme_fg_color, 0.06); }"
        ".file-icon { opacity: 0.7; }"
        ".file-name { font-weight: 600; font-size: 12.5px; }"
        ".size-badge { background: alpha(#6366f1, 0.12); color: #6366f1; border-radius: 99px; padding: 2px 9px; font-size: 11px; font-weight: 700; }"
        ".remove-btn { min-height: 22px; min-width: 22px; padding: 2px; border-radius: 99px; opacity: 0.6; }"
        ".remove-btn:hover { opacity: 1.0; background: alpha(#ef4444, 0.15); color: #ef4444; }"
        ".quality-pill { background: alpha(#6366f1, 0.14); color: #6366f1; border-radius: 8px; padding: 2px 8px; font-weight: 700; font-size: 12px; min-width: 44px; }"
        ".replace-row { background: alpha(@theme_fg_color, 0.03); border: 1px solid alpha(@theme_fg_color, 0.09); border-radius: 10px; padding: 8px 12px; margin-top: 2px; }"
        ".caution-icon { color: alpha(@theme_fg_color, 0.65); transition: opacity 150ms ease; }"
        ".caution-icon:hover { color: #6366f1; }"
        "expander > title { font-weight: 700; font-size: 13px; padding: 2px 0; }"
        ".log-frame { border-radius: 10px; border: 1px solid alpha(@theme_fg_color, 0.12); background: #0f172a; }"
        "textview { font-family: monospace; font-size: 11.5px; background: #0f172a; color: #e2e8f0; padding: 10px; }"
        "textview text { background: #0f172a; color: #e2e8f0; }"
        ".footer-bar { background: @theme_bg_color; border-top: none; box-shadow: none; }"
        ".run-btn { border-radius: 10px; padding: 9px 26px; font-weight: 700; font-size: 13.5px; }"
        "button { border-radius: 9px; padding: 6px 12px; font-weight: 600; }"
        "entry, spinbutton, dropdown { border-radius: 8px; min-height: 32px; }";
#if GTK_CHECK_VERSION(4, 12, 0)
    gtk_css_provider_load_from_string(css, style);
#else
    gtk_css_provider_load_from_data(css, style, -1);
#endif
    gtk_style_context_add_provider_for_display(gdk_display_get_default(),
        GTK_STYLE_PROVIDER(css), GTK_STYLE_PROVIDER_PRIORITY_APPLICATION);
    g_object_unref(css);

    state->window = gtk_application_window_new(application);
    gtk_window_set_title(GTK_WINDOW(state->window), "Compress with scompress!");
    /* Exact 640x628 geometry: fits Advanced Options without scrolling and removes bottom blank gap */
    gtk_window_set_default_size(GTK_WINDOW(state->window), 640, 628);
    gtk_widget_set_size_request(state->window, 640, 628);
    gtk_window_set_resizable(GTK_WINDOW(state->window), FALSE);
    g_signal_connect(state->window, "close-request", G_CALLBACK(on_window_close), state);

    /* Minimalist Top Header */
    GtkWidget *header = gtk_header_bar_new();
    gtk_widget_add_css_class(header, "flat-header");
    gtk_header_bar_set_show_title_buttons(GTK_HEADER_BAR(header), TRUE);
    GtkWidget *top_title = gtk_label_new("Compress with scompress!");
    gtk_widget_add_css_class(top_title, "app-heading");
    gtk_widget_set_halign(top_title, GTK_ALIGN_START);
    gtk_header_bar_set_title_widget(GTK_HEADER_BAR(header), top_title);
    gtk_window_set_titlebar(GTK_WINDOW(state->window), header);

    GtkWidget *root = gtk_box_new(GTK_ORIENTATION_VERTICAL, 0);
    gtk_window_set_child(GTK_WINDOW(state->window), root);

    GtkWidget *scroller = gtk_scrolled_window_new();
    gtk_widget_set_vexpand(scroller, TRUE);
    gtk_scrolled_window_set_policy(GTK_SCROLLED_WINDOW(scroller), GTK_POLICY_NEVER, GTK_POLICY_AUTOMATIC);
    gtk_box_append(GTK_BOX(root), scroller);

    GtkWidget *content = gtk_box_new(GTK_ORIENTATION_VERTICAL, 10);
    gtk_widget_set_margin_start(content, 18);
    gtk_widget_set_margin_end(content, 18);
    gtk_widget_set_margin_top(content, 8);
    gtk_widget_set_margin_bottom(content, 4);
    gtk_scrolled_window_set_child(GTK_SCROLLED_WINDOW(scroller), content);

    /* Inline validation banner (hidden unless needed) */
    state->toast_label = gtk_label_new("");
    gtk_label_set_xalign(GTK_LABEL(state->toast_label), 0.0f);
    gtk_label_set_wrap(GTK_LABEL(state->toast_label), TRUE);
    gtk_widget_add_css_class(state->toast_label, "toast-banner");
    gtk_widget_set_visible(state->toast_label, FALSE);
    gtk_box_append(GTK_BOX(content), state->toast_label);

    /* 1. Selected Images & Drag-and-Drop Card */
    GtkWidget *files_card = make_card(NULL, NULL);
    GtkWidget *files_header = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 8);
    GtkWidget *files_title_box = gtk_box_new(GTK_ORIENTATION_VERTICAL, 1);
    gtk_widget_set_hexpand(files_title_box, TRUE);
    GtkWidget *files_heading = gtk_label_new("Image Queue");
    gtk_label_set_xalign(GTK_LABEL(files_heading), 0.0f);
    gtk_widget_add_css_class(files_heading, "section-title");
    state->count_label = gtk_label_new("No images selected");
    gtk_widget_add_css_class(state->count_label, "dim-label");
    gtk_label_set_xalign(GTK_LABEL(state->count_label), 0.0f);
    gtk_box_append(GTK_BOX(files_title_box), files_heading);
    gtk_box_append(GTK_BOX(files_title_box), state->count_label);

    state->clear_button = gtk_button_new_with_label("Clear");
    gtk_widget_set_valign(state->clear_button, GTK_ALIGN_CENTER);
    g_signal_connect(state->clear_button, "clicked", G_CALLBACK(on_clear_files), state);

    state->add_button = gtk_button_new_with_label("＋ Add Images");
    gtk_widget_set_valign(state->add_button, GTK_ALIGN_CENTER);
    g_signal_connect(state->add_button, "clicked", G_CALLBACK(on_add_files), state);

    gtk_box_append(GTK_BOX(files_header), files_title_box);
    gtk_box_append(GTK_BOX(files_header), state->clear_button);
    gtk_box_append(GTK_BOX(files_header), state->add_button);
    gtk_box_append(GTK_BOX(files_card), files_header);

    state->list_scroller = gtk_scrolled_window_new();
    gtk_scrolled_window_set_policy(GTK_SCROLLED_WINDOW(state->list_scroller), GTK_POLICY_NEVER, GTK_POLICY_AUTOMATIC);
    gtk_widget_set_size_request(state->list_scroller, -1, 114);
    gtk_widget_add_css_class(state->list_scroller, "list-scroll");

    state->file_list = gtk_list_box_new();
    gtk_list_box_set_selection_mode(GTK_LIST_BOX(state->file_list), GTK_SELECTION_NONE);
    gtk_scrolled_window_set_child(GTK_SCROLLED_WINDOW(state->list_scroller), state->file_list);
    gtk_box_append(GTK_BOX(files_card), state->list_scroller);
    gtk_box_append(GTK_BOX(content), files_card);

    /* Attach Drag-and-Drop Controller */
    GtkDropTarget *drop_target = gtk_drop_target_new(G_TYPE_INVALID, GDK_ACTION_COPY);
#if GTK_CHECK_VERSION(4, 6, 0)
    GType drop_types[] = { GDK_TYPE_FILE_LIST, G_TYPE_FILE, G_TYPE_STRING };
    gtk_drop_target_set_gtypes(drop_target, drop_types, G_N_ELEMENTS(drop_types));
#else
    GType drop_types[] = { G_TYPE_FILE, G_TYPE_STRING };
    gtk_drop_target_set_gtypes(drop_target, drop_types, G_N_ELEMENTS(drop_types));
#endif
    g_signal_connect(drop_target, "enter", G_CALLBACK(on_drop_enter), state);
    g_signal_connect(drop_target, "leave", G_CALLBACK(on_drop_leave), state);
    g_signal_connect(drop_target, "drop", G_CALLBACK(on_drop_files), state);
    gtk_widget_add_controller(state->window, GTK_EVENT_CONTROLLER(drop_target));

    /* 2. Primary Settings Box ("Compression & Resize" only — no subtitle) */
    state->settings_box = gtk_box_new(GTK_ORIENTATION_VERTICAL, 10);
    GtkWidget *primary_card = make_card("Compression & Resize", NULL);
    GtkWidget *grid = make_grid();

    /* Row 0: Target Size */
    state->preset_dropdown = gtk_drop_down_new_from_strings(preset_labels);
    gtk_drop_down_set_selected(GTK_DROP_DOWN(state->preset_dropdown), 2); /* 500 KB */
    gtk_widget_set_hexpand(state->preset_dropdown, TRUE);

    state->target_custom_box = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 6);
    state->target_entry = gtk_entry_new();
    gtk_editable_set_text(GTK_EDITABLE(state->target_entry), "500");
    gtk_entry_set_placeholder_text(GTK_ENTRY(state->target_entry), "Size");
    gtk_entry_set_input_purpose(GTK_ENTRY(state->target_entry), GTK_INPUT_PURPOSE_NUMBER);
    gtk_editable_set_width_chars(GTK_EDITABLE(state->target_entry), 7);
    GtkWidget *kb_lbl = gtk_label_new("KB");
    gtk_widget_add_css_class(kb_lbl, "dim-label");
    gtk_box_append(GTK_BOX(state->target_custom_box), state->target_entry);
    gtk_box_append(GTK_BOX(state->target_custom_box), kb_lbl);
    gtk_widget_set_visible(state->target_custom_box, FALSE);

    GtkWidget *target_row = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 8);
    gtk_widget_set_hexpand(target_row, TRUE);
    gtk_box_append(GTK_BOX(target_row), state->preset_dropdown);
    gtk_box_append(GTK_BOX(target_row), state->target_custom_box);

    gtk_grid_attach(GTK_GRID(grid), field_label("Target size"), 0, 0, 1, 1);
    gtk_grid_attach(GTK_GRID(grid), target_row, 1, 0, 1, 1);
    g_signal_connect(state->preset_dropdown, "notify::selected", G_CALLBACK(on_preset_changed), state);

    /* Row 1 & 2: Dimension ("Default" / "Custom") + Equal-width Custom Spinbuttons Below */
    state->dimension_dropdown = gtk_drop_down_new_from_strings(dimension_labels);
    gtk_drop_down_set_selected(GTK_DROP_DOWN(state->dimension_dropdown), 0); /* Default */
    gtk_widget_set_hexpand(state->dimension_dropdown, TRUE);

    gtk_grid_attach(GTK_GRID(grid), field_label("Dimension"), 0, 1, 1, 1);
    gtk_grid_attach(GTK_GRID(grid), state->dimension_dropdown, 1, 1, 1, 1);
    g_signal_connect(state->dimension_dropdown, "notify::selected", G_CALLBACK(on_dimension_changed), state);

    /* Custom Width & Height Row */
    state->dimension_custom_box = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 10);
    gtk_box_set_homogeneous(GTK_BOX(state->dimension_custom_box), TRUE);
    gtk_widget_set_hexpand(state->dimension_custom_box, TRUE);

    GtkWidget *w_group = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 6);
    GtkWidget *w_lbl = gtk_label_new("Width");
    gtk_widget_add_css_class(w_lbl, "dim-label");
    state->width_spin = gtk_spin_button_new_with_range(0, 30000, 10);
    gtk_spin_button_set_numeric(GTK_SPIN_BUTTON(state->width_spin), TRUE);
    gtk_widget_set_hexpand(state->width_spin, TRUE);
    gtk_box_append(GTK_BOX(w_group), w_lbl);
    gtk_box_append(GTK_BOX(w_group), state->width_spin);

    GtkWidget *h_group = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 6);
    GtkWidget *h_lbl = gtk_label_new("Height");
    gtk_widget_add_css_class(h_lbl, "dim-label");
    state->height_spin = gtk_spin_button_new_with_range(0, 30000, 10);
    gtk_spin_button_set_numeric(GTK_SPIN_BUTTON(state->height_spin), TRUE);
    gtk_widget_set_hexpand(state->height_spin, TRUE);
    gtk_box_append(GTK_BOX(h_group), h_lbl);
    gtk_box_append(GTK_BOX(h_group), state->height_spin);

    gtk_box_append(GTK_BOX(state->dimension_custom_box), w_group);
    gtk_box_append(GTK_BOX(state->dimension_custom_box), h_group);
    gtk_widget_set_visible(state->dimension_custom_box, FALSE);
    gtk_grid_attach(GTK_GRID(grid), state->dimension_custom_box, 1, 2, 1, 1);

    /* Row 3: Output Format */
    state->format_dropdown = gtk_drop_down_new_from_strings(format_labels);
    gtk_drop_down_set_selected(GTK_DROP_DOWN(state->format_dropdown), 0);
    gtk_widget_set_hexpand(state->format_dropdown, TRUE);
    gtk_grid_attach(GTK_GRID(grid), field_label("Output format"), 0, 3, 1, 1);
    gtk_grid_attach(GTK_GRID(grid), state->format_dropdown, 1, 3, 1, 1);

    gtk_box_append(GTK_BOX(primary_card), grid);

    /* Neutral Replace Original Row + Caution Icon with Tooltip */
    GtkWidget *replace_row = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 6);
    gtk_widget_add_css_class(replace_row, "replace-row");
    gtk_widget_set_valign(replace_row, GTK_ALIGN_CENTER);

    state->replace_check = gtk_check_button_new_with_label("Replace original file(s) in place");
    g_signal_connect(state->replace_check, "toggled", G_CALLBACK(on_replace_toggled), state);

    GtkWidget *caution_icon = gtk_image_new_from_icon_name("dialog-warning-symbolic");
    gtk_image_set_pixel_size(GTK_IMAGE(caution_icon), 16);
    gtk_widget_add_css_class(caution_icon, "caution-icon");
    gtk_widget_set_valign(caution_icon, GTK_ALIGN_CENTER);
    gtk_widget_set_tooltip_text(caution_icon, "Selected originals are replaced only after every output is verified.");

    gtk_box_append(GTK_BOX(replace_row), state->replace_check);
    gtk_box_append(GTK_BOX(replace_row), caution_icon);
    gtk_box_append(GTK_BOX(primary_card), replace_row);

    gtk_box_append(GTK_BOX(state->settings_box), primary_card);

    /* 3. Collapsible Advanced Options Card */
    GtkWidget *adv_card = make_card(NULL, NULL);
    state->advanced_expander = gtk_expander_new("Advanced Options");
    gtk_expander_set_expanded(GTK_EXPANDER(state->advanced_expander), FALSE);

    GtkWidget *adv_box = gtk_box_new(GTK_ORIENTATION_VERTICAL, 10);
    gtk_widget_set_margin_top(adv_box, 10);
    GtkWidget *adv_grid = make_grid();

    /* Advanced Row 0: Preferred Quality */
    state->quality_scale = gtk_scale_new_with_range(GTK_ORIENTATION_HORIZONTAL, 20, 95, 1);
    gtk_range_set_value(GTK_RANGE(state->quality_scale), 95);
    gtk_scale_set_draw_value(GTK_SCALE(state->quality_scale), FALSE);
    gtk_widget_set_hexpand(state->quality_scale, TRUE);
    state->quality_label = gtk_label_new("95%");
    gtk_widget_add_css_class(state->quality_label, "quality-pill");
    gtk_widget_set_valign(state->quality_label, GTK_ALIGN_CENTER);
    GtkWidget *quality_box = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 10);
    gtk_box_append(GTK_BOX(quality_box), state->quality_scale);
    gtk_box_append(GTK_BOX(quality_box), state->quality_label);
    gtk_grid_attach(GTK_GRID(adv_grid), field_label("Preferred quality"), 0, 0, 1, 1);
    gtk_grid_attach(GTK_GRID(adv_grid), quality_box, 1, 0, 1, 1);
    g_signal_connect(state->quality_scale, "value-changed", G_CALLBACK(on_quality_changed), state);

    /* Advanced Row 1: Metadata Policy */
    state->metadata_dropdown = gtk_drop_down_new_from_strings(metadata_labels);
    gtk_drop_down_set_selected(GTK_DROP_DOWN(state->metadata_dropdown), 0);
    gtk_widget_set_hexpand(state->metadata_dropdown, TRUE);
    gtk_grid_attach(GTK_GRID(adv_grid), field_label("Metadata policy"), 0, 1, 1, 1);
    gtk_grid_attach(GTK_GRID(adv_grid), state->metadata_dropdown, 1, 1, 1, 1);

    /* Advanced Row 2: Output Directory */
    GtkWidget *output_row = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 8);
    state->output_entry = gtk_entry_new();
    gtk_widget_set_hexpand(state->output_entry, TRUE);
    gtk_entry_set_placeholder_text(GTK_ENTRY(state->output_entry), "Same folder as original images");
    state->output_browse_button = gtk_button_new_with_label("Browse…");
    g_signal_connect(state->output_browse_button, "clicked", G_CALLBACK(on_choose_output), state);
    gtk_box_append(GTK_BOX(output_row), state->output_entry);
    gtk_box_append(GTK_BOX(output_row), state->output_browse_button);
    gtk_grid_attach(GTK_GRID(adv_grid), field_label("Output folder"), 0, 2, 1, 1);
    gtk_grid_attach(GTK_GRID(adv_grid), output_row, 1, 2, 1, 1);

    gtk_box_append(GTK_BOX(adv_box), adv_grid);

    state->first_frame_check = gtk_check_button_new_with_label("Extract first frame when converting animated GIFs to still formats");
    gtk_box_append(GTK_BOX(adv_box), state->first_frame_check);

    gtk_expander_set_child(GTK_EXPANDER(state->advanced_expander), adv_box);
    gtk_box_append(GTK_BOX(adv_card), state->advanced_expander);
    gtk_box_append(GTK_BOX(state->settings_box), adv_card);

    gtk_box_append(GTK_BOX(content), state->settings_box);

    /* Borderless Action Footer: "scompress (sub) by Shoaib (version)" + "Compress" */
    GtkWidget *footer = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 12);
    gtk_widget_add_css_class(footer, "footer-bar");
    gtk_widget_set_margin_start(footer, 20);
    gtk_widget_set_margin_end(footer, 20);
    gtk_widget_set_margin_top(footer, 4);
    gtk_widget_set_margin_bottom(footer, 12);

    GtkWidget *brand_label = gtk_label_new(NULL);
    gchar *brand_markup = g_strdup_printf(
        "<span weight='bold' foreground='#6366f1' size='10500'>scompress</span> "
        "<span size='8200' rise='-1500' alpha='75%%'>by Shoaib</span> "
        "<span size='8500' alpha='55%%'>(v%s)</span>",
        APP_VERSION);
    gtk_label_set_markup(GTK_LABEL(brand_label), brand_markup);
    g_free(brand_markup);
    gtk_widget_set_hexpand(brand_label, TRUE);
    gtk_label_set_xalign(GTK_LABEL(brand_label), 0.0f);
    gtk_widget_set_valign(brand_label, GTK_ALIGN_CENTER);

    state->run_button = gtk_button_new_with_label("Compress");
    gtk_widget_add_css_class(state->run_button, "suggested-action");
    gtk_widget_add_css_class(state->run_button, "run-btn");
    g_signal_connect(state->run_button, "clicked", G_CALLBACK(on_start_compression), state);

    gtk_box_append(GTK_BOX(footer), brand_label);
    gtk_box_append(GTK_BOX(footer), state->run_button);
    gtk_box_append(GTK_BOX(root), footer);

    refresh_file_list(state);
    gtk_window_present(GTK_WINDOW(state->window));
}

int main(int argc, char **argv) {
    AppState state = {0};
    state.files = g_ptr_array_new_with_free_func(g_free);
    state.cli_path = find_cli_binary();
    for (int i = 1; i < argc; i++) add_file(&state, argv[i]);

    if (!state.cli_path) {
        g_printerr("scompress-gui: the scompress CLI executable was not found. Install the package first.\n");
        g_ptr_array_unref(state.files);
        return 127;
    }

    state.application = gtk_application_new(APP_ID, G_APPLICATION_DEFAULT_FLAGS);
    g_signal_connect(state.application, "activate", G_CALLBACK(activate), &state);
    int gtk_argc = 1;
    char *gtk_argv[] = { argv[0], NULL };
    int status = g_application_run(G_APPLICATION(state.application), gtk_argc, gtk_argv);

    g_object_unref(state.application);
    g_ptr_array_unref(state.files);
    g_free(state.cli_path);
    return status;
}
