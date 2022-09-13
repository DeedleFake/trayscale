package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
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

func (t Object) NamedChildren() (children []Object) {
	children = t.namedChildren(children)
	return children
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

type Property struct {
	XMLName xml.Name `xml:"property"`

	Name  string `xml:"name,attr"`
	Value Value  `xml:",chardata"`
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

type Func string

func (f Func) Args() Args {
	switch f {
	case "adw.NewApplicationWindow":
		return Args{
			{"app", "*gtk.Application"},
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

type Arg struct {
	Name, Type string
}

type Value struct {
	Val any
}

func (v Value) String() string {
	switch val := v.Val.(type) {
	case int, float64:
		return fmt.Sprint(val)
	case string:
		return fmt.Sprintf("%q", val)
	default:
		panic(fmt.Errorf("unexpected value type (%T): %q", val, val))
	}
}

func (v *Value) UnmarshalText(text []byte) error {
	str := string(text)

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
