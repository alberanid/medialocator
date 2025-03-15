package config

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/alberanid/medialocator/version"
)

const DEFAULT_PLEX_DB = "com.plexapp.plugins.library.db"

// store command line configuration.
type Config struct {
	Tags    []string
	PlexDb  string
	Verbose bool
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
	flag.StringVar(&tags, "tags", "", "Filter movies with this comma-separated tags")

	flag.StringVar(&c.PlexDb, "plex-db", DEFAULT_PLEX_DB, "Plex database file")

	flag.BoolVar(&c.Verbose, "verbose", false, "be more verbose")
	getVer := flag.Bool("version", false, "print version and quit")

	flag.Parse()

	if *getVer {
		fmt.Printf("version %s\n", version.VERSION)
		os.Exit(0)
	}

	c.Tags = splitAndTrim(tags)

	return &c
}
