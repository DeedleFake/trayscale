package ui

import (
	_ "embed"

	"deedles.dev/trayscale/internal/tsutil"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

//go:embed offlinepage.ui
var offlinePageXML string

type OfflinePage struct {
	app *App

	Page           *adw.StatusPage
	NeedsAuthGroup *adw.PreferencesGroup
}

func NewOfflinePage(app *App) *OfflinePage {
	page := OfflinePage{app: app}
	fillFromBuilder(&page, offlinePageXML)
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

func (page *OfflinePage) Update(status tsutil.Status) bool {
	if status, ok := status.(*tsutil.IPNStatus); ok {
		page.NeedsAuthGroup.SetVisible(status.NeedsAuth())
		return !status.Online()
	}
	return true
}
