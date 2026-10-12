package ui

import (
	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/gtk"
	"deedles.dev/trayscale/internal/gutil"
	"deedles.dev/trayscale/internal/locale"
)

// window returns the main window, borrowed, or nil.
func (a *App) window() *gtk.Window {
	if a == nil {
		return nil
	}
	if a.win == nil {
		return nil
	}

	return &a.win.MainWindow.Window
}

// parent returns the main window as a dialog parent. puregotk can't
// pass a nil pointer for a nullable parameter, so with no window it
// returns a wrapper around NULL.
func (a *App) parent() *gtk.Widget {
	if w := a.window(); w != nil {
		return &w.Widget
	}
	return &gtk.Widget{}
}

// present shows dialog and drops the caller's reference; a presented
// dialog keeps itself alive until it's closed.
func (a *App) present(dialog *adw.AlertDialog) {
	dialog.Present(a.parent())
	dialog.Unref()
}

type Confirmation struct {
	Heading string
	Body    string
	Accept  string
	Reject  string
}

func (d Confirmation) Show(a *App, res func(bool)) {
	dialog := adw.NewAlertDialog(d.Heading, d.Body)
	dialog.AddResponse("reject", d.Reject)
	dialog.SetCloseResponse("reject")
	dialog.AddResponse("accept", d.Accept)
	dialog.SetResponseAppearance("accept", adw.ResponseSuggestedValue)
	dialog.SetDefaultResponse("accept")

	gutil.Connect(dialog, dialog.ConnectResponse, func(_ adw.AlertDialog, response string) {
		res(response == "accept")
	})

	a.present(dialog)
}

type Prompt struct {
	Heading     string
	Body        string
	Placeholder string
	Purpose     gtk.InputPurpose
	Responses   []PromptResponse
}

type PromptResponse struct {
	ID         string
	Label      string
	Appearance adw.ResponseAppearance
	Default    bool
}

func (d Prompt) Show(a *App, initialValue string, res func(response, val string)) {
	input := gtk.NewEntry()
	input.SetText(initialValue)
	input.SetInputPurpose(d.Purpose)
	input.SetPlaceholderText(d.Placeholder)

	dialog := adw.NewAlertDialog(d.Heading, d.Body)
	dialog.SetExtraChild(&input.Widget)

	def := "activate"
	for _, r := range d.Responses {
		dialog.AddResponse(r.ID, r.Label)
		dialog.SetResponseAppearance(r.ID, r.Appearance)
		if r.Default {
			dialog.SetDefaultResponse(r.ID)
			def = r.ID
		}
	}

	gutil.Connect(dialog, dialog.ConnectResponse, func(_ adw.AlertDialog, response string) {
		res(response, input.GetText())
	})
	gutil.Connect(input, input.ConnectActivate, func(gtk.Entry) {
		defer dialog.Close()
		res(def, input.GetText())
	})
	input.Unref() // The dialog holds it now.

	a.present(dialog)
}

type Info struct {
	Heading string
	Body    string
	// Extra returns a new widget, which the dialog takes over.
	Extra func() *gtk.Widget
}

func (d Info) Show(a *App, closed func()) {
	dialog := adw.NewAlertDialog(d.Heading, d.Body)
	dialog.SetBodyUseMarkup(true)
	dialog.AddResponse("close", locale.Get("_Close"))
	dialog.SetDefaultResponse("close")
	if d.Extra != nil {
		extra := d.Extra()
		dialog.SetExtraChild(extra)
		extra.Unref()
	}

	if closed != nil {
		gutil.Connect(dialog, dialog.ConnectResponse, func(adw.AlertDialog, string) {
			closed()
		})
	}

	a.present(dialog)
}

type Select[T any] struct {
	Heading string
	Body    string
	Options []SelectOption[T]
}

type SelectOption[T any] struct {
	Title    string
	Subtitle string
	Value    T
}

func (d Select[T]) Show(a *App, res func([]SelectOption[T])) {
	options := gtk.NewListBox()
	options.AddCssClass("boxed-list")
	options.SetSelectionMode(gtk.SelectionSingleValue)
	for _, option := range d.Options {
		row := adw.NewActionRow()
		row.SetTitle(option.Title)
		row.SetSubtitle(option.Subtitle)
		row.SetSelectable(true)
		options.Append(&row.Widget)
		row.Unref()
	}

	dialog := adw.NewAlertDialog(d.Heading, d.Body)
	dialog.SetExtraChild(&options.Widget)

	dialog.AddResponse("select", locale.Get("Select"))
	dialog.SetResponseAppearance("select", adw.ResponseSuggestedValue)
	dialog.SetDefaultResponse("select")

	dialog.AddResponse("cancel", locale.Get("Cancel"))

	gutil.Connect(dialog, dialog.ConnectResponse, func(_ adw.AlertDialog, response string) {
		if response != "select" {
			res(nil)
			return
		}

		// Single selection, so there is at most one selected row.
		var selected []SelectOption[T]
		if row := options.GetSelectedRow(); row != nil {
			selected = append(selected, d.Options[row.GetIndex()])
			row.Unref()
		}
		res(selected)
	})
	options.Unref() // The dialog holds it now.

	a.present(dialog)
}
