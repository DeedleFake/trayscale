#ifndef APP_H
#define APP_H

extern char *APP_ID;

typedef struct {
	AdwApplication parent;
} App;

typedef struct {
	AdwApplicationClass parent;
} AppClass;

App *app_new(void);
void app_run(App *app, int argc, char *argv[]);
void app_quit(App *app);

#endif
