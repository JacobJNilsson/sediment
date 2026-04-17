// Package main provides the sediment CLI entry point.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jacobjnilsson/sediment/internal/decay"
	"github.com/jacobjnilsson/sediment/internal/model"
	"github.com/jacobjnilsson/sediment/internal/store"
)

const defaultDBFile = ".sediment.db"

// erodeTransition records a memory that changed lifecycle state during erosion.
type erodeTransition struct {
	ID       string      `json:"id"`
	OldState model.State `json:"old_state"`
	NewState model.State `json:"new_state"`
	OldConf  float64     `json:"old_confidence"`
	NewConf  float64     `json:"new_confidence"`
}

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
		return fmt.Errorf("usage: sediment <command> [args]\n\nCommands:\n  init      Initialise a new sediment database\n  status    Show database status\n  deposit   Store a new memory\n  strata    List memory layers\n  excavate  Retrieve a memory by ID (reinforces confidence)\n  erode     Run decay cycle across all memories\n  compact   Identify or apply compression of related memories\n  resolve   Apply a contradiction resolution to a memory")
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
	case "erode":
		return cmdErode(args[1:], w)
	case "compact":
		return cmdCompact(args[1:], w)
	case "resolve":
		return cmdResolve(args[1:], w)
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

	// Reinforce: the act of accessing a memory strengthens it.
	decay.Reinforce(m, now, cfg)

	// Reclassify state based on new confidence.
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

