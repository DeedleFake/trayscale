#include <adwaita.h>

#include "_cgo_export.h"
#include "ui.h"
#include "app.h"

G_DEFINE_TYPE(UiApp, ui_app, ADW_TYPE_APPLICATION);

void ui_app_init(UiApp *app);
void ui_app_class_init(UiAppClass *app_class);
void ui_app_activate(GApplication *gapplication);

UiApp *ui_app_new(void) {
	UiApp *app;

	app = g_object_new(UI_APP_TYPE,
			"application-id", APP_ID,
			"flags", G_APPLICATION_HANDLES_OPEN,
			NULL);
	return app;
}

void ui_app_run(UiApp *app, int argc, char *argv[]) {
	g_application_run(G_APPLICATION(app), argc, argv);
}

void ui_app_quit(UiApp *app) {
	g_application_quit(G_APPLICATION(app));
}

void ui_app_init(UiApp *app) {
	GtkCssProvider *css;

	adw_init();
	css = gtk_css_provider_new();
	gtk_css_provider_load_from_string(css, APP_CSS);
	gtk_style_context_add_provider_for_display(gdk_display_get_default(),
			GTK_STYLE_PROVIDER(css),
			GTK_STYLE_PROVIDER_PRIORITY_APPLICATION);
}

void ui_app_class_init(UiAppClass *app_class) {
	G_APPLICATION_CLASS(app_class)->activate = ui_app_activate;
}

void ui_app_activate(GApplication *gapplication) {
	printf("app activate\n");
}
