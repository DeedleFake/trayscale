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

// accountMenuLabel escapes dbusmenu mnemonic markers in a profile title.
func accountMenuLabel(title string) string {
	return strings.ReplaceAll(title, "_", "__")
}
