#ifndef UI_UI_H
#define UI_UI_H

extern const char *APP_ID;

#define DECLARE_RESOURCE(name) extern const char *name; extern int name##_LEN
DECLARE_RESOURCE(APP_CSS);

#endif
