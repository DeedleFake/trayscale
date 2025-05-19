package metadata

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"iter"
	"os"
	"runtime/debug"
	"slices"
	"strings"

	"deedles.dev/trayscale"
)

const AppID = "dev.deedles.Trayscale"

var Private = os.Getenv("TRAYSCALE_PRIVATE") == "1"

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

func License() string {
	return readAssetString("LICENSE")
}

func ReleaseNotes() (string, string) {
	file := must(trayscale.Assets().Open(AppID + ".metainfo.xml"))
	defer file.Close()

	var release string
	var description bool
	var buf strings.Builder
loop:
	for t, err := range tokens(xml.NewDecoder(file)) {
		must(t, err)

		switch t := t.(type) {
		case xml.StartElement:
			if t.Name.Local == "release" {
				i := slices.IndexFunc(t.Attr, func(attr xml.Attr) bool { return attr.Name.Local == "version" })
				release = t.Attr[i].Value
				continue
			}
			if t.Name.Local == "description" {
				description = true
				continue
			}
		case xml.EndElement:
			if release != "" && t.Name.Local == "description" {
				break loop
			}
		}

		if release != "" && description {
			writeToken(&buf, t)
		}
	}

	return release, buf.String()
}

// must returns v if err is nil. If err is not nil, it panics with
// err's value.
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// readAssetString returns the contents of the given embedded asset as
// a string. It panics if there are any errors.
func readAssetString(file string) string {
	var str strings.Builder
	f := must(trayscale.Assets().Open(file))
	defer f.Close()
	must(io.Copy(&str, f))
	return str.String()
}

func tokens(d *xml.Decoder) iter.Seq2[xml.Token, error] {
	return func(yield func(xml.Token, error) bool) {
		for {
			t, err := d.Token()
			if err == io.EOF || !yield(t, err) {
				return
			}
		}
	}
}

func writeToken(w io.Writer, t xml.Token) {
	switch t := t.(type) {
	case xml.StartElement:
		fmt.Fprintf(w, "<%v", t.Name.Local)
		for _, attr := range t.Attr {
			fmt.Fprintf(w, " %v=%q", attr.Name.Local, attr.Value)
		}
		w.Write([]byte{'>'})

	case xml.EndElement:
		fmt.Fprintf(w, "</%v>", t.Name.Local)

	case xml.CharData:
		if len(bytes.TrimSpace(t)) == 0 {
			break
		}
		w.Write(t)

	default:
		// Ignore everything else.
	}
}
