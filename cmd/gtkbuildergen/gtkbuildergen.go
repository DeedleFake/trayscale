package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"io"
	"os"
)

type Generator struct {
	ui []Interface
}

func (gen *Generator) Generate(w io.Writer, pkg string) error {
	return tmpl.ExecuteTemplate(w, "output.tmpl", map[string]any{
		"Package": pkg,
		"UI":      gen.ui,
	})
}

func (gen *Generator) Add(data Interface) {
	gen.ui = append(gen.ui, data)
}

func loadFile(gen *Generator, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer file.Close()

	ui, err := LoadInterface(file)
	if err != nil {
		return fmt.Errorf("load: %w", err)
	}

	gen.Add(ui)

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

	var buf bytes.Buffer
	err := gen.Generate(&buf, *pkg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: write output file: %v\n", err)
		os.Exit(1)
	}

	fbuf, err := format.Source(buf.Bytes())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: gofmt output: %v\n", err)
		os.Exit(1)
	}

	err = os.WriteFile(*out, fbuf, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: write output file: %v\n", err)
		os.Exit(1)
	}
}
