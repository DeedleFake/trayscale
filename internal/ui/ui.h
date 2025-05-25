#pragma once

extern char *APP_ID;

#define DECLARE_RESOURCE(name) extern char *name; extern int name##_LEN
DECLARE_RESOURCE(APP_CSS);

void cgo_handle_delete(uintptr_t p);

typedef uintptr_t TsApp;
