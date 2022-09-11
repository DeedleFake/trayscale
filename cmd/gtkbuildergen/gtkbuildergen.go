package main

import (
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"os"
)

type Generator struct{}

func (gen *Generator) Generate(w io.Writer, pkg string) error {
	panic("Not implemented.")
}

func (gen *Generator) Load(r io.Reader) error {
	d := xml.NewDecoder(r)

	var data UIDef
	err := d.Decode(&data)
	if err != nil {
		return fmt.Errorf("decode UI definition: %w", err)
	}

	return gen.Add(data)
}

func (gen *Generator) Add(data UIDef) error {
	panic("Not implemented.")
}

func loadFile(gen *Generator, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer file.Close()

	err = gen.Load(file)
	if err != nil {
		return fmt.Errorf("load: %w", err)
	}

	return nil
}

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: gtkbuildergen [options] <input files...>")
		flag.PrintDefaults()
	}
	out := flag.String("out", "", "output file (default is first input with .go attached)")
	pkg := flag.String("pkg", "main", "output Go package name")
	flag.Parse()

	in := flag.Args()
	if len(in) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no input files provided\n\n")
		flag.Usage()
		os.Exit(2)
	}

	if *out == "" {
		*out = in[0] + ".go"
	}

	var gen Generator
	for _, file := range in {
		err := loadFile(&gen, file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: open input file: %v\n", err)
			os.Exit(1)
		}
	}

	outf, err := os.Create(*out)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: create output file: %v\n", err)
		os.Exit(1)
	}
	defer outf.Close()

	err = gen.Generate(outf, *pkg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: write output file: %v\n", err)
		os.Exit(1)
	}
}
