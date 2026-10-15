package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/cosmofactory/recetas-backend/internal/catalog/importer"
)

// Exit codes.
const (
	exitOK         = 0
	exitInvalid    = 1 // usage or bundle validation error
	exitDatabase   = 2
	maxSourceBytes = 200
)

type stdio struct {
	in       io.Reader
	out, err io.Writer
}

type options struct {
	file   string
	source string
	dryRun bool
}

// run imports one bundle and returns the exit code. It reaches db only once
// the bundle is valid.
func run(ctx context.Context, args []string, std stdio, db importer.Beginner) int {
	opts, err := parseFlags(args, std.err)
	if errors.Is(err, flag.ErrHelp) {
		return exitOK
	}
	if err != nil {
		say(std.err, "import: %v\n", err)
		return exitInvalid
	}
	b, err := readBundle(opts.file, std.in)
	if err != nil {
		printError(std.err, err)
		return exitInvalid
	}
	s, err := importer.Apply(ctx, db, b, importer.Options{Source: opts.source, DryRun: opts.dryRun})
	if err != nil {
		printError(std.err, err)
		var verr *importer.ValidationError
		if errors.As(err, &verr) {
			return exitInvalid
		}
		return exitDatabase
	}
	printSummary(std.out, s, opts)
	return exitOK
}

func parseFlags(args []string, stderr io.Writer) (options, error) {
	var opts options
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&opts.file, "file", "", "bundle file (default: read stdin)")
	fs.StringVar(&opts.source, "source", "", "label stored with the content version (required)")
	fs.BoolVar(&opts.dryRun, "dry-run", false, "print the changes, then roll them back")
	fs.Usage = func() {
		say(stderr, "usage: import --source <label> [--file bundle.json] [--dry-run]  (version %s)\n", version)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	opts.source = strings.TrimSpace(opts.source)
	switch {
	case fs.NArg() > 0:
		return options{}, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	case opts.source == "":
		return options{}, errors.New("--source is required")
	case len(opts.source) > maxSourceBytes:
		return options{}, fmt.Errorf("--source is longer than %d bytes", maxSourceBytes)
	}
	return opts, nil
}

// readBundle parses and validates the bundle in file, or on stdin without one.
func readBundle(file string, stdin io.Reader) (importer.Bundle, error) {
	r := stdin
	if file != "" {
		f, err := os.Open(filepath.Clean(file))
		if err != nil {
			return importer.Bundle{}, err
		}
		defer func() { _ = f.Close() }()
		r = f
	}
	b, err := importer.Parse(r)
	if err != nil {
		return importer.Bundle{}, err
	}
	return b, importer.Validate(b)
}

func printError(w io.Writer, err error) {
	var verr *importer.ValidationError
	if !errors.As(err, &verr) {
		say(w, "import: %v\n", err)
		return
	}
	say(w, "import: invalid bundle, %d problem(s):\n", len(verr.Problems))
	for _, p := range verr.Problems {
		say(w, "  %s\n", p)
	}
}

func printSummary(w io.Writer, s importer.Summary, opts options) {
	switch {
	case s.NoChanges && opts.dryRun:
		say(w, "dry run: no changes (content version %d is current)\n", s.Version)
		return
	case s.NoChanges:
		say(w, "no changes (content version %d is current)\n", s.Version)
		return
	case opts.dryRun:
		say(w, "dry run, rolled back; changes against content version %d:\n", s.Version)
	default:
		say(w, "imported content version %d (source %q):\n", s.Version, opts.source)
	}
	for _, e := range importer.Entities {
		c := s.Changes[e]
		say(w, "  %-10s  created %3d  updated %3d  deleted %3d\n", e, c.Created, c.Updated, c.Deleted)
	}
}

// say writes CLI output; a failed write to stdout or stderr has nowhere to be
// reported.
func say(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}
