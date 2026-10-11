# Repository Guidelines

## Project Structure & Module Organization

Medialocator is a Go CLI that lists movie and TV media paths from a Plex SQLite database. `main.go` contains database queries, path transformations, output handling, and the CLI entry point. `config/config.go` parses and validates flags; `version/version.go` holds the release version. `main_test.go` contains CLI regression tests and disposable database fixtures. `samples/` contains a sample SQLite database; `README.md` documents usage.

## Build, Test, and Development Commands

Use Go compatible with the `go 1.23.2` declaration in `go.mod`. The `github.com/mattn/go-sqlite3` driver requires CGO and a C compiler.

- `go build .` builds the `medialocator` executable.
- `go test ./...` runs all package tests.
- `go test -cover ./...` reports test coverage; no minimum coverage threshold is configured.
- `go vet ./...` checks common Go mistakes.
- `go run . -tags preserve,classic -plex-db /path/to/plex.db` runs locally and prints matching paths. Add `-output-file media.list` to save them.

## Coding Style & Naming Conventions

Format changed Go files with `gofmt -w`; use its standard tab indentation and import formatting. Follow existing package boundaries, mixed-case Go identifiers, and descriptive helper names such as `readOnlyDSN`. Keep SQL values parameterized and propagate errors with useful context. No custom formatter or lint configuration is present.

## Testing Guidelines

Tests use Go's standard `testing` package. Name files `*_test.go` and functions `TestBehavior`; use table-driven subtests for related cases. Extend the existing `newPlexFixture` helper and `t.TempDir()` fixtures rather than relying on a live Plex installation. Cover changed selection behavior, inherited labels, library filters, path prefixes, and failure handling. Run `go test ./...` before submitting changes.

## Commit & Pull Request Guidelines

Recent commits use short, lowercase, action-oriented subjects, such as `introduce a test suite`; follow that style and keep commits focused. Pull requests should explain the problem, resulting behavior, and validation commands, and link related issues when applicable. Update `README.md` when flags or documented behavior change.

## Database Safety

Preserve read-only database access, rejection of output paths that alias the database, and atomic output replacement. Never commit personal Plex databases, WAL/SHM files, or generated binaries; check `.gitignore` and use disposable fixtures for validation.
