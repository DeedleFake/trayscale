package ui

import (
	_ "embed"

	"deedles.dev/trayscale/internal/gutil"
	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/gtk"
)

//go:embed preferences.ui
var preferencesXML string

type PreferencesDialog struct {
	PreferencesDialog            *adw.PreferencesDialog
	UseTrayIconRow               *adw.SwitchRow
	PollingIntervalRow           *adw.SpinRow
	PollingIntervalAdjustment    *gtk.Adjustment
	TaildropAutoSaveRow          *adw.SwitchRow
	TaildropAutoSaveFolderButton *gtk.Button
}

func NewPreferencesDialog() *PreferencesDialog {
	var win PreferencesDialog
	gutil.FillFromUI(&win, preferencesXML)
	return &win
}
