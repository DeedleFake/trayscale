#include <adwaita.h>

#include "_cgo_export.h"
#include "ui.h"
#include "app.h"

G_DEFINE_TYPE(UiApp, ui_app, ADW_TYPE_APPLICATION);

UiApp *ui_app_new(void) {
	UiApp *ui_app;

	ui_app = g_object_new(UI_APP_TYPE,
			"application-id", APP_ID,
			"flags", G_APPLICATION_HANDLES_OPEN,
			NULL);
	return ui_app;
}

void ui_app_run(UiApp *ui_app, int argc, char *argv[]) {
	g_application_run(G_APPLICATION(ui_app), argc, argv);
}

void ui_app_quit(UiApp *ui_app) {
	g_application_quit(G_APPLICATION(ui_app));
}

void ui_app_init(UiApp *ui_app) {
	GtkCssProvider *css;

	adw_init();
	css = gtk_css_provider_new();
	gtk_css_provider_load_from_string(css, APP_CSS);
	gtk_style_context_add_provider_for_display(gdk_display_get_default(),
			GTK_STYLE_PROVIDER(css),
			GTK_STYLE_PROVIDER_PRIORITY_APPLICATION);

	g_application_hold(G_APPLICATION(ui_app));
}

void ui_app_open(GApplication *g_application, GFile *files[], int nfiles, const char *hint) {
	printf("app open\n");
}

void ui_app_activate(GApplication *g_application) {
	printf("app activate\n");
}

void ui_app_class_init(UiAppClass *ui_app_class) {
	G_APPLICATION_CLASS(ui_app_class)->open = ui_app_open;
	G_APPLICATION_CLASS(ui_app_class)->activate = ui_app_activate;
}
