package ui

import (
	"testing"

	"github.com/diamondburned/gotk4/pkg/gio/v2"
)

func TestOnlinePeerIconBundled(t *testing.T) {
	// Breeze doesn't ship this icon, so Trayscale bundles a fallback.
	name := peerIconName(true, false, false)
	path := "/dev/deedles/Trayscale/icons/scalable/status/" + name + ".svg"
	if _, err := gio.ResourcesLookupData(path, gio.ResourceLookupFlagsNone); err != nil {
		t.Fatalf("icon %q is not bundled: %v", name, err)
	}
}
