package main

// import sqlite3 driver
import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/alberanid/medialocator/config"
	_ "github.com/mattn/go-sqlite3"
)

func tag2items(db *sql.DB, tag string) []int {
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
		rows.Scan(&tagId)
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
	for rows.Next() {
		var metadataItemId int
		rows.Scan(&metadataItemId)

		miRows, err := db.Query("SELECT id FROM media_items WHERE metadata_item_id=?", metadataItemId)
		if err != nil {
			slog.Error(fmt.Sprintf("tag2items error getting media_items.id for tag %s: %s", tag, err))
			return items
		}
		for miRows.Next() {
			var miRowID int
			miRows.Scan(&miRowID)
			items = append(items, miRowID)
		}
	}
	slog.Debug(fmt.Sprintf("tag %s metadata items: %d", tag, len(items)))
	return items
}

func media2parts(db *sql.DB, mediaId int) []string {
	parts := []string{}
	rows, err := db.Query("SELECT file FROM media_parts WHERE media_item_id=?", mediaId)
	if err != nil {
		slog.Error(fmt.Sprintf("media2parts error getting media_parts for media %d: %s", mediaId, err))
		return parts
	}
	for rows.Next() {
		var part string
		rows.Scan(&part)
		parts = append(parts, part)
	}
	slog.Debug(fmt.Sprintf("media %d parts: %s", mediaId, strings.Join(parts, ", ")))
	return parts
}

func main() {
	cfg := config.ParseArgs()

	if _, err := os.Stat(cfg.PlexDb); os.IsNotExist(err) {
		fmt.Printf("File %s does not exist\n", cfg.PlexDb)
		os.Exit(1)
	}

	// Open the database
	db, err := sql.Open("sqlite3", cfg.PlexDb)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	for _, tag := range cfg.Tags {
		items := tag2items(db, tag)
		for _, item := range items {
			parts := media2parts(db, item)
			for _, part := range parts {
				fmt.Printf("%s\n", part)
			}
		}
	}

}
