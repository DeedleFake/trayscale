package ui

import (
	_ "embed"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

//go:embed preferences.ui
var preferencesXML string

type PreferencesWindow struct {
	*adw.PreferencesWindow `gtk:"PreferencesWindow"`

	UseTrayIconRow            *adw.SwitchRow
	PollingIntervalRow        *adw.SpinRow
	PollingIntervalAdjustment *gtk.Adjustment
	ControlURLRow             *adw.EntryRow
}

func NewPreferencesWindow() *PreferencesWindow {
	var win PreferencesWindow
	fillFromBuilder(&win, preferencesXML)
	return &win
}