func cmdErode(args []string, w io.Writer) error {
	db, _, err := openAndMigrate(args)
	if err != nil {
		return err
	}
	defer db.Close()

	all, err := db.ListAll()
	if err != nil {
		return fmt.Errorf("erode list: %w", err)
	}

	cfg := decay.DefaultConfig()
	now := timeNow()

	transitions := []erodeTransition{}
	updated := 0

	type pendingUpdate struct {
		memory   *model.Memory
		oldState model.State
		oldConf  float64
	}
	var pending []pendingUpdate

	for _, m := range all {
		oldState := m.State
		oldConf := m.Confidence

		newConf := decay.CurrentConfidence(m, now, cfg.Lambda)
		newState := decay.Classify(newConf, cfg)

		if math.Abs(newConf-oldConf) < 1e-12 && newState == oldState {
			continue
		}

		m.Confidence = newConf
		m.State = newState
		m.UpdatedAt = now

		pending = append(pending, pendingUpdate{m, oldState, oldConf})
	}

	if len(pending) > 0 {
		if err := db.RunInTx(func() error {
			for _, p := range pending {
				if err := db.Update(p.memory); err != nil {
					return fmt.Errorf("erode update %s: %w", p.memory.ID, err)
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}

	for _, p := range pending {
		updated++
		if p.memory.State != p.oldState {
			transitions = append(transitions, erodeTransition{
				ID:       p.memory.ID,
				OldState: p.oldState,
				NewState: p.memory.State,
				OldConf:  p.oldConf,
				NewConf:  p.memory.Confidence,
			})
		}
	}

	return writeJSON(w, map[string]any{
		"processed":   len(all),
		"updated":     updated,
		"transitions": transitions,
	})
}

// cmdCompact has two modes:
//   - Without --apply: lists dormant/archived memories as compression candidates.
//   - With --apply: replaces a set of source memories with a single compacted memory.
//     The agent is expected to have already synthesised the compressed content.
func cmdCompact(args []string, w io.Writer) error {
	db, _, err := openAndMigrate(args)
	if err != nil {
		return err
	}
	defer db.Close()

	applyContent := flagValue(args, "--apply")
	if applyContent != "" {
		return compactApply(db, args, applyContent, w)
	}
	return compactCandidates(db, w)
}

// compactCandidates lists dormant and archived memories that could be compressed.
func compactCandidates(db storeI, w io.Writer) error {
	dormant, err := db.ListByState(model.StateDormant)
	if err != nil {
		return fmt.Errorf("compact list dormant: %w", err)
	}
	archived, err := db.ListByState(model.StateArchived)
	if err != nil {
		return fmt.Errorf("compact list archived: %w", err)
	}

	candidates := append(dormant, archived...)
	return writeJSON(w, map[string]any{
		"candidates": candidates,
		"count":      len(candidates),
	})
}

// compactApply replaces source memories with a single compressed memory.
func compactApply(db storeI, args []string, content string, w io.Writer) error {
	sourceIDs := flagValue(args, "--sources")
	if sourceIDs == "" {
		return fmt.Errorf("--sources is required with --apply")
	}

	var ids []string
	for _, id := range strings.Split(sourceIDs, ",") {
		id = strings.TrimSpace(id)
		if id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return fmt.Errorf("--sources must contain at least one ID")
	}

	// Verify all source memories exist and collect tags in one pass.
	tagSet := map[string]struct{}{}
	for _, id := range ids {
		m, err := db.Get(id)
		if err != nil {
			return fmt.Errorf("compact source %s: %w", id, err)
		}
		for _, t := range m.Tags {
			tagSet[t] = struct{}{}
		}
	}
	var tags []string
	for t := range tagSet {
		tags = append(tags, t)
	}
	sort.Strings(tags)

	now := timeNow()
	m := &model.Memory{
		ID:             newUUID(),
		Content:        content,
		Confidence:     0.8, // compacted memories start slightly below full confidence
		State:          model.StateActive,
		AccessCount:    0,
		CreatedAt:      now,
		UpdatedAt:      now,
		LastAccessedAt: now,
		Tags:           tags,
		Source:         "compact:" + strings.Join(ids, ","),
	}

	// Run insert + deletes in a transaction so either all succeed or all roll back.
	if err := db.RunInTx(func() error {
		if err := db.Insert(m); err != nil {
			return fmt.Errorf("compact insert: %w", err)
		}
		for _, id := range ids {
			if err := db.Delete(id); err != nil {
				return fmt.Errorf("compact delete %s: %w", id, err)
			}
		}
		return nil
	}); err != nil {
		return err
	}

	return writeJSON(w, map[string]any{
		"compacted":    m,
		"replaced":     ids,
		"source_count": len(ids),
	})
}

// cmdResolve applies a contradiction resolution to a memory.
// The agent detects the contradiction and decides the resolution;
// this command applies the decision by updating or replacing content.
//
// Modes:
//   - --action=update --id=<id> --content=<new>  Update existing memory's content.
//   - --action=supersede --id=<id> --content=<new>  Archive old, deposit new.
//   - --action=keep --id=<id>  Keep the existing memory, no change (just acknowledge).
func cmdResolve(args []string, w io.Writer) error {
	action := flagValue(args, "--action")
	if action == "" {
		return fmt.Errorf("--action is required (update|supersede|keep)")
	}
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
		return fmt.Errorf("resolve get: %w", err)
	}

	now := timeNow()

	switch action {
	case "update":
		content := flagValue(args, "--content")
		if content == "" {
			return fmt.Errorf("--content is required for action=update")
		}
		oldContent := m.Content
		m.Content = content
		m.UpdatedAt = now
		if err := db.Update(m); err != nil {
			return fmt.Errorf("resolve update: %w", err)
		}
		if err := writeJSON(w, map[string]any{
			"action":      "update",
			"id":          m.ID,
			"old_content": oldContent,
			"new_content": content,
			"memory":      m,
		}); err != nil {
			return err
		}

	case "supersede":
		content := flagValue(args, "--content")
		if content == "" {
			return fmt.Errorf("--content is required for action=supersede")
		}
		// Archive the old memory and deposit a new one in a single transaction.
		m.State = model.StateArchived
		m.UpdatedAt = now
		newMem := &model.Memory{
			ID:             newUUID(),
			Content:        content,
			Confidence:     1.0,
			State:          model.StateActive,
			AccessCount:    0,
			CreatedAt:      now,
			UpdatedAt:      now,
			LastAccessedAt: now,
			Tags:           m.Tags,
			Source:         "supersede:" + m.ID,
		}
		if err := db.RunInTx(func() error {
			if err := db.Update(m); err != nil {
				return fmt.Errorf("resolve archive: %w", err)
			}
			if err := db.Insert(newMem); err != nil {
				return fmt.Errorf("resolve insert: %w", err)
			}
			return nil
		}); err != nil {
			return err
		}
		if err := writeJSON(w, map[string]any{
			"action":     "supersede",
			"archived":   m,
			"new_memory": newMem,
		}); err != nil {
			return err
		}

	case "keep":
		if err := writeJSON(w, map[string]any{
			"action": "keep",
			"id":     m.ID,
			"memory": m,
		}); err != nil {
			return err
		}

	default:
		return fmt.Errorf("unknown action: %s (expected update|supersede|keep)", action)
	}

	return nil
}
