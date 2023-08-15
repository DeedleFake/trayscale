package ui

import (
	_ "embed"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

//go:embed peerpage.ui
var peerPageXML []byte

type PeerPage struct {
	gtk.Widget

	Page                    *adw.StatusPage
	IPGroup                 *adw.PreferencesGroup
	OptionsGroup            *adw.PreferencesGroup
	AdvertiseExitNodeRow    *adw.ActionRow
	AdvertiseExitNodeSwitch *gtk.Switch
	AllowLANAccessRow       *adw.ActionRow
	AllowLANAccessSwitch    *gtk.Switch
	AdvertisedRoutesGroup   *adw.PreferencesGroup
	AdvertiseRouteButton    *gtk.Button
	NetCheckGroup           *adw.PreferencesGroup
	NetCheckButton          *gtk.Button
	LastNetCheckRow         *adw.ActionRow
	LastNetCheck            *gtk.Label
	UDPRow                  *adw.ActionRow
	UDP                     *gtk.Image
	IPv4Row                 *adw.ActionRow
	IPv4Icon                *gtk.Image
	IPv4Addr                *gtk.Label
	IPv6Row                 *adw.ActionRow
	IPv6Icon                *gtk.Image
	IPv6Addr                *gtk.Label
	UPnPRow                 *adw.ActionRow
	UPnP                    *gtk.Image
	PMPRow                  *adw.ActionRow
	PMP                     *gtk.Image
	PCPRow                  *adw.ActionRow
	PCP                     *gtk.Image
	HairPinningRow          *adw.ActionRow
	HairPinning             *gtk.Image
	PreferredDERPRow        *adw.ActionRow
	PreferredDERP           *gtk.Label
	DERPLatencies           *adw.ExpanderRow
	MiscGroup               *adw.PreferencesGroup
	ExitNodeRow             *adw.ActionRow
	ExitNodeSwitch          *gtk.Switch
	OnlineRow               *adw.ActionRow
	Online                  *gtk.Image
	LastSeenRow             *adw.ActionRow
	LastSeen                *gtk.Label
	CreatedRow              *adw.ActionRow
	Created                 *gtk.Label
	LastWriteRow            *adw.ActionRow
	LastWrite               *gtk.Label
	LastHandshakeRow        *adw.ActionRow
	LastHandshake           *gtk.Label
	RxBytesRow              *adw.ActionRow
	RxBytes                 *gtk.Label
	TxBytesRow              *adw.ActionRow
	TxBytes                 *gtk.Label
}

var peerPageType = coreglib.RegisterSubclass[*PeerPage](
	coreglib.WithClassInit(func(class *gtk.WidgetClass) {
		class.SetLayoutManagerType(gtk.GTypeBinLayout)
		class.SetTemplate(glib.NewBytesWithGo(peerPageXML))
	}),
)

func NewPeerPage() *PeerPage {
	page := peerPageType.New()
	page.InitTemplate()
	return page
}
