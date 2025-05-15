package ui

import (
	_ "embed"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

//go:embed preferences.ui
var preferencesXML string

type PreferencesDialog struct {
	*adw.PreferencesDialog `gtk:"PreferencesDialog"`

	UseTrayIconRow            *adw.SwitchRow
	PollingIntervalRow        *adw.SpinRow
	PollingIntervalAdjustment *gtk.Adjustment
}

func NewPreferencesDialog() *PreferencesDialog {
	var win PreferencesDialog
	fillFromBuilder(&win, preferencesXML)
	return &win
}
