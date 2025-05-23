package gutil

import (
	"context"

	"github.com/jwijenbergh/puregotk/v4/gio"
)

func ContextCancellable(ctx context.Context) *gio.Cancellable {
	c := gio.NewCancellable()
	context.AfterFunc(ctx, c.Cancel)
	return c
}
