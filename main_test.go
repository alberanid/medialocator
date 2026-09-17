package main

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Expected output for the fixture below, as run prints it (sorted, one path per
// line).
const (
	fixtureListAll        = "/media/Movies/collision.mp4\n/media/Movies/other-label.mp4\n/media/Movies/untagged.mp4\n/media/TV/episode1.mkv\n/media/TV/episode2.mkv\n/media/TV/show.mkv\n"
	fixturePreservePaths  = "/media/TV/episode1.mkv\n/media/TV/episode2.mkv\n/media/TV/show.mkv\n"
	fixtureNoTagsPaths    = "/media/Movies/untagged.mp4\n"
	fixturePreserveOther  = "/media/Movies/other-label.mp4\n/media/TV/episode1.mkv\n/media/TV/episode2.mkv\n/media/TV/show.mkv\n"
	fixtureMoviesPrefixed = "/backup/Movies/collision.mp4\n/backup/Movies/other-label.mp4\n/backup/Movies/untagged.mp4\n"
)

// fixtureStatements build a small Plex-like database:
//
//	metadata 20 (TV show)   is labeled 'preserve' (tag type 11)
//	metadata 21 (episode)   is a child of the show
//	metadata 22 (episode)   is a grandchild of the show, a child of 21
//	metadata 10 (movie)     has two 'preserve' tags in non-label categories
//	metadata 30 (movie)     is labeled 'other'
//	metadata 40 (movie)     is untagged
//	metadata 41 (movie)     is untagged and has blank parts only
var fixtureStatements = []string{
	`CREATE TABLE library_sections (id INTEGER PRIMARY KEY, name TEXT)`,
	`CREATE TABLE metadata_items (id INTEGER PRIMARY KEY, parent_id INTEGER, library_section_id INTEGER)`,
	`CREATE TABLE media_items (id INTEGER PRIMARY KEY, metadata_item_id INTEGER, library_section_id INTEGER)`,
	`CREATE TABLE media_parts (id INTEGER PRIMARY KEY, media_item_id INTEGER, file TEXT)`,
	`CREATE TABLE tags (id INTEGER PRIMARY KEY, tag TEXT, tag_type INTEGER)`,
	`CREATE TABLE taggings (metadata_item_id INTEGER, tag_id INTEGER)`,
	`INSERT INTO library_sections VALUES (1,'Movies'),(2,'TV Shows')`,
	`INSERT INTO metadata_items VALUES (10,NULL,1),(20,NULL,2),(21,20,2),(22,21,2),(30,NULL,1),(40,NULL,1),(41,NULL,1)`,
	`INSERT INTO media_items VALUES (100,10,1),(200,20,2),(201,21,2),(202,22,2),(300,30,1),(400,40,1),(401,41,1)`,
	`INSERT INTO media_parts VALUES
		(1000,100,'/media/Movies/collision.mp4'),
		(2000,200,'/media/TV/show.mkv'),
		(2001,201,'/media/TV/episode1.mkv'),
		(2002,202,'/media/TV/episode2.mkv'),
		(3000,300,'/media/Movies/other-label.mp4'),
		(4000,400,'/media/Movies/untagged.mp4'),
		(4001,401,''),
		(4002,401,'   ')`,
	`INSERT INTO tags VALUES (1,'preserve',11),(2,'preserve',8),(3,'other',11),(4,'preserve',2)`,
	`INSERT INTO taggings VALUES (20,1),(10,2),(10,4),(30,3)`,
}

// newPlexFixture creates a disposable database holding fixtureStatements and
// returns its path.
func newPlexFixture(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "plex.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("opening fixture database: %v", err)
	}
	for _, statement := range fixtureStatements {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			t.Fatalf("executing fixture statement %q: %v", statement, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("closing fixture database: %v", err)
	}
	return dbPath
}

// runCLI invokes run with captured output.
func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	err := run(args, &stdout, &stderr)
	if err != nil {
		t.Logf("run(%q) stderr: %s", args, stderr.String())
	}
	return stdout.String(), err
}

// runFixture runs the tool against the fixture database and fails on error.
func runFixture(t *testing.T, dbPath string, args ...string) string {
	t.Helper()
	out, err := runCLI(t, append(args, "-plex-db", dbPath)...)
	if err != nil {
		t.Fatalf("run(%q): %v", args, err)
	}
	return out
}

func TestVersion(t *testing.T) {
	got, err := runCLI(t, "-version")
	if err != nil {
		t.Fatalf("run(-version): %v", err)
	}
	if !strings.HasPrefix(got, "version ") {
		t.Errorf("got %q, want a version line", got)
	}
}

func TestOutputAliasRejected(t *testing.T) {
	dbPath := newPlexFixture(t)

	aliases := []struct {
		name   string
		output func(t *testing.T) string
	}{
		{"same path", func(t *testing.T) string { return dbPath }},
		{"symlink", func(t *testing.T) string {
			link := filepath.Join(t.TempDir(), "alias.list")
			if err := os.Symlink(dbPath, link); err != nil {
				t.Fatalf("creating symlink: %v", err)
			}
			return link
		}},
		{"hard link", func(t *testing.T) string {
			link := filepath.Join(t.TempDir(), "alias.list")
			if err := os.Link(dbPath, link); err != nil {
				t.Fatalf("creating hard link: %v", err)
			}
			return link
		}},
	}

	for _, alias := range aliases {
		t.Run(alias.name, func(t *testing.T) {
			out, err := runCLI(t, "-list-all", "-plex-db", dbPath, "-output-file", alias.output(t))
			if err == nil {
				t.Fatalf("aliased output file accepted, output %q", out)
			}
			if got := runFixture(t, dbPath, "-list-all"); got != fixtureListAll {
				t.Errorf("database changed after refusal:\n got %q\nwant %q", got, fixtureListAll)
			}
		})
	}
}

