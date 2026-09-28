package trayscale

import (
	"embed"
	"io/fs"
)

//go:embed LICENSE *.metainfo.xml
var assetsFS embed.FS

//go:embed po
var localeFS embed.FS

func Assets() fs.FS {
	return assetsFS
}

// LocaleFS returns the embedded gettext catalogs under po/.
func LocaleFS() fs.FS {
	sub, err := fs.Sub(localeFS, "po")
	if err != nil {
		return localeFS
	}
	return sub
}
