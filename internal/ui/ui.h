#pragma once

extern char *APP_ID;

void cgo_handle_delete(uintptr_t p);
char *ui_get_file(char *name);
GBytes *ui_get_file_bytes(char *name);

typedef uintptr_t TsApp;
typedef uintptr_t TsutilStatus;
