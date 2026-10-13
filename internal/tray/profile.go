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

// escapeMnemonics escapes underscores in s so that dbusmenu doesn't treat them as mnemonic markers.
func escapeMnemonics(s string) string {
	return strings.ReplaceAll(s, "_", "__")
}
