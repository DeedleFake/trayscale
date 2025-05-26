#include <adwaita.h>

#include "ui.h"
#include "app.h"
#include "main_window.h"

G_DEFINE_TYPE(UiMainWindow, ui_main_window, ADW_TYPE_APPLICATION_WINDOW);

static GMenuModel *ui_main_window_main_menu, *ui_main_window_page_menu;

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

	gtk_menu_button_set_menu_model(ui_main_window->main_menu_button, ui_main_window_main_menu);
	gtk_menu_button_set_menu_model(ui_main_window->page_menu_button, ui_main_window_page_menu);
}

void ui_main_window_class_init(UiMainWindowClass *ui_main_window_class) {
	GBytes *template;
	char *menu_ui;
	GtkBuilder *gtk_builder;

	GtkWidgetClass *gtk_widget_class = GTK_WIDGET_CLASS(ui_main_window_class);

	template = ui_get_file_bytes("main_window.ui");
	gtk_widget_class_set_template(gtk_widget_class, template);
	gtk_widget_class_bind_template_child(gtk_widget_class, UiMainWindow, main_menu_button);
	gtk_widget_class_bind_template_child(gtk_widget_class, UiMainWindow, page_menu_button);

	menu_ui = ui_get_file("menu.ui");
	gtk_builder = gtk_builder_new_from_string(menu_ui, -1);
	ui_main_window_main_menu = G_MENU_MODEL(gtk_builder_get_object(gtk_builder, "main_menu"));
	ui_main_window_page_menu = G_MENU_MODEL(gtk_builder_get_object(gtk_builder, "page_menu"));

	g_object_ref(ui_main_window_main_menu);
	g_object_ref(ui_main_window_page_menu);

	g_object_unref(gtk_builder);
	free(menu_ui);
	g_bytes_unref(template);
}
