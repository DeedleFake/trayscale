package metadata

import (
	"os"
	"runtime/debug"
)

var version = ""

func Version() (string, bool) {
	if version != "" {
		return version, true
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", false
	}

	return info.Main.Version, true
}

var Private = os.Getenv("TRAYSCALE_PRIVATE") == "1"
