package ui

import (
	_ "embed"

	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/gtk"
	"deedles.dev/trayscale/internal/gobj"
)

//go:embed preferences.ui
var preferencesXML string

var preferencesDialogClass = gobj.Widget[PreferencesDialog](gobj.WidgetSpec{
	Name:     "TrayscalePreferencesDialog",
	Parent:   adw.PreferencesDialogGLibType,
	Template: preferencesXML,
})

type PreferencesDialog struct {
	gobj.Base

	PreferencesDialog            *adw.PreferencesDialog `gtk:"-"`
	UseTrayIconRow               *adw.SwitchRow
	PollingIntervalRow           *adw.SpinRow
	PollingIntervalAdjustment    *gtk.Adjustment
	TaildropAutoSaveRow          *adw.SwitchRow
	TaildropAutoSaveFolderButton *gtk.Button
}

func NewPreferencesDialog() *PreferencesDialog {
	win := preferencesDialogClass.New()
	win.PreferencesDialog = adw.PreferencesDialogNewFromInternalPtr(win.GoPointer())
	return win
}
