package ui

import (
	"deedles.dev/trayscale/internal/tsutil"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type OfflinePage struct {
	app *App

	Page *adw.StatusPage
}

func NewOfflinePage(app *App) *OfflinePage {
	page := OfflinePage{app: app}

	page.Page = adw.NewStatusPage()
	page.Page.SetTitle("Not Connected")
	page.Page.SetIconName("network-offline-symbolic")
	page.Page.SetDescription("Tailscale is not connected")

	return &page
}

func (page *OfflinePage) Widget() gtk.Widgetter {
	return page.Page
}

func (page *OfflinePage) Actions() gio.ActionGrouper {
	return nil
}

func (page *OfflinePage) Init(row *PageRow) {
	row.SetTitle(page.Page.Title())
	row.SetIconName(page.Page.IconName())
}

func (page *OfflinePage) Update(status *tsutil.Status) bool {
	return !status.Online()
}
