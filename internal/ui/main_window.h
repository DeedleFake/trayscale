#pragma once

#define UI_MAIN_WINDOW_TYPE ui_main_window_get_type
G_DECLARE_FINAL_TYPE(UiMainWindow, ui_main_window, UI, MAIN_WINDOW, AdwApplicationWindow);

struct _UiMainWindow {
	AdwApplicationWindow parent;
}

struct _UiMainWindowClass {
	AdwApplicationWindowClass parent;
}

UiMainWindow *ui_main_window_new(UiApp *ui_app);
