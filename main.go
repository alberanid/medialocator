package main

// import sqlite3 driver
import (
	"database/sql"
	"fmt"
	"os"

	"github.com/alberanid/medialocator/config"
)

func tag2items(db *sql.DB, tag string) []int {
	items := []int{}
	// get tag id from tags table
	rows, err := db.Query("SELECT id FROM tag WHERE tag=?", tag)
	if err != nil {
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
		return items
	}
	//fmt.Printf()
	return items
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
		fmt.Printf("tag %s items: %d\n", tag, len(items))
	}

}
