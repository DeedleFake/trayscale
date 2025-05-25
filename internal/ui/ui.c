#include <adwaita.h>

#include "ui.h"

char *APP_ID = NULL;

#define DEFINE_RESOURCE(name) char *name = NULL; int name##_LEN = 0
DEFINE_RESOURCE(APP_CSS);
