#include <adwaita.h>

#include "ui.h"
#include "app.h"
#include "main_window.h"

G_DEFINE_TYPE(UiMainWindow, ui_main_window, ADW_TYPE_APPLICATION_WINDOW);

UiMainWindow *ui_main_window_new(UiApp *ui_app) {
	UiMainWindow *ui_main_window;

	ui_main_window = g_object_new(UI_TYPE_MAIN_WINDOW,
			"application", ui_app,
			NULL);
	ui_main_window->ui_app = ui_app;

	return ui_main_window;
}

void ui_main_window_init(UiMainWindow *ui_main_window) {
	gtk_widget_init_template(GTK_WIDGET(ui_main_window));
}

void ui_main_window_class_init(UiMainWindowClass *ui_main_window_class) {
	GBytes *template;

	GtkWidgetClass *gtk_widget_class = GTK_WIDGET_CLASS(ui_main_window_class);

	template = ui_get_file_bytes("main_window.ui");
	gtk_widget_class_set_template(gtk_widget_class, template);

	g_bytes_unref(template);
}
