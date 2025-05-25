#ifndef APP_H
#define APP_H

extern char *APP_ID;

#define UI_APP_TYPE ui_app_get_type()
G_DECLARE_FINAL_TYPE(UiApp, ui_app, UI, APP, AdwApplication);

struct _UiApp {
	AdwApplication parent;
};

struct _UiAppClass {
	AdwApplicationClass parent;
};

UiApp *ui_app_new(void);
void ui_app_run(UiApp *app, int argc, char *argv[]);
void ui_app_quit(UiApp *app);

#endif
