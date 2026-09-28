package trayscale

import (
	"embed"
	"io/fs"
)

//go:embed LICENSE *.metainfo.xml po
var assetsFS embed.FS

func Assets() fs.FS {
	return assetsFS
}

// LocaleFS returns the embedded gettext catalogs under po/.
func LocaleFS() fs.FS {
	sub, err := fs.Sub(assetsFS, "po")
	if err != nil {
		return assetsFS
	}
	return sub
}
