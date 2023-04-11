package main

import (
	"embed"
	"text/template"

	"deedles.dev/trayscale/internal/set"
)

var (
	//go:embed *.tmpl
	tmplFS embed.FS

	tmpl *template.Template
)

func init() {
	tmpl = template.New("")
	tmpl.Funcs(map[string]any{
		"requires": func(ui []Interface) (r []string) {
			for _, i := range ui {
				for _, req := range i.Requires {
					r = append(r, req.Import())
				}
			}

			for _, i := range ui {
				if len(i.Menus) != 0 {
					r = append(r, "github.com/diamondburned/gotk4/pkg/gio/v2")
					break
				}
			}

		outer:
			for _, i := range ui {
				for _, t := range i.Templates {
					if _, ok := findDeepProperty(t, "actions"); ok {
						r = append(r, "github.com/diamondburned/gotk4/pkg/gdk/v4")
						break outer
					}
				}
				for _, obj := range i.Objects {
					if _, ok := findDeepProperty(obj, "actions"); ok {
						r = append(r, "github.com/diamondburned/gotk4/pkg/gdk/v4")
						break outer
					}
				}
			}

			return r
		},
		"newValueSet": func() set.Set[string] {
			return make(set.Set[string])
		},
	})
	tmpl = template.Must(tmpl.ParseFS(tmplFS, "*.tmpl"))
}

func findDeepProperty(obj Object, name string) (Property, bool) {
	if p := obj.FindProperty(name); p != (Property{}) {
		return p, true
	}

	for _, c := range obj.Children {
		if p, ok := findDeepProperty(c.Object, name); ok {
			return p, ok
		}
	}

	return Property{}, false
}
