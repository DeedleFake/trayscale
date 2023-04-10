package main

import (
	"context"
	"io"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type Confirmation struct {
	Heading string
	Body    string
	Accept  string
	Reject  string
}

func (d Confirmation) Show(a *App, res func(bool)) {
	dialog := adw.NewMessageDialog(&a.win.Window, d.Heading, d.Body)
	dialog.AddResponse("reject", d.Reject)
	dialog.SetCloseResponse("reject")
	dialog.AddResponse("accept", d.Accept)
	dialog.SetResponseAppearance("accept", adw.ResponseSuggested)
	dialog.SetDefaultResponse("accept")

	dialog.ConnectResponse(func(response string) {
		res(response == "accept")
	})

	dialog.Show()
}

type Prompt struct {
	Heading string
	Body    string
}

func (d Prompt) Show(a *App, res func(val string)) {
	input := gtk.NewText()

	dialog := adw.NewMessageDialog(&a.win.Window, d.Heading, d.Body)
	dialog.SetExtraChild(input)
	dialog.AddResponse("cancel", "_Cancel")
	dialog.SetCloseResponse("cancel")
	dialog.AddResponse("add", "_Add")
	dialog.SetResponseAppearance("add", adw.ResponseSuggested)
	dialog.SetDefaultResponse("add")

	dialog.ConnectResponse(func(response string) {
		switch response {
		case "add":
			res(input.Buffer().Text())
		}
	})
	input.ConnectActivate(func() {
		defer dialog.Close()
		res(input.Buffer().Text())
	})

	dialog.Show()
}

type GStream interface {
	Write(context.Context, []byte) (int, error)
}

type gwriter struct {
	ctx context.Context
	s   GStream
}

func NewGWriter(ctx context.Context, s GStream) io.Writer {
	return gwriter{ctx, s}
}

func (w gwriter) Write(data []byte) (int, error) {
	// TODO: Make this async and probably add a progress bar to the UI.
	return w.s.Write(w.ctx, data)
}
