#include <adwaita.h>

#include "ui.h"
#include "app.h"
#include "main_window.h"

G_DEFINE_TYPE(UiMainWindow, ui_main_window, ADW_TYPE_APPLICATION_WINDOW);

static GMenuModel *menu_model_main, *menu_model_page;

UiMainWindow *ui_main_window_new(UiApp *ui_app) {
	UiMainWindow *ui_main_window;

	ui_main_window = g_object_new(UI_TYPE_MAIN_WINDOW,
			"application", ui_app,
			NULL);
	ui_main_window->ui_app = ui_app;

	return ui_main_window;
}

void ui_main_window_dispose(GObject *g_object) {
	GtkWidget *gtk_widget = GTK_WIDGET(g_object);

	gtk_widget_dispose_template(gtk_widget, UI_TYPE_MAIN_WINDOW);

	G_OBJECT_CLASS(ui_main_window_parent_class)->dispose(g_object);
}

void ui_main_window_init(UiMainWindow *ui_main_window) {
	gtk_widget_init_template(GTK_WIDGET(ui_main_window));

	gtk_menu_button_set_menu_model(ui_main_window->main_menu_button, menu_model_main);
	gtk_menu_button_set_menu_model(ui_main_window->page_menu_button, menu_model_page);
}

void ui_main_window_class_init(UiMainWindowClass *ui_main_window_class) {
	GBytes *template;
	char *menu_ui;
	GtkBuilder *gtk_builder;

	GtkWidgetClass *gtk_widget_class = GTK_WIDGET_CLASS(ui_main_window_class);
	GObjectClass *g_object_class = G_OBJECT_CLASS(ui_main_window_class);

	g_object_class->dispose = ui_main_window_dispose;

	template = ui_get_file_bytes("main_window.ui");
	gtk_widget_class_set_template(gtk_widget_class, template);
	gtk_widget_class_bind_template_child(gtk_widget_class, UiMainWindow, main_menu_button);
	gtk_widget_class_bind_template_child(gtk_widget_class, UiMainWindow, page_menu_button);

	menu_ui = ui_get_file("menu.ui");
	gtk_builder = gtk_builder_new_from_string(menu_ui, -1);
	menu_model_main = G_MENU_MODEL(gtk_builder_get_object(gtk_builder, "main_menu"));
	menu_model_page = G_MENU_MODEL(gtk_builder_get_object(gtk_builder, "page_menu"));

	g_object_ref(menu_model_main);
	g_object_ref(menu_model_page);

	g_object_unref(gtk_builder);
	free(menu_ui);
	g_bytes_unref(template);
}
