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

	dialog.SetVisible(true)
}

type Prompt struct {
	Heading   string
	Body      string
	Responses []PromptResponse
}

type PromptResponse struct {
	ID         string
	Label      string
	Appearance adw.ResponseAppearance
	Default    bool
}

func (d Prompt) Show(a *App, initialValue string, res func(response, val string)) {
	input := gtk.NewText()
	if initialValue != "" {
		input.Buffer().SetText(initialValue, len(initialValue))
	}

	dialog := adw.NewMessageDialog(&a.win.Window, d.Heading, d.Body)
	dialog.SetExtraChild(input)

	def := "activate"
	for _, r := range d.Responses {
		dialog.AddResponse(r.ID, r.Label)
		dialog.SetResponseAppearance(r.ID, r.Appearance)
		if r.Default {
			dialog.SetDefaultResponse(r.ID)
			def = r.ID
		}
	}

	dialog.ConnectResponse(func(response string) {
		res(response, input.Buffer().Text())
	})
	input.ConnectActivate(func() {
		defer dialog.Close()
		res(def, input.Buffer().Text())
	})

	dialog.SetVisible(true)
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

	dialog.SetVisible(true)
}

type Select[T any] struct {
	Heading  string
	Body     string
	Options  []SelectOption[T]
	Multiple bool
}

type SelectOption[T any] struct {
	Title    string
	Subtitle string
	Value    T
}

func (d Select[T]) Show(a *App, res func([]T)) {
	options := gtk.NewListBox()
	options.AddCSSClass("boxed-list")
	options.SetSelectionMode(gtk.SelectionSingle)
	if d.Multiple {
		// BUG: See https://gitlab.gnome.org/GNOME/gtk/-/issues/552.
		options.SetSelectionMode(gtk.SelectionMultiple)
	}
	for _, option := range d.Options {
		row := adw.NewActionRow()
		row.SetTitle(option.Title)
		row.SetSubtitle(option.Subtitle)
		row.SetSelectable(true)
		options.Append(row)
	}

	scroll := gtk.NewScrolledWindow()
	scroll.SetMaxContentHeight(500)
	scroll.SetPropagateNaturalHeight(true)
	scroll.SetChild(options)

	dialog := adw.NewMessageDialog(&a.win.Window, d.Heading, d.Body)
	dialog.SetExtraChild(scroll)

	dialog.AddResponse("select", "Select")
	dialog.SetResponseAppearance("select", adw.ResponseSuggested)
	dialog.SetDefaultResponse("select")

	dialog.AddResponse("cancel", "Cancel")

	dialog.ConnectResponse(func(response string) {
		if response != "select" {
			res(nil)
			return
		}

		rows := options.SelectedRows()
		selected := make([]T, 0, len(rows))
		for _, row := range rows {
			option := d.Options[row.Index()]
			selected = append(selected, option.Value)
		}
		res(selected)
	})

	dialog.SetVisible(true)
}
