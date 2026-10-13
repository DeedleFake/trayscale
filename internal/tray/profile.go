package tray

import (
	"strings"

	"deedles.dev/trayscale/internal/metadata"
	"tailscale.com/ipn"
)

func profileName(profile ipn.LoginProfile) string {
	if metadata.Private {
		return "profile@example.com"
	}
	return profile.Name
}

// accountMenuLabel escapes dbusmenu mnemonic markers in a profile title.
func accountMenuLabel(title string) string {
	return strings.ReplaceAll(title, "_", "__")
}
