#pragma once

#define UI_TYPE_MAIN_WINDOW ui_main_window_get_type()
G_DECLARE_FINAL_TYPE(UiMainWindow, ui_main_window, UI, MAIN_WINDOW, AdwApplicationWindow);

struct _UiMainWindow {
	AdwApplicationWindow parent;

	UiApp *ui_app;
	GtkMenuButton *main_menu_button, *page_menu_button;
};

struct _UiMainWindowClass {
	AdwApplicationWindowClass parent;
};

UiMainWindow *ui_main_window_new(UiApp *ui_app);
