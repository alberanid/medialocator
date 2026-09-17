package config

import (
	"errors"
	"flag"
	"io"
	"strings"
)

const DEFAULT_PLEX_DB = "/var/lib/plexmediaserver/Library/Application Support/Plex Media Server/Plug-in Support/Databases/com.plexapp.plugins.library.db"

// store command line configuration.
type Config struct {
	Tags        []string
	PlexDb      string
	AddPrefix   string
	StripPrefix string
	OutputFile  string
	Verbose     bool
	ListAll     bool
	NoTags      bool
	Libraries   []string
	ShowVersion bool
}

// Split and trim comma-separated values
func splitAndTrim(s string) []string {
	pieces := []string{}
	parts := strings.Split(s, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		pieces = append(pieces, part)
	}
	return pieces
}

// Parse parses the command line arguments without terminating the process.
// Flag usage and parse failures are written to output; the returned error is
// flag.ErrHelp when help was requested.
func Parse(args []string, output io.Writer) (*Config, error) {
	c := Config{}
	fs := flag.NewFlagSet("medialocator", flag.ContinueOnError)
	fs.SetOutput(output)
	tags := ""
	libraries := ""
	fs.StringVar(&tags, "tags", "", "Filter movies with this comma-separated tags")
	fs.StringVar(&libraries, "libraries", "", "Filter by comma-separated library names")
	fs.StringVar(&c.PlexDb, "plex-db", DEFAULT_PLEX_DB, "Plex database file")
	fs.StringVar(&c.AddPrefix, "add-prefix", "", "Add this prefix to the file paths")
	fs.StringVar(&c.StripPrefix, "strip-prefix", "", "Remove this prefix from the file paths")
	fs.StringVar(&c.OutputFile, "output-file", "", "Write output to this file")
	fs.BoolVar(&c.Verbose, "verbose", false, "be more verbose")
	fs.BoolVar(&c.ListAll, "list-all", false, "List all media_parts without filtering by tags (includes all libraries)")
	fs.BoolVar(&c.NoTags, "no-tags", false, "Filter media items with no tags associated")
	fs.BoolVar(&c.ShowVersion, "version", false, "print version and quit")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	if c.ShowVersion {
		return &c, nil
	}

	c.Tags = splitAndTrim(tags)
	c.Libraries = splitAndTrim(libraries)

	if c.ListAll && len(c.Tags) != 0 {
		return nil, errors.New("-list-all and -tags are mutually exclusive")
	}
	if c.NoTags && len(c.Tags) != 0 {
		return nil, errors.New("-no-tags and -tags are mutually exclusive")
	}
	if c.NoTags && c.ListAll {
		return nil, errors.New("-no-tags and -list-all are mutually exclusive")
	}
	if len(c.Tags) == 0 && !c.NoTags && !c.ListAll {
		return nil, errors.New("no tags specified, use -tags or -no-tags or -list-all")
	}

	return &c, nil
}
