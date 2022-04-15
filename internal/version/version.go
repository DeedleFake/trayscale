package version

import "runtime/debug"

var version = ""

func Get() (string, bool) {
	if version != "" {
		return version, true
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", false
	}

	return info.Main.Version, true
}
