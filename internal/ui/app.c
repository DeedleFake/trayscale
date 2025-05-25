#include <adwaita.h>

#include "ui.h"
#include "app.h"

G_DEFINE_TYPE(UiApp, ui_app, ADW_TYPE_APPLICATION);

UiApp *ui_app_new(TsApp ts_app) {
	UiApp *ui_app;

	ui_app = g_object_new(UI_APP_TYPE,
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

void ui_app_init(UiApp *ui_app) {
	adw_init();

	ui_app->css_provider = gtk_css_provider_new();
	gtk_css_provider_load_from_string(ui_app->css_provider, APP_CSS);
	gtk_style_context_add_provider_for_display(gdk_display_get_default(),
			GTK_STYLE_PROVIDER(ui_app->css_provider),
			GTK_STYLE_PROVIDER_PRIORITY_APPLICATION);

	g_application_hold(G_APPLICATION(ui_app));
}

void ui_app_open(GApplication *g_application, GFile *files[], int nfiles, const char *hint) {
	printf("app open\n");
}

void ui_app_activate(GApplication *g_application) {
	ui_app_start_tray(UI_APP(g_application));
}

void ui_app_dispose(GObject *g_object) {
	cgo_handle_delete(UI_APP(g_object)->ts_app);
	g_object_unref(UI_APP(g_object)->css_provider);
}

void ui_app_class_init(UiAppClass *ui_app_class) {
	G_APPLICATION_CLASS(ui_app_class)->open = ui_app_open;
	G_APPLICATION_CLASS(ui_app_class)->activate = ui_app_activate;

	G_OBJECT_CLASS(ui_app_class)->dispose = ui_app_dispose;
}

void ui_app_notify(UiApp *ui_app, const char *title, const char *body) {
	GIcon *icon;
	GNotification *notification;
	GError *err = NULL;

	icon = g_icon_new_for_string(APP_ID, &err);

	notification = g_notification_new(title);
	g_notification_set_body(notification, body);
	if (err == NULL) {
		g_notification_set_icon(notification, icon);
	}

	g_application_send_notification(G_APPLICATION(ui_app), "tailscale-status", notification);

	g_object_unref(notification);
	g_object_unref(icon);
}
