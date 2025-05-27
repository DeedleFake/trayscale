#pragma once

extern char *APP_ID;

void cgo_handle_delete(uintptr_t p);
char *ui_get_file(char *name);
GBytes *ui_get_file_bytes(char *name);

typedef uintptr_t TsApp;
void ts_app_quit(TsApp ts_app);

typedef uintptr_t TsutilStatus;

gboolean tsutil_is_ipnstatus(TsutilStatus tsutil_status);
gboolean tsutil_is_filestatus(TsutilStatus tsutil_status);
gboolean tsutil_is_profilestatus(TsutilStatus tsutil_status);

gboolean tsutil_ipnstatus_online(TsutilStatus tsutil_status);

#define UI_TYPE_APP ui_app_get_type()
G_DECLARE_FINAL_TYPE(UiApp, ui_app, UI, APP, AdwApplication);

struct _UiApp {
	AdwApplication parent_instance;

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
void ui_app_update(UiApp *app, TsutilStatus tsutil_status);

#define UI_TYPE_MAIN_WINDOW ui_main_window_get_type()
G_DECLARE_FINAL_TYPE(UiMainWindow, ui_main_window, UI, MAIN_WINDOW, AdwApplicationWindow);

struct _UiMainWindow {
	AdwApplicationWindow parent_instance;

	UiApp *ui_app;
	GtkMenuButton *main_menu_button, *page_menu_button;
	GtkSwitch *status_switch;
};

struct _UiMainWindowClass {
	AdwApplicationWindowClass parent;
};

UiMainWindow *ui_main_window_new(UiApp *ui_app);

extern GMenuModel *menu_model_main, *menu_model_page;
