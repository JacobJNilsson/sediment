// Package migrate runs versioned upgrades on startup.
//
// Each migration is a Go function that receives a context with access to the
// database and data directory. Migrations run forward-only, sequentially from
// the current schema version to the latest.
package migrate

import (
	"database/sql"
	"fmt"
	"slices"
	"strconv"
)

// Context is passed to each migration function.
type Context struct {
	DB      *sql.DB
	DataDir string
}

// Migration describes a single versioned upgrade step.
type Migration struct {
	Version     int
	Description string
	Up          func(ctx Context) error
}

const metaKey = "schema_version"

// Run executes all pending migrations in order. It sorts migrations by version,
// bootstraps the meta table if it does not exist, reads the current schema
// version, and applies each migration whose version exceeds the current one.
func Run(db *sql.DB, dataDir string, migrations []Migration) error {
	sorted := make([]Migration, len(migrations))
	copy(sorted, migrations)
	slices.SortFunc(sorted, func(a, b Migration) int {
		return a.Version - b.Version
	})

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("migrate: bootstrap meta: %w", err)
	}

	current, err := currentVersion(db)
	if err != nil {
		return fmt.Errorf("migrate: read version: %w", err)
	}

	ctx := Context{DB: db, DataDir: dataDir}

	for _, m := range sorted {
		if m.Version <= current {
			continue
		}
		if err := m.Up(ctx); err != nil {
			return fmt.Errorf("migrate: v%d (%s): %w", m.Version, m.Description, err)
		}
		if err := setVersion(db, m.Version); err != nil {
			return fmt.Errorf("migrate: set version %d: %w", m.Version, err)
		}
	}

	return nil
}

func currentVersion(db *sql.DB) (int, error) {
	var raw string
	err := db.QueryRow(`SELECT value FROM meta WHERE key = ?`, metaKey).Scan(&raw)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(raw)
}

func setVersion(db *sql.DB, v int) error {
	_, err := db.Exec(
		`INSERT INTO meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		metaKey, strconv.Itoa(v),
	)
	return err
}
