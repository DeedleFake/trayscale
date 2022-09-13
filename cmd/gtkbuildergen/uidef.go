package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type Interface struct {
	XMLName xml.Name `xml:"interface"`

	Requires  []Requires `xml:"requires"`
	Templates []Object   `xml:"template"`
	Objects   []Object   `xml:"object"`
	Menus     []Menu     `xml:"menu"`
}

func LoadInterface(r io.Reader) (Interface, error) {
	d := xml.NewDecoder(r)

	var data Interface
	err := d.Decode(&data)
	if err != nil {
		return data, fmt.Errorf("decode UI definition: %w", err)
	}
	return data, nil
}

type Requires struct {
	XMLName xml.Name `xml:"requires"`

	Lib     string `xml:"lib,attr"`
	Version string `xml:"version,attr"`
}

func (req Requires) Import() string {
	switch req.Lib {
	case "gtk":
		return "github.com/diamondburned/gotk4/pkg/gtk/v" + req.Version[:1]
	case "libadwaita":
		return "github.com/diamondburned/gotk4-adwaita/pkg/adw"
	default:
		return req.Lib + "/" + req.Version
	}
}

type Object struct {
	XMLName xml.Name

	Class  Class  `xml:"class,attr"`
	Parent Class  `xml:"parent,attr"`
	ID     string `xml:"id,attr"`

	Properties []Property `xml:"property"`
	Children   []Child    `xml:"child"`
}

func (obj Object) Type() Class {
	if obj.Parent != "" {
		return obj.Parent
	}
	return obj.Class
}

func (t Object) NamedChildren() (children []Object) {
	return t.namedChildren(children)
}

func (t Object) namedChildren(children []Object) []Object {
	for _, c := range t.Children {
		if c.Object.ID != "" {
			children = append(children, c.Object)
		}
		children = c.Object.namedChildren(children)
	}

	return children
}

func (t Object) Descendants() (desc []Object) {
	return t.descendants("parent", desc)
}

func (t Object) descendants(base string, desc []Object) []Object {
	for i, c := range t.Children {
		if c.Object.ID == "" {
			c.Object.ID = fmt.Sprintf("%v%v", base, i)
		}
		desc = append(desc, c.Object)
		desc = c.Object.descendants(c.Object.ID, desc)
	}
	return desc
}

type Property struct {
	XMLName xml.Name `xml:"property"`

	Name  string `xml:"name,attr"`
	Value Value  `xml:",chardata"`
}

func (p Property) WantsWidget() bool {
	return (p.Name == "content") || (p.Name == "stack")
}

type Child struct {
	XMLName xml.Name `xml:"child"`

	Type string `xml:"type,attr"`

	Object Object `xml:"object"`
}

type Menu struct {
	XMLName xml.Name `xml:"menu"`

	ID string `xml:"id,attr"`
}

type Class string

func (class Class) parts() (pkg, short string) {
	switch {
	case strings.HasPrefix(string(class), "Gtk"):
		return "gtk.", string(class)[3:]
	case strings.HasPrefix(string(class), "Adw"):
		return "adw.", string(class)[3:]
	default:
		return "", string(class)
	}
}

func (class Class) Short() string {
	_, short := class.parts()
	return short
}

func (class Class) Convert() string {
	pkg, short := class.parts()
	return pkg + short
}

func (class Class) Constructor() Func {
	pkg, short := class.parts()
	return Func(pkg + "New" + short)
}

func (class Class) AddChild(t, name string) string {
	switch class {
	case "AdwHeaderBar":
		switch t {
		case "title":
			return fmt.Sprintf("SetTitleWidget(%v)", name)
		case "start":
			return fmt.Sprintf("PackStart(%v)", name)
		case "end":
			return fmt.Sprintf("PackEnd(%v)", name)
		}

	case "AdwToastOverlay", "AdwClamp", "AdwStatusPage":
		return fmt.Sprintf("SetChild(%v)", name)

	case "AdwApplicationWindow":
		return fmt.Sprintf("SetContent(%v)", name)

	case "GtkBox", "AdwLeaflet":
		return fmt.Sprintf("Append(%v)", name)

	case "AdwActionRow":
		switch t {
		case "prefix":
			return fmt.Sprintf("AddPrefix(%v)", name)
		default:
			return fmt.Sprintf("AddSuffix(%v)", name)
		}

	case "AdwPreferencesGroup":
		return fmt.Sprintf("Add(%v)", name)
	}

	panic(fmt.Errorf("unexpected class and child type combination: %q -> %q", class, t))
}

type Func string

func (f Func) Args() Args {
	switch f {
	case "adw.NewApplicationWindow":
		return Args{
			{"app", "*gtk.Application"},
		}
	case "gtk.NewBox":
		return Args{
			{"orientation", "gtk.Orientation"},
			{"spacing", "int"},
		}
	case "gtk.NewLabel":
		return Args{
			{"text", "string"},
		}
	default:
		return nil
	}
}

type Args []Arg

func (args Args) String() string {
	names := make([]string, 0, len(args))
	for _, arg := range args {
		names = append(names, arg.Name)
	}
	return strings.Join(names, ", ")
}

func (args Args) WithTypes() string {
	defs := make([]string, 0, len(args))
	for _, arg := range args {
		defs = append(defs, arg.Name+" "+arg.Type)
	}
	return strings.Join(defs, ", ")
}

func (args Args) Defaults() string {
	defs := make([]string, 0, len(args))
	for _, arg := range args {
		defs = append(defs, arg.Default())
	}
	return strings.Join(defs, ", ")
}

type Arg struct {
	Name, Type string
}

func (arg Arg) Default() string {
	switch arg.Type {
	case "gtk.Orientation", "int":
		return "0"
	case "string":
		return "\"\""
	default:
		panic(fmt.Errorf("unexpected arg type %q: %q", arg.Type, arg.Name))
	}
}

// TODO: Parse values lazily with more context instead of guessing as
// this currently does.
type Value struct {
	Val any
}

func (v Value) String() string {
	switch val := v.Val.(type) {
	case int, float64, bool:
		return fmt.Sprint(val)
	case string:
		return fmt.Sprintf("%q", val)
	case gtk.Orientation:
		return fmt.Sprint(int(val))
	case gtk.StackTransitionType:
		return fmt.Sprint(int(val))
	default:
		panic(fmt.Errorf("unexpected value type (%T): %q", val, val))
	}
}

func (v *Value) UnmarshalText(text []byte) error {
	str := string(text)

	switch str {
	case "true", "True":
		v.Val = true
		return nil
	case "false", "False":
		v.Val = false
		return nil
	case "horizontal":
		v.Val = gtk.OrientationHorizontal
		return nil
	case "vertical":
		v.Val = gtk.OrientationVertical
		return nil
	case "slide-up-down":
		v.Val = gtk.StackTransitionTypeSlideUpDown
		return nil
	}

	i, err := strconv.ParseInt(str, 10, 0)
	if err == nil {
		v.Val = int(i)
		return nil
	}

	f, err := strconv.ParseFloat(str, 64)
	if err == nil {
		v.Val = f
		return nil
	}

	v.Val = str
	return nil
}
