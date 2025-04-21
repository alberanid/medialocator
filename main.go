package main

// import sqlite3 driver
import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/alberanid/medialocator/config"
	_ "github.com/mattn/go-sqlite3"
)

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

// recursively get metadata_items.id selecting rows from metadata_items that have a parent_id
func getChildren(db *sql.DB, parentId int) []int {
	items := []int{}
	rows, err := db.Query("SELECT id FROM metadata_items WHERE parent_id=?", parentId)
	if err != nil {
		slog.Error(fmt.Sprintf("getChildren error getting children of %d: %s", parentId, err))
		return items
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		rows.Scan(&id)
		items = append(items, id)
		children := getChildren(db, id)
		items = append(items, children...)
	}
	return items
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
			continue
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// tag2items returns a list of media_items.id for a given tag, filtered by librarySectionIDs if not empty
func tag2items(db *sql.DB, tag string, librarySectionIDs []int) []int {
	items := []int{}
	// get tag id from tags table
	rows, err := db.Query("SELECT id FROM tags WHERE tag=?", tag)
	if err != nil {
		slog.Error(fmt.Sprintf("tag2items error getting tag %s: %s", tag, err))
		return items
	}
	defer rows.Close()
	// get first row
	var tagId int
	_found := false
	if rows.Next() {
		err = rows.Scan(&tagId)
		if err != nil {
			slog.Error(fmt.Sprintf("tag2items error scanning tag %s: %s", tag, err))
			return items
		}
		_found = true
	}
	if !_found {
		slog.Debug(fmt.Sprintf("tag %s not found", tag))
		return items
	}
	slog.Debug(fmt.Sprintf("tag %s id %d", tag, tagId))
	// get metadata_item_id from taggings table
	rows, err = db.Query("SELECT metadata_item_id FROM taggings WHERE tag_id=?", tagId)
	if err != nil {
		slog.Error(fmt.Sprintf("tag2items error getting taggings for tag %s: %s", tag, err))
		return items
	}

	metadataItemIds := []int{}
	for rows.Next() {
		var metadataItemId int
		err = rows.Scan(&metadataItemId)
		if err != nil {
			slog.Error(fmt.Sprintf("tag2items error scanning taggings for tag %s: %s", tag, err))
			continue
		}
		metadataItemIds = append(metadataItemIds, metadataItemId)
	}

	// also add entries that came from rows that have a parent_id set to one of
	// the values seen in taggings.
	childrenMetadataItemIds := []int{}
	for _, metadataItemId := range metadataItemIds {
		childrenMetadataItemIds = append(childrenMetadataItemIds, getChildren(db, metadataItemId)...)
	}
	metadataItemIds = append(metadataItemIds, childrenMetadataItemIds...)

	for _, metadataItemId := range metadataItemIds {
		var miRows *sql.Rows
		var err error
		if len(librarySectionIDs) > 0 {
			clause, args := librarySectionFilter("library_section_id", librarySectionIDs)
			query := fmt.Sprintf("SELECT id FROM media_items WHERE metadata_item_id=? AND %s", clause)
			args = append([]interface{}{metadataItemId}, args...)
			miRows, err = db.Query(query, args...)
		} else {
			miRows, err = db.Query("SELECT id FROM media_items WHERE metadata_item_id=?", metadataItemId)
		}
		if err != nil {
			slog.Error(fmt.Sprintf("tag2items error getting media_items.id for tag %s: %s", tag, err))
			return items
		}
		for miRows.Next() {
			var miRowID int
			err = miRows.Scan(&miRowID)
			if err != nil {
				slog.Error(fmt.Sprintf("tag2items error scanning media_items.id for tag %s: %s", tag, err))
				continue
			}
			items = append(items, miRowID)
		}
	}
	slog.Debug(fmt.Sprintf("tag %s metadata items: %d", tag, len(items)))
	return items
}

// media2parts returns a list of media_parts.file for a given media_item.id
func media2parts(db *sql.DB, mediaId int) []string {
	parts := []string{}
	rows, err := db.Query("SELECT file FROM media_parts WHERE media_item_id=?", mediaId)
	if err != nil {
		slog.Error(fmt.Sprintf("media2parts error getting media_parts for media %d: %s", mediaId, err))
		return parts
	}
	for rows.Next() {
		var part string
		err = rows.Scan(&part)
		if err != nil {
			slog.Error(fmt.Sprintf("media2parts error scanning media_parts for media %d: %s", mediaId, err))
			continue
		}
		parts = append(parts, part)
	}
	slog.Debug(fmt.Sprintf("media %d parts: %s", mediaId, strings.Join(parts, ", ")))
	return parts
}

// allMediaParts returns a list of all media_parts.file, filtered by librarySectionIDs if not empty
func allMediaParts(cfg *config.Config, db *sql.DB, librarySectionIDs []int) []string {
	parts := []string{}
	var rows *sql.Rows
	var err error
	if len(librarySectionIDs) > 0 {
		clause, args := librarySectionFilter("mi.library_section_id", librarySectionIDs)
		query := fmt.Sprintf(`SELECT mp.file FROM media_parts mp JOIN media_items mi ON mp.media_item_id=mi.id WHERE %s`, clause)
		rows, err = db.Query(query, args...)
	} else {
		rows, err = db.Query("SELECT file FROM media_parts")
	}
	if err != nil {
		slog.Error(fmt.Sprintf("error querying media_parts: %s", err))
		os.Exit(3)
	}
	defer rows.Close()
	for rows.Next() {
		var part string
		if err := rows.Scan(&part); err != nil {
			slog.Error(fmt.Sprintf("error scanning media_parts: %s", err))
			continue
		}
		if cfg.StripPrefix != "" {
			part = strings.TrimPrefix(part, cfg.StripPrefix)
		}
		if cfg.AddPrefix != "" {
			part = path.Join(cfg.AddPrefix, part)
		}
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		parts = append(parts, part)
	}
	slog.Debug(fmt.Sprintf("got a total of %d media parts", len(parts)))
	return parts
}

// itemsNoTags returns a list of media_items.id with no associated tags, filtered by librarySectionIDs if not empty
func itemsNoTags(db *sql.DB, librarySectionIDs []int) []int {
	items := []int{}
	var rows *sql.Rows
	var err error
	if len(librarySectionIDs) > 0 {
		clause, args := librarySectionFilter("library_section_id", librarySectionIDs)
		query := fmt.Sprintf("SELECT id FROM media_items WHERE metadata_item_id NOT IN (SELECT metadata_item_id FROM taggings) AND %s", clause)
		rows, err = db.Query(query, args...)
	} else {
		rows, err = db.Query("SELECT id FROM media_items WHERE metadata_item_id NOT IN (SELECT metadata_item_id FROM taggings)")
	}
	if err != nil {
		slog.Error(fmt.Sprintf("itemsNoTags error querying media_items: %s", err))
		return items
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			slog.Error(fmt.Sprintf("itemsNoTags error scanning media_items.id: %s", err))
			continue
		}
		items = append(items, id)
	}
	slog.Debug(fmt.Sprintf("items with no tags: %d", len(items)))
	return items
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

func main() {
	cfg := config.ParseArgs()

	if _, err := os.Stat(cfg.PlexDb); os.IsNotExist(err) {
		slog.Error(fmt.Sprintf("database %s does not exist", cfg.PlexDb))
		os.Exit(1)
	}

	db, err := sql.Open("sqlite3", fmt.Sprintf("%s?mode=ro", cfg.PlexDb))
	if err != nil {
		slog.Error(fmt.Sprintf("error opening database %s: %s", cfg.PlexDb, err))
	}
	defer db.Close()

	librarySectionIDs := []int{}
	if len(cfg.Libraries) > 0 {
		ids, err := getLibrarySectionIDs(db, cfg.Libraries)
		if err != nil {
			slog.Error(fmt.Sprintf("error getting library section ids: %s", err))
			os.Exit(1)
		}
		if len(ids) == 0 {
			slog.Error("no matching libraries found for -libraries argument")
			os.Exit(1)
		}
		librarySectionIDs = ids
	}

	parts := []string{}
	if cfg.ListAll {
		parts = allMediaParts(cfg, db, librarySectionIDs)
	} else if cfg.NoTags {
		items := itemsNoTags(db, librarySectionIDs)
		for _, item := range items {
			mediaParts := media2parts(db, item)
			parts = append(parts, mediaParts...)
		}
	} else {
		for _, tag := range cfg.Tags {
			items := tag2items(db, tag, librarySectionIDs)
			for _, item := range items {
				mediaParts := media2parts(db, item)
				parts = append(parts, mediaParts...)
			}
		}
	}

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

	outFile := os.Stdout
	if cfg.OutputFile != "" {
		f, err := os.OpenFile(cfg.OutputFile, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			slog.Error(fmt.Sprintf("error opening output file %s: %s", cfg.OutputFile, err))
			os.Exit(2)
		}
		defer f.Close()
		outFile = f
	}

	for _, part := range parts {
		fmt.Fprintf(outFile, "%s\n", part)
	}
}
