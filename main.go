package main

// import sqlite3 driver
import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/alberanid/medialocator/config"
	"github.com/alberanid/medialocator/version"
	_ "github.com/mattn/go-sqlite3"
)

// readOnlyDSN builds the SQLite URI used to open the Plex database.
// The DSN must start with "file:" for the go-sqlite3 driver to keep the query
// parameters: a bare path is opened with SQLITE_OPEN_READWRITE|SQLITE_OPEN_CREATE
// and its parameters are dropped. mode=ro makes SQLite reject writes, and
// _query_only=true sets PRAGMA query_only as defense in depth.
func readOnlyDSN(dbPath string) string {
	u := url.URL{
		Scheme:   "file",
		OmitHost: true,
		Path:     dbPath,
		RawQuery: "mode=ro&_query_only=true",
	}
	return u.String()
}

// ensureDistinctOutput refuses an output path that resolves to the database
// being read, including symlink and hard-link aliases, so that a run can never
// overwrite the source database.
func ensureDistinctOutput(dbPath, outputPath string) error {
	dbInfo, err := os.Stat(dbPath)
	if err != nil {
		return err
	}
	outInfo, err := os.Stat(outputPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if os.SameFile(dbInfo, outInfo) {
		return errors.New("output file is the same file as the database")
	}
	return nil
}

// validateDatabase forces a first read of the database so that a missing,
// unreadable or non-SQLite file is reported before any selection work begins.
func validateDatabase(db *sql.DB) error {
	var schemaVersion int
	return db.QueryRow("PRAGMA schema_version").Scan(&schemaVersion)
}

// writeLines writes one path per line and reports the first write failure.
func writeLines(w io.Writer, parts []string) error {
	for _, part := range parts {
		if _, err := fmt.Fprintf(w, "%s\n", part); err != nil {
			return err
		}
	}
	return nil
}

// writeOutputAtomically writes the lines to a temporary file in the
// destination directory and renames it over outputPath, so an interrupted or
// failing run leaves any pre-existing output file untouched.
func writeOutputAtomically(outputPath string, parts []string) error {
	tmp, err := os.CreateTemp(filepath.Dir(outputPath), ".medialocator-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if tmpName != "" {
			os.Remove(tmpName)
		}
	}()
	// Keep the permissions of an existing output file, since the rename
	// replaces the inode that carried them.
	mode := os.FileMode(0644)
	if info, err := os.Stat(outputPath); err == nil {
		mode = info.Mode().Perm()
	}
	if err := writeLines(tmp, parts); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	if err := os.Rename(tmpName, outputPath); err != nil {
		return err
	}
	tmpName = ""
	return nil
}

// helper to generate SQL IN clause and args for librarySectionIDs
func librarySectionFilter(field string, ids []int) (string, []interface{}) {
	if len(ids) == 0 {
		return "", nil
	}
	placeholders := strings.Repeat(",?", len(ids)-1)
	clause := fmt.Sprintf("%s IN (?%s)", field, placeholders)
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return clause, args
}

// helper to generate SQL IN clause and args for string values
func stringInClause(field string, values []string) (string, []interface{}) {
	if len(values) == 0 {
		return "", nil
	}
	placeholders := strings.Repeat(",?", len(values)-1)
	clause := fmt.Sprintf("%s IN (?%s)", field, placeholders)
	args := make([]interface{}, len(values))
	for i, v := range values {
		args[i] = v
	}
	return clause, args
}

