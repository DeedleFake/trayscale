package tray

import (
	"deedles.dev/trayscale/internal/metadata"
	"tailscale.com/ipn"
)

// ProfileTitle is the name shown for a login profile.
func ProfileTitle(profile ipn.LoginProfile) string {
	if metadata.Private {
		return "profile@example.com"
	}
	return profile.Name
}

func accountMenuLabel(title string) string {
	return title
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
		label := "  " + profile.Name
		if profile.ID == active {
			label = "● " + profile.Name
		}
		items[i] = accountItem{
			id:     profile.ID,
			label:  label,
			active: profile.ID == active,
		}
	}
	return accountMenu{show: true, items: items}
}
