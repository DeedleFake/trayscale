#include <adwaita.h>

#include "ui.h"
#include "app.h"
#include "main_window.h"

G_DEFINE_TYPE(UiApp, ui_app, ADW_TYPE_APPLICATION);

static GIcon *notification_icon = NULL;

static guint signal_update_id;
static gboolean g_settings_schema_found = FALSE;

UiApp *ui_app_new(TsApp ts_app) {
	UiApp *ui_app;

	ui_app = g_object_new(UI_TYPE_APP,
			"application-id", APP_ID,
			"flags", G_APPLICATION_HANDLES_OPEN,
			NULL);
	ui_app->ts_app = ts_app;

	return ui_app;
}

void ui_app_run(UiApp *ui_app, int argc, char *argv[]) {
	g_application_run(G_APPLICATION(ui_app), argc, argv);
}

void ui_app_quit(UiApp *ui_app) {
	g_application_quit(G_APPLICATION(ui_app));
}

void ui_app_notify(UiApp *ui_app, const char *title, const char *body) {
	GNotification *notification;
	GError *err = NULL;

	if (notification_icon == NULL) {
		notification_icon = g_icon_new_for_string(APP_ID, &err);
	}

	notification = g_notification_new(title);
	g_notification_set_body(notification, body);
	if (notification_icon != NULL) {
		g_notification_set_icon(notification, notification_icon);
	}

	g_application_send_notification(G_APPLICATION(ui_app), "tailscale-status", notification);

	g_clear_object(&notification);
}

void ui_app_update(UiApp *ui_app, TsutilStatus tsutil_status) {
	g_signal_emit(ui_app, signal_update_id, 0, tsutil_status);
}

void ui_app_g_settings_changed(GSettings *g_settings, const char *key, UiApp *ui_app) {
	if (strcmp(key, "tray-icon") == 0) {
		gboolean trayIcon = g_settings_get_boolean(g_settings, key);
		if (trayIcon) {
			ui_app_start_tray(ui_app);
			return;
		}
		ui_app_stop_tray(ui_app);
		return;
	}

	if (strcmp(key, "polling-interval") == 0) {
		g_print("polling-interval: %f\n", g_settings_get_double(g_settings, "polling-interval"));
		ui_app_set_polling_interval(ui_app, g_settings_get_double(g_settings, "polling-interval"));
		return;
	}
}

void ui_app_open(GApplication *g_application, GFile *files[], int nfiles, const char *hint) {
	printf("app open\n");

	G_APPLICATION_CLASS(ui_app_parent_class)->open(g_application, files, nfiles, hint);
}

void ui_app_activate(GApplication *g_application) {
	UiMainWindow *ui_main_window;

	UiApp *ui_app = UI_APP(g_application);
	GSettings *g_settings = ui_app->g_settings;

	gdouble interval = g_settings != NULL ? g_settings_get_double(g_settings, "polling-interval") : 5;
	ui_app_set_polling_interval(ui_app, interval);

	if (g_settings == NULL || g_settings_get_boolean(g_settings, "tray-icon")) {
		ui_app_start_tray(ui_app);
	}

	ui_main_window = ui_main_window_new(ui_app);
	gtk_window_present(GTK_WINDOW(ui_main_window));

	G_APPLICATION_CLASS(ui_app_parent_class)->activate(g_application);
}

void ui_app_dispose(GObject *g_object) {
	UiApp *ui_app = UI_APP(g_object);

	cgo_handle_delete(ui_app->ts_app);
	g_clear_object(&ui_app->css_provider);
	g_clear_object(&ui_app->g_settings);

	G_OBJECT_CLASS(ui_app_parent_class)->dispose(g_object);
}

void ui_app_action_quit(GSimpleAction *g_simple_action, GVariant *p, UiApp *ui_app) {
	ts_app_quit(ui_app->ts_app);
}

void ui_app_init_css_provider(UiApp *ui_app) {
	char *app_css;

	app_css = ui_get_file("app.css");
	ui_app->css_provider = gtk_css_provider_new();
	gtk_css_provider_load_from_string(ui_app->css_provider, app_css);
	gtk_style_context_add_provider_for_display(gdk_display_get_default(),
			GTK_STYLE_PROVIDER(ui_app->css_provider),
			GTK_STYLE_PROVIDER_PRIORITY_APPLICATION);

	free(app_css);
}

void ui_app_init_g_settings(UiApp *ui_app) {
	if (g_settings_schema_found) {
		ui_app->g_settings = g_settings_new(APP_ID);
		g_signal_connect(ui_app->g_settings, "changed", G_CALLBACK(ui_app_g_settings_changed), ui_app);
	}
}

void ui_app_init_actions(UiApp *ui_app) {
	GSimpleAction *g_simple_action;
	const char *quit_accels[] = {"<Ctrl>q", NULL};

	g_simple_action = g_simple_action_new("quit", NULL);
	g_signal_connect(g_simple_action, "activate", G_CALLBACK(ui_app_action_quit), ui_app);
	g_action_map_add_action(G_ACTION_MAP(ui_app), G_ACTION(g_simple_action));
	g_clear_object(&g_simple_action);

	gtk_application_set_accels_for_action(GTK_APPLICATION(ui_app), "app.quit", quit_accels);
}

void ui_app_init(UiApp *ui_app) {
	adw_init();

	ui_app_init_css_provider(ui_app);
	ui_app_init_g_settings(ui_app);
	ui_app_init_actions(ui_app);

	g_application_hold(G_APPLICATION(ui_app));
}

void ui_app_class_init_g_settings_schema_found() {
	GSettingsSchemaSource *g_settings_schema_source;
	GSettingsSchema *g_settings_schema;

	g_settings_schema_source = g_settings_schema_source_get_default();
	g_settings_schema = g_settings_schema_source_lookup(g_settings_schema_source, APP_ID, TRUE);
	g_settings_schema_found = g_settings_schema != NULL;
	if (g_settings_schema_found) {
		g_settings_schema_unref(g_settings_schema);
	}
}

void ui_app_class_init(UiAppClass *ui_app_class) {
	GApplicationClass *g_application_class = G_APPLICATION_CLASS(ui_app_class);
	GObjectClass *g_object_class = G_OBJECT_CLASS(ui_app_class);

	g_application_class->open = ui_app_open;
	g_application_class->activate = ui_app_activate;

	g_object_class->dispose = ui_app_dispose;

	signal_update_id = g_signal_new("update",
			G_TYPE_FROM_CLASS(ui_app_class),
			G_SIGNAL_RUN_LAST | G_SIGNAL_NO_RECURSE | G_SIGNAL_NO_HOOKS,
			0,
			NULL,
			NULL,
			NULL,
			G_TYPE_NONE,
			1,
			G_TYPE_POINTER);

	ui_app_class_init_g_settings_schema_found();
}
