package trayscale

import (
	"embed"
	"io/fs"
)

//go:embed LICENSE *.metainfo.xml
var assetsFS embed.FS

func Assets() fs.FS {
	return assetsFS
}
