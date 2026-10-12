package ui

import (
	_ "embed"

	"deedles.dev/trayscale/internal/gutil"
	"deedles.dev/trayscale/internal/tsutil"
	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/gio"
	"codeberg.org/puregotk/puregotk/v4/gtk"
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
	gutil.FillFromUI(&page, offlinePageXML)
	return &page
}

func (page *OfflinePage) Widget() *gtk.Widget {
	return &page.Page.Widget
}

func (page *OfflinePage) Actions() *gio.SimpleActionGroup {
	return nil
}

func (page *OfflinePage) Bind(stackPage *adw.ViewStackPage) {
	stackPage.SetTitle(page.Page.GetTitle())
	stackPage.SetIconName(page.Page.GetIconName())
}

func (page *OfflinePage) Destroy() {
	gutil.UnrefFields(page)
}

func (page *OfflinePage) Update(status tsutil.Status) bool {
	if status, ok := status.(*tsutil.IPNStatus); ok {
		page.NeedsAuthGroup.SetVisible(status.NeedsAuth())
		return !status.Online()
	}
	return true
}
