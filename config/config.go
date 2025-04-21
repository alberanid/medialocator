package config

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/alberanid/medialocator/version"
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

// parse command line arguments.
func ParseArgs() *Config {
	c := Config{}
	tags := ""
	libraries := ""
	flag.StringVar(&tags, "tags", "", "Filter movies with this comma-separated tags")
	flag.StringVar(&libraries, "libraries", "", "Filter by comma-separated library names")
	flag.StringVar(&c.PlexDb, "plex-db", DEFAULT_PLEX_DB, "Plex database file")
	flag.StringVar(&c.AddPrefix, "add-prefix", "", "Add this prefix to the file paths")
	flag.StringVar(&c.StripPrefix, "strip-prefix", "", "Remove this prefix from the file paths")
	flag.StringVar(&c.OutputFile, "output-file", "", "Write output to this file")
	flag.BoolVar(&c.Verbose, "verbose", false, "be more verbose")
	flag.BoolVar(&c.ListAll, "list-all", false, "List all media_parts without filtering by tags (includes all libraries)")
	flag.BoolVar(&c.NoTags, "no-tags", false, "Filter media items with no tags associated")
	getVer := flag.Bool("version", false, "print version and quit")

	flag.Parse()

	if *getVer {
		fmt.Printf("version %s\n", version.VERSION)
		os.Exit(0)
	}

	c.Tags = splitAndTrim(tags)
	c.Libraries = splitAndTrim(libraries)

	if c.ListAll && len(c.Tags) != 0 {
		slog.Error("-list-all and -tags are mutually exclusive")
		os.Exit(1)
	}
	if c.NoTags && len(c.Tags) != 0 {
		slog.Error("-no-tags and -tags are mutually exclusive")
		os.Exit(1)
	}
	if c.NoTags && c.ListAll {
		slog.Error("-no-tags and -list-all are mutually exclusive")
		os.Exit(1)
	}
	if len(c.Tags) == 0 && !c.NoTags && !c.ListAll {
		slog.Error("no tags specified, use -tags or -no-tags or -list-all")
		os.Exit(1)
	}

	if c.Verbose {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}
	return &c
}
