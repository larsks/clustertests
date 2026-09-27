package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/pflag"
)

func loadSuites(paths []string) ([]JUnitTestSuite, error) {
	if len(paths) == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("reading stdin: %w", err)
		}
		suites, err := parseReport(data)
		if err != nil {
			return nil, fmt.Errorf("parsing stdin: %w", err)
		}
		return suites, nil
	}

	var all []JUnitTestSuite
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		suites, err := parseReport(data)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}
		all = append(all, suites...)
	}
	return all, nil
}

func run() error {
	var outPath string
	pflag.StringVarP(&outPath, "out", "o", "", "write output to file (default: stdout)")
	pflag.Parse()

	suites, err := loadSuites(pflag.Args())
	if err != nil {
		return err
	}

	out := os.Stdout
	if outPath != "" {
		f, err := os.Create(outPath)
		if err != nil {
			return err
		}
		defer f.Close()
		out = f
	}

	return Render(out, suites)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "junit2html:", err)
		os.Exit(1)
	}
}
