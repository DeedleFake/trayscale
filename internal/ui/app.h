#pragma once

#define UI_APP_TYPE ui_app_get_type()
G_DECLARE_FINAL_TYPE(UiApp, ui_app, UI, APP, AdwApplication);

struct _UiApp {
	AdwApplication parent;

	TsApp ts_app;
	GtkCssProvider *css_provider;
	GSettings *g_settings;

	gboolean online;
};

struct _UiAppClass {
	AdwApplicationClass parent;
};

UiApp *ui_app_new(TsApp ts_app);
void ui_app_run(UiApp *app, int argc, char *argv[]);
void ui_app_quit(UiApp *app);
gboolean ui_app_start_tray(UiApp *app);
gboolean ui_app_stop_tray(UiApp *app);
void ui_app_set_polling_interval(UiApp *app, gdouble interval);
void ui_app_notify(UiApp *app, const char *title, const char *body);
