package main

import (
	"embed"
	_ "embed"
	"text/template"
)

var (
	//go:embed *.tmpl
	tmplFS embed.FS

	tmpl *template.Template
)

func init() {
	tmpl = template.New("")
	tmpl.Funcs(map[string]any{
		"requires": func(ui []Interface) []Requires {
			// TODO: Return better values.
			return ui[0].Requires
		},
		"initializer": func() *Initializer {
			return new(Initializer)
		},
	})
	tmpl = template.Must(tmpl.ParseFS(tmplFS, "*.tmpl"))
}
