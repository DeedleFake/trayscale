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
