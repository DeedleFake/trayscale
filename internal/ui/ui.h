#pragma once

extern char *APP_ID;

void cgo_handle_delete(uintptr_t p);
char *ui_get_file(char *name);

typedef uintptr_t TsApp;
typedef uintptr_t TsutilStatus;
