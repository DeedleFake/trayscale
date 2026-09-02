package ui

import (
	_ "embed"
	"slices"

	"deedles.dev/trayscale/internal/gutil"
	"deedles.dev/trayscale/internal/tsutil"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
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

	// Auto-VPN
	AutoVpnGroup             *adw.PreferencesGroup
	AutoVpnEnabledRow        *adw.SwitchRow
	AutoVpnCurrentNetworkRow *adw.ActionRow

	// tracked trusted SSID rows for removal
	trustedRows map[string]*adw.ActionRow
}

func NewPreferencesDialog() *PreferencesDialog {
	var win PreferencesDialog
	gutil.FillFromUI(&win, preferencesXML)
	win.trustedRows = make(map[string]*adw.ActionRow)
	return &win
}

func (d *PreferencesDialog) addTrustedRow(a *App, ssid string) {
	if _, exists := d.trustedRows[ssid]; exists {
		return
	}

	row := adw.NewActionRow()
	row.SetTitle(ssid)
	row.AddPrefix(gtk.NewImageFromIconName("network-wireless-symbolic"))

	removeBtn := gtk.NewButtonFromIconName("user-trash-symbolic")
	removeBtn.SetVAlign(gtk.AlignCenter)
	removeBtn.AddCSSClass("flat")
	removeBtn.SetTooltipText("Remove")
	removeBtn.ConnectClicked(func() {
		current := a.settings.Strv("auto-vpn-trusted-ssids")
		current = slices.DeleteFunc(current, func(v string) bool { return v == ssid })
		a.settings.SetStrv("auto-vpn-trusted-ssids", current)
		d.removeTrustedRow(ssid)
	})
	row.AddSuffix(removeBtn)

	d.trustedRows[ssid] = row
	d.AutoVpnGroup.Add(row)
}

func (d *PreferencesDialog) removeTrustedRow(ssid string) {
	if row, exists := d.trustedRows[ssid]; exists {
		d.AutoVpnGroup.Remove(row)
		delete(d.trustedRows, ssid)
	}
}

// setupAutoVPN wires up the Auto-VPN UI elements.
func (d *PreferencesDialog) setupAutoVPN(a *App) {
	if a.settings == nil {
		return
	}

	// Auto-VPN resolves the current Wi-Fi SSID through NetworkManager.
	// There is no portable alternative -- neither gio.NetworkMonitor nor
	// the NetworkMonitor portal expose SSIDs -- so without it the feature
	// cannot work. Disable the controls and say why rather than offering
	// settings that would silently do nothing.
	if !tsutil.NetworkManagerAvailable() {
		d.AutoVpnGroup.SetSensitive(false)
		d.AutoVpnGroup.SetDescription("Unavailable: Auto-VPN needs NetworkManager to identify the current Wi-Fi network.")
		d.AutoVpnCurrentNetworkRow.SetSubtitle("NetworkManager not available")
		return
	}

	ssid := tsutil.DetectCurrentSSID()

	trustedSSIDs := a.settings.Strv("auto-vpn-trusted-ssids")
	isTrusted := slices.Contains(trustedSSIDs, ssid)

	if ssid != "" {
		d.AutoVpnCurrentNetworkRow.SetSubtitle(ssid)

		trustBtn := gtk.NewButton()
		if isTrusted {
			trustBtn.SetLabel("Untrust")
			trustBtn.AddCSSClass("destructive-action")
		} else {
			trustBtn.SetLabel("Trust")
			trustBtn.AddCSSClass("suggested-action")
		}
		trustBtn.SetVAlign(gtk.AlignCenter)
		trustBtn.ConnectClicked(func() {
			current := a.settings.Strv("auto-vpn-trusted-ssids")
			if slices.Contains(current, ssid) {
				current = slices.DeleteFunc(current, func(s string) bool { return s == ssid })
				trustBtn.SetLabel("Trust")
				trustBtn.RemoveCSSClass("destructive-action")
				trustBtn.AddCSSClass("suggested-action")
				d.removeTrustedRow(ssid)
			} else {
				current = append(current, ssid)
				trustBtn.SetLabel("Untrust")
				trustBtn.RemoveCSSClass("suggested-action")
				trustBtn.AddCSSClass("destructive-action")
				d.addTrustedRow(a, ssid)
			}
			a.settings.SetStrv("auto-vpn-trusted-ssids", current)
		})
		d.AutoVpnCurrentNetworkRow.AddSuffix(trustBtn)
	} else {
		d.AutoVpnCurrentNetworkRow.SetSubtitle("Not connected to Wi-Fi")
	}

	// Manual SSID entry row
	entryRow := adw.NewEntryRow()
	entryRow.SetTitle("Add trusted SSID")
	entryRow.ConnectApply(func() {
		text := entryRow.Text()
		if text == "" {
			return
		}
		current := a.settings.Strv("auto-vpn-trusted-ssids")
		if !slices.Contains(current, text) {
			current = append(current, text)
			a.settings.SetStrv("auto-vpn-trusted-ssids", current)
			d.addTrustedRow(a, text)
		}
		entryRow.SetText("")
	})
	entryRow.SetShowApplyButton(true)
	d.AutoVpnGroup.Add(entryRow)

	// Build initial trusted SSID list
	for _, s := range trustedSSIDs {
		d.addTrustedRow(a, s)
	}
}

// addAutoVPNActions sets up the settings bindings and callbacks for
// the Auto-VPN preferences UI.
func (d *PreferencesDialog) addAutoVPNActions(a *App, settings *gio.Settings) {
	settings.Bind("auto-vpn-enabled", d.AutoVpnEnabledRow.Object, "active", gio.SettingsBindDefault)
	d.setupAutoVPN(a)
}
