#include <adwaita.h>

#include "_cgo_export.h"
#include "app.h"

G_DEFINE_TYPE(App, app, ADW_TYPE_APPLICATION);

void app_init(App *app);
void app_class_init(AppClass *app_class);
void app_activate(GApplication *gapp);

App *app_new(void) {
	App *app;

	app = g_object_new(app_get_type(),
			"application-id", APP_ID,
			"flags", G_APPLICATION_HANDLES_OPEN,
			NULL);
	return app;
}

void app_run(App *app, int argc, char *argv[]) {
	g_application_run(G_APPLICATION(app), argc, argv);
}

void app_quit(App *app) {
	g_application_quit(G_APPLICATION(app));
}

void app_init(App *app) {
	printf("app init\n");
}

void app_class_init(AppClass *app_class) {
	GApplicationClass *g_application_class = G_APPLICATION_CLASS(app_class);

	g_application_class->activate = app_activate;
}

void app_activate(GApplication *gapp) {
	printf("app activate\n");
}
