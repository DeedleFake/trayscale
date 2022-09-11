package main

import "encoding/xml"

type UIDef struct {
	XMLName xml.Name `xml:"interface"`

	Requires []UIDefRequires
	Objects  []UIDefObject
	Menus    []UIDefMenu
}

type UIDefRequires struct {
	XMLName xml.Name `xml:"requires"`

	Lib     string `xml:"lib,attr"`
	Version string `xml:"version,attr"`
}

type UIDefObject struct {
	XMLName xml.Name `xml:"object"`

	Class string `xml:"class,attr"`
	ID    string `xml:"id,attr"`

	Properties []UIDefProperty
	Children   []UIDefChild
}

type UIDefProperty struct {
	XMLName xml.Name `xml:"property"`

	Name  string `xml:"name,attr"`
	Value string `xml:",chardata"`
}

type UIDefChild struct {
	XMLName xml.Name `xml:"child"`

	Type string `xml:"type,attr"`

	Object UIDefObject
}

type UIDefMenu struct {
	XMLName xml.Name `xml:"menu"`

	ID string `xml:"id,attr"`
}
