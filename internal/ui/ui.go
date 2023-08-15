package ui

/*
#cgo pkg-config: gtk4 libadwaita-1
#include "ui.h"
*/
import "C"

import (
	"context"

	"deedles.dev/trayscale/internal/tsutil"
)

type UI struct {
	TS *tsutil.Client
}

func (ui *UI) Run(ctx context.Context) error {
	C._run_ui()
	return nil
}
