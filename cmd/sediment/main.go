// Package main provides the sediment CLI entry point.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jacobjnilsson/sediment/internal/model"
	"github.com/jacobjnilsson/sediment/internal/store"
)

const defaultDBFile = ".sediment.db"

// storeI is the subset of store.DB that commands need.
type storeI interface {
	Migrate() error
	ListAll() ([]*model.Memory, error)
	Insert(m *model.Memory) error
	Close() error
}

// openFunc is the function used to open a database. Overridable for tests.
var openFunc = func(path string) (storeI, error) {
	return store.Open(path)
}

// absFunc wraps filepath.Abs. Overridable for tests.
var absFunc = filepath.Abs

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, w io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: sediment <command> [args]\n\nCommands:\n  init    Initialise a new sediment database\n  status  Show database status")
	}

	switch args[0] {
	case "init":
		return cmdInit(args[1:], w)
	case "status":
		return cmdStatus(args[1:], w)
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func dbPath(args []string) string {
	for i, a := range args {
		if a == "--db" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return defaultDBFile
}

func writeJSON(w io.Writer, v any) {
	out, _ := json.Marshal(v)
	fmt.Fprintln(w, string(out))
}

func openAndMigrate(args []string) (db storeI, absPath string, err error) {
	absPath, err = absFunc(dbPath(args))
	if err != nil {
		return nil, "", fmt.Errorf("resolve path: %w", err)
	}
	db, err = openFunc(absPath)
	if err != nil {
		return nil, "", fmt.Errorf("open database: %w", err)
	}
	if err = db.Migrate(); err != nil {
		db.Close()
		return nil, "", fmt.Errorf("migrate: %w", err)
	}
	return db, absPath, nil
}

func cmdInit(args []string, w io.Writer) error {
	db, absPath, err := openAndMigrate(args)
	if err != nil {
		return err
	}
	defer db.Close()

	writeJSON(w, map[string]string{
		"status": "ok",
		"path":   absPath,
	})
	return nil
}

func cmdStatus(args []string, w io.Writer) error {
	db, absPath, err := openAndMigrate(args)
	if err != nil {
		return err
	}
	defer db.Close()

	all, err := db.ListAll()
	if err != nil {
		return fmt.Errorf("list: %w", err)
	}

	active, dormant, archived := 0, 0, 0
	for _, m := range all {
		switch m.State {
		case "active":
			active++
		case "dormant":
			dormant++
		case "archived":
			archived++
		}
	}

	writeJSON(w, map[string]any{
		"total":    len(all),
		"active":   active,
		"dormant":  dormant,
		"archived": archived,
		"path":     absPath,
	})
	return nil
}