func TestOutputFileTruncatedAndWriteFailuresReported(t *testing.T) {
	dbPath := newPlexFixture(t)
	dir := t.TempDir()
	output := filepath.Join(dir, "media.list")
	if err := os.WriteFile(output, []byte("STALE-CONTENT\n"), 0o600); err != nil {
		t.Fatalf("preparing stale output file: %v", err)
	}

	if _, err := runCLI(t, "-tags", "missing", "-plex-db", dbPath, "-output-file", output); err != nil {
		t.Fatalf("run with a nonexistent tag: %v", err)
	}
	content, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("reading output file: %v", err)
	}
	if len(content) != 0 {
		t.Errorf("stale content retained: %q", content)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading output directory: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("temporary files left behind in %s: %v", dir, entries)
	}

	if _, err := runCLI(t, "-list-all", "-plex-db", dbPath, "-output-file", filepath.Join(dir, "missing", "media.list")); err == nil {
		t.Error("write into a nonexistent directory reported success")
	}
}

func TestListAllAppliesPrefixOnce(t *testing.T) {
	dbPath := newPlexFixture(t)
	got := runFixture(t, dbPath, "-list-all", "-libraries", "Movies", "-strip-prefix", "/media", "-add-prefix", "/backup")
	if got != fixtureMoviesPrefixed {
		t.Errorf("got %q, want %q", got, fixtureMoviesPrefixed)
	}
}

func TestTagsOnlyMatchLabels(t *testing.T) {
	dbPath := newPlexFixture(t)
	// 'preserve' also exists as tag types 8 and 2 on the movie, whose path must
	// not be selected.
	if got := runFixture(t, dbPath, "-tags", "preserve"); got != fixturePreservePaths {
		t.Errorf("got %q, want %q", got, fixturePreservePaths)
	}
	if got := runFixture(t, dbPath, "-tags", "preserve,other"); got != fixturePreserveOther {
		t.Errorf("multi-label got %q, want %q", got, fixturePreserveOther)
	}
}

func TestTagHierarchyIsInherited(t *testing.T) {
	dbPath := newPlexFixture(t)
	got := runFixture(t, dbPath, "-tags", "preserve")
	for _, want := range []string{"/media/TV/show.mkv", "/media/TV/episode1.mkv", "/media/TV/episode2.mkv"} {
		if !strings.Contains(got, want+"\n") {
			t.Errorf("labeled hierarchy missing %s in %q", want, got)
		}
	}
}

func TestNoTagsUsesInheritedClosure(t *testing.T) {
	dbPath := newPlexFixture(t)
	// Only the genuinely untagged movie is selected: the episodes inherit the
	// show's label, the colliding movie is tagged in other categories, and the
	// item with blank parts has no path to report.
	if got := runFixture(t, dbPath, "-no-tags"); got != fixtureNoTagsPaths {
		t.Errorf("got %q, want %q", got, fixtureNoTagsPaths)
	}
}

func TestBlankPathsNeverEmitted(t *testing.T) {
	dbPath := newPlexFixture(t)
	modes := [][]string{{"-list-all"}, {"-tags", "preserve"}, {"-no-tags"}}
	for _, args := range modes {
		got := runFixture(t, dbPath, args...)
		if got == "" {
			continue
		}
		for _, line := range strings.Split(strings.TrimSuffix(got, "\n"), "\n") {
			if strings.TrimSpace(line) == "" {
				t.Errorf("%q emitted a blank line: %q", args, got)
			}
		}
	}
}

func TestInvalidDatabaseFails(t *testing.T) {
	modes := [][]string{{"-list-all"}, {"-tags", "preserve"}, {"-no-tags"}}

	t.Run("not a database", func(t *testing.T) {
		notADatabase := filepath.Join(t.TempDir(), "bad.db")
		if err := os.WriteFile(notADatabase, []byte("this is not a sqlite database\n"), 0o644); err != nil {
			t.Fatalf("writing invalid database: %v", err)
		}
		for _, args := range modes {
			if out, err := runCLI(t, append(args, "-plex-db", notADatabase)...); err == nil {
				t.Errorf("%q accepted an invalid database, output %q", args, out)
			}
		}
	})

	t.Run("missing Plex tables", func(t *testing.T) {
		unrelated := filepath.Join(t.TempDir(), "unrelated.db")
		db, err := sql.Open("sqlite3", unrelated)
		if err != nil {
			t.Fatalf("opening sqlite database: %v", err)
		}
		if _, err := db.Exec(`CREATE TABLE unrelated (x INTEGER)`); err != nil {
			t.Fatalf("creating table: %v", err)
		}
		db.Close()
		for _, args := range modes {
			if out, err := runCLI(t, append(args, "-plex-db", unrelated)...); err == nil {
				t.Errorf("%q accepted a database without Plex tables, output %q", args, out)
			}
		}
	})

	t.Run("missing file", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "absent.db")
		if out, err := runCLI(t, "-list-all", "-plex-db", missing); err == nil {
			t.Errorf("accepted a missing database, output %q", out)
		}
	})
}
