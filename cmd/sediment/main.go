// Package main provides the sediment CLI entry point.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jacobjnilsson/sediment/internal/decay"
	"github.com/jacobjnilsson/sediment/internal/model"
	"github.com/jacobjnilsson/sediment/internal/store"
)

const defaultDBFile = ".sediment.db"

// storeI is the subset of store.DB that commands need.
type storeI interface {
	Migrate() error
	ListAll() ([]*model.Memory, error)
	ListByState(state model.State) ([]*model.Memory, error)
	Get(id string) (*model.Memory, error)
	Insert(m *model.Memory) error
	Update(m *model.Memory) error
	Delete(id string) error
	RunInTx(fn func() error) error
	Close() error
}

// openFunc is the function used to open a database. Overridable for tests.
var openFunc = func(path string) (storeI, error) {
	return store.Open(path)
}

// absFunc wraps filepath.Abs. Overridable for tests.
var absFunc = filepath.Abs

// newUUID generates a new UUID string. Overridable for tests.
var newUUID = func() string {
	return uuid.New().String()
}

// timeNow returns the current time. Overridable for tests.
var timeNow = time.Now

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, w io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: sediment <command> [args]\n\nCommands:\n  init      Initialise a new sediment database\n  status    Show database status\n  deposit   Store a new memory\n  strata    List memory layers\n  excavate  Retrieve a memory by ID (reinforces confidence)")
	}

	switch args[0] {
	case "init":
		return cmdInit(args[1:], w)
	case "status":
		return cmdStatus(args[1:], w)
	case "deposit":
		return cmdDeposit(args[1:], w)
	case "strata":
		return cmdStrata(args[1:], w)
	case "excavate":
		return cmdExcavate(args[1:], w)
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func flagValue(args []string, name string) string {
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func flagValueDefault(args []string, name, fallback string) string {
	if v := flagValue(args, name); v != "" {
		return v
	}
	return fallback
}

func writeJSON(w io.Writer, v any) error {
	out, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("json marshal: %w", err)
	}
	_, err = fmt.Fprintln(w, string(out))
	return err
}

func openAndMigrate(args []string) (db storeI, absPath string, err error) {
	absPath, err = absFunc(flagValueDefault(args, "--db", defaultDBFile))
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

	return writeJSON(w, map[string]string{
		"status": "ok",
		"path":   absPath,
	})
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
		case model.StateActive:
			active++
		case model.StateDormant:
			dormant++
		case model.StateArchived:
			archived++
		}
	}

	return writeJSON(w, map[string]any{
		"total":    len(all),
		"active":   active,
		"dormant":  dormant,
		"archived": archived,
		"path":     absPath,
	})
}

func cmdDeposit(args []string, w io.Writer) error {
	content := flagValue(args, "--content")
	if content == "" {
		return fmt.Errorf("--content is required")
	}

	source := flagValue(args, "--source")

	var tags []string
	if raw := flagValue(args, "--tags"); raw != "" {
		for _, t := range strings.Split(raw, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}
	}

	db, _, err := openAndMigrate(args)
	if err != nil {
		return err
	}
	defer db.Close()

	now := timeNow()
	m := &model.Memory{
		ID:             newUUID(),
		Content:        content,
		Confidence:     1.0,
		State:          model.StateActive,
		AccessCount:    0,
		CreatedAt:      now,
		UpdatedAt:      now,
		LastAccessedAt: now,
		Tags:           tags,
		Source:         source,
	}

	if err := db.Insert(m); err != nil {
		return fmt.Errorf("deposit: %w", err)
	}

	return writeJSON(w, m)
}

func cmdStrata(args []string, w io.Writer) error {
	db, _, err := openAndMigrate(args)
	if err != nil {
		return err
	}
	defer db.Close()

	stateFilter := flagValue(args, "--state")

	var memories []*model.Memory
	if stateFilter != "" {
		s := model.State(stateFilter)
		if !model.ValidStates[s] {
			return fmt.Errorf("unknown state: %s (valid: active, dormant, archived)", stateFilter)
		}
		memories, err = db.ListByState(s)
	} else {
		memories, err = db.ListAll()
	}
	if err != nil {
		return fmt.Errorf("strata: %w", err)
	}

	return writeJSON(w, memories)
}

func cmdExcavate(args []string, w io.Writer) error {
	id := flagValue(args, "--id")
	if id == "" {
		return fmt.Errorf("--id is required")
	}

	db, _, err := openAndMigrate(args)
	if err != nil {
		return err
	}
	defer db.Close()

	m, err := db.Get(id)
	if err != nil {
		return fmt.Errorf("excavate: %w", err)
	}

	cfg := decay.DefaultConfig()
	now := timeNow()

	// Snapshot the decayed confidence before reinforcement so the caller
	// can see how much the memory had faded. Reinforce also calls
	// CurrentConfidence internally, but we need the pre-boost value.
	decayedConfidence := decay.CurrentConfidence(m, now, cfg.Lambda)
	decay.Reinforce(m, now, cfg)
	m.State = decay.Classify(m.Confidence, cfg)

	if err := db.Update(m); err != nil {
		return fmt.Errorf("excavate update: %w", err)
	}

	return writeJSON(w, map[string]any{
		"memory":             m,
		"decayed_confidence": decayedConfidence,
		"boosted_confidence": m.Confidence,
		"state":              string(m.State),
	})
}
