package ui

import (
	_ "embed"

	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/gio"
	"deedles.dev/trayscale/internal/gobj"
	"deedles.dev/trayscale/internal/tsutil"
)

//go:embed offlinepage.ui
var offlinePageXML string

var offlinePageClass = gobj.Widget[OfflinePage](gobj.WidgetSpec{
	Name:     "TrayscaleOfflinePage",
	Parent:   adw.BinGLibType,
	Template: offlinePageXML,
})

type OfflinePage struct {
	gobj.Base

	Page           *adw.StatusPage
	NeedsAuthGroup *adw.PreferencesGroup
}

func (page *OfflinePage) Actions() *gio.SimpleActionGroup {
	return nil
}

func (page *OfflinePage) Bind(stackPage *adw.ViewStackPage) {
	stackPage.SetTitle(page.Page.GetTitle())
	stackPage.SetIconName(page.Page.GetIconName())
}

func (page *OfflinePage) Destroy() {
	page.Widget().Unref()
}

func (page *OfflinePage) Update(status tsutil.Status) bool {
	if status, ok := status.(*tsutil.IPNStatus); ok {
		page.NeedsAuthGroup.SetVisible(status.NeedsAuth())
		return !status.Online()
	}
	return true
}
