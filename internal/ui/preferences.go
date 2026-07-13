package ui

import (
	_ "embed"

	"deedles.dev/trayscale/internal/gutil"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

//go:embed preferences.ui
var preferencesXML string

type PreferencesDialog struct {
	PreferencesDialog         *adw.PreferencesDialog
	UseTrayIconRow            *adw.SwitchRow
	PollingIntervalRow        *adw.SpinRow
	PollingIntervalAdjustment *gtk.Adjustment
}

func NewPreferencesDialog() *PreferencesDialog {
	var win PreferencesDialog
	gutil.FillFromUI(&win, preferencesXML)
	return &win
}
