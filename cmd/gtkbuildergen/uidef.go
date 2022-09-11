package main

import "encoding/xml"

type UIDef struct {
	XMLName  xml.Name `xml:"interface"`
	Requires []UIDefRequires
	Objects  []UIDefObject
}

type UIDefRequires struct {
	XMLName xml.Name `xml:"requires"`
	Lib     string   `xml:"lib,attr"`
	Version string   `xml:"version,attr"`
}

type UIDefObject struct {
	XMLName xml.Name `xml:"object"`
	Class   string   `xml:"class,attr"`
	ID      string   `xml:"id,attr"`
}
