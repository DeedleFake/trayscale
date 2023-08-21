package ui

import (
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

type Info struct {
	Heading string
	Body    string
}

func (d Info) Show(a *App, closed func()) {
	dialog := adw.NewMessageDialog(&a.win.Window, d.Heading, d.Body)
	dialog.SetBodyUseMarkup(true)
	dialog.AddResponse("close", "_Close")
	dialog.SetDefaultResponse("close")

	if closed != nil {
		dialog.ConnectResponse(func(string) {
			closed()
		})
	}

	dialog.Show()
}
