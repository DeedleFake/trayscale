package ui

/*
#include <adwaita.h>

#include "app.h"
*/
import "C"

import (
	"os"
	"unsafe"
)

type App C.UiApp

func NewApp() *App {
	return (*App)(C.ui_app_new())
}

func (app *App) c() *C.UiApp {
	return (*C.UiApp)(app)
}

func (app *App) Run() {
	args := toCStrings(os.Args)
	defer freeAll(args)

	C.ui_app_run(app.c(), C.int(len(args)), unsafe.SliceData(args))
}

func (app *App) Quit() {
	C.ui_app_quit(app.c())
}
