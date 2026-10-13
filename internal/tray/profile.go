package tray

import (
	"strings"

	"deedles.dev/trayscale/internal/metadata"
	"tailscale.com/ipn"
)

func ProfileTitle(profile ipn.LoginProfile) string {
	if metadata.Private {
		return "profile@example.com"
	}
	name := profile.Name
	tailnet := ""
	if profile.NetworkProfile.DomainName != "" {
		tailnet = profile.NetworkProfile.DisplayNameOrDefault()
	}
	if tailnet == "" || strings.EqualFold(name, tailnet) {
		return name
	}
	return name + " (" + tailnet + ")"
}

// accountMenuLabel escapes GTK mnemonic markers in a profile title.
func accountMenuLabel(title string) string {
	return strings.ReplaceAll(title, "_", "__")
}

type accountMenu struct {
	show  bool
	items []accountItem
}

type accountItem struct {
	id     ipn.ProfileID
	label  string
	active bool
}

func planAccountMenu(profiles []ipn.LoginProfile, active ipn.ProfileID) accountMenu {
	if len(profiles) <= 1 {
		return accountMenu{}
	}
	items := make([]accountItem, len(profiles))
	for i, profile := range profiles {
		items[i] = accountItem{
			id:     profile.ID,
			label:  accountMenuLabel(ProfileTitle(profile)),
			active: profile.ID == active,
		}
	}
	return accountMenu{show: true, items: items}
}