// getLibrarySectionIDs returns a list of library_section ids for the given library names
func getLibrarySectionIDs(db *sql.DB, names []string) ([]int, error) {
	if len(names) == 0 {
		return nil, nil
	}
	clause, args := stringInClause("name", names)
	query := fmt.Sprintf("SELECT id FROM library_sections WHERE %s", clause)
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

// plexLabelTagType is the tags.tag_type of Plex labels, the category that -tags
// values name. Other categories (countries, genres, collections, ...) reuse the
// same tag text, so the label lookup must be restricted to this type.
const plexLabelTagType = 11

// labelClosureCTE expands the labels given to -tags to the metadata_items they
// tag plus every descendant of those items, so a tagged show or season also
// selects its episodes.
const labelClosureCTE = `
WITH RECURSIVE tagged(id) AS (
    SELECT tg.metadata_item_id
      FROM taggings tg
      JOIN tags t ON t.id = tg.tag_id
     WHERE t.tag_type = ? AND t.tag IN (?%s)
  UNION
    SELECT child.id
      FROM metadata_items child
      JOIN tagged parent ON child.parent_id = parent.id
)`

// anyTagClosureCTE expands every tagging to the tagged metadata_items plus all
// of their descendants.
const anyTagClosureCTE = `
WITH RECURSIVE tagged(id) AS (
    SELECT metadata_item_id FROM taggings
  UNION
    SELECT child.id
      FROM metadata_items child
      JOIN tagged parent ON child.parent_id = parent.id
)`

// queryMediaPaths runs a query returning media_parts.file values and returns
// them untrimmed, so that meaningful whitespace in a filename is preserved.
func queryMediaPaths(db *sql.DB, query string, args []interface{}) ([]string, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	parts := []string{}
	for rows.Next() {
		var part string
		if err := rows.Scan(&part); err != nil {
			return nil, err
		}
		parts = append(parts, part)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return parts, nil
}

// mediaPathConditions returns the WHERE conditions shared by every selection
// mode: a nonblank media_parts.file, optionally restricted to libraries.
func mediaPathConditions(librarySectionIDs []int) (string, []interface{}) {
	where := "TRIM(mp.file) <> ''"
	clause, args := librarySectionFilter("mi.library_section_id", librarySectionIDs)
	if clause != "" {
		where += " AND " + clause
	}
	return where, args
}

// taggedMediaParts returns the media parts of every media item whose metadata
// is tagged with one of the labels, or descends from a metadata item that is.
func taggedMediaParts(db *sql.DB, labels []string, librarySectionIDs []int) ([]string, error) {
	if len(labels) == 0 {
		return nil, nil
	}
	args := []interface{}{plexLabelTagType}
	for _, label := range labels {
		args = append(args, label)
	}
	where, libArgs := mediaPathConditions(librarySectionIDs)
	args = append(args, libArgs...)
	query := fmt.Sprintf(labelClosureCTE, strings.Repeat(",?", len(labels)-1)) + `
SELECT mp.file
  FROM tagged
  JOIN media_items mi ON mi.metadata_item_id = tagged.id
  JOIN media_parts mp ON mp.media_item_id = mi.id
 WHERE ` + where
	return queryMediaPaths(db, query, args)
}

// untaggedMediaParts returns the media parts of every media item whose metadata
// carries no tag at all, neither directly nor through an ancestor. It uses the
// same inherited closure as taggedMediaParts so the two modes partition the
// library consistently.
func untaggedMediaParts(db *sql.DB, librarySectionIDs []int) ([]string, error) {
	where, args := mediaPathConditions(librarySectionIDs)
	where += " AND NOT EXISTS (SELECT 1 FROM tagged WHERE tagged.id = mi.metadata_item_id)"
	query := anyTagClosureCTE + `
SELECT mp.file
  FROM media_items mi
  JOIN media_parts mp ON mp.media_item_id = mi.id
 WHERE ` + where
	return queryMediaPaths(db, query, args)
}

// allMediaParts returns every media part file, optionally restricted to
// libraries. The join is only needed to apply the library filter.
func allMediaParts(db *sql.DB, librarySectionIDs []int) ([]string, error) {
	where, args := mediaPathConditions(librarySectionIDs)
	from := "media_parts mp"
	if len(librarySectionIDs) > 0 {
		from = "media_parts mp JOIN media_items mi ON mp.media_item_id = mi.id"
	}
	return queryMediaPaths(db, "SELECT mp.file FROM "+from+" WHERE "+where, args)
}

// deduplicate a list of strings
func dedupStrings(s []string) []string {
	m := make(map[string]bool)
	for _, item := range s {
		m[item] = true
	}
	result := []string{}
	for item := range m {
		result = append(result, item)
	}
	slices.Sort(result)
	return result
}

// errOutput marks failures that produce the output stream or file; main exits
// with status 2 for them and with status 1 for database or configuration
// failures.
var errOutput = errors.New("output error")

// run executes a single medialocator invocation, writing the media list to
// stdout or to -output-file and returning any failure instead of terminating
// the process.
func run(args []string, stdout, stderr io.Writer) error {
	cfg, err := config.Parse(args, stderr)
	if err != nil {
		return err
	}
	if cfg.ShowVersion {
		fmt.Fprintf(stdout, "version %s\n", version.VERSION)
		return nil
	}
	if cfg.Verbose {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	if _, err := os.Stat(cfg.PlexDb); err != nil {
		return fmt.Errorf("cannot use database %s: %s", cfg.PlexDb, err)
	}

	if cfg.OutputFile != "" {
		if err := ensureDistinctOutput(cfg.PlexDb, cfg.OutputFile); err != nil {
			return fmt.Errorf("%w: refusing output file %s: %s", errOutput, cfg.OutputFile, err)
		}
	}

	db, err := sql.Open("sqlite3", readOnlyDSN(cfg.PlexDb))
	if err != nil {
		return fmt.Errorf("error opening database %s: %s", cfg.PlexDb, err)
	}
	defer db.Close()

	if err := validateDatabase(db); err != nil {
		return fmt.Errorf("error reading database %s: %s", cfg.PlexDb, err)
	}

	librarySectionIDs := []int{}
	if len(cfg.Libraries) > 0 {
		ids, err := getLibrarySectionIDs(db, cfg.Libraries)
		if err != nil {
			return fmt.Errorf("error getting library section ids: %s", err)
		}
		if len(ids) == 0 {
			return errors.New("no matching libraries found for -libraries argument")
		}
		librarySectionIDs = ids
	}

	parts := []string{}
	switch {
	case cfg.ListAll:
		parts, err = allMediaParts(db, librarySectionIDs)
	case cfg.NoTags:
		parts, err = untaggedMediaParts(db, librarySectionIDs)
	default:
		parts, err = taggedMediaParts(db, cfg.Tags, librarySectionIDs)
	}
	if err != nil {
		return fmt.Errorf("error selecting media parts: %s", err)
	}
	slog.Debug(fmt.Sprintf("selected %d media parts", len(parts)))

	parts = dedupStrings(parts)
	for idx, part := range parts {
		if cfg.StripPrefix != "" {
			part = strings.TrimPrefix(part, cfg.StripPrefix)
		}
		if cfg.AddPrefix != "" {
			part = path.Join(cfg.AddPrefix, part)
		}
		parts[idx] = part
	}

	if cfg.OutputFile == "" {
		if err := writeLines(stdout, parts); err != nil {
			return fmt.Errorf("%w: error writing to standard output: %s", errOutput, err)
		}
		return nil
	}

	if err := writeOutputAtomically(cfg.OutputFile, parts); err != nil {
		return fmt.Errorf("%w: error writing output file %s: %s", errOutput, cfg.OutputFile, err)
	}
	return nil
}

func main() {
	switch err := run(os.Args[1:], os.Stdout, os.Stderr); {
	case err == nil, errors.Is(err, flag.ErrHelp):
	case errors.Is(err, errOutput):
		slog.Error(err.Error())
		os.Exit(2)
	default:
		slog.Error(err.Error())
		os.Exit(1)
	}
}
