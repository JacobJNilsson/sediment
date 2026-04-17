// Package main provides the sediment CLI entry point.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/google/uuid"
	"github.com/jacobjnilsson/sediment/internal/decay"
	"github.com/jacobjnilsson/sediment/internal/model"
	"github.com/jacobjnilsson/sediment/internal/store"
)

const defaultDBFile = ".sediment.db"

const skillContent = `---
name: sediment
description: Manage persistent memory across sessions. Use this skill at the start of every session to load context, and whenever you learn something worth remembering about the user, project, or codebase. Also use it when facts contradict what you previously stored.
---

# Sediment - Persistent Agent Memory

You have access to ` + "`sediment`" + `, a CLI tool for persistent memory across sessions.
The database defaults to ` + "`.sediment.db`" + ` in the current working directory - one
per repo. Run ` + "`sediment --help`" + ` for full usage.

## When to use this skill

### Session start (always)
Load relevant memories to prime your context:

` + "```sh" + `
sediment strata
` + "```" + `

If there are many memories, filter by state:

` + "```sh" + `
sediment strata --state active
` + "```" + `

Excavate specific memories you plan to use (this reinforces their confidence):

` + "```sh" + `
sediment excavate --id <uuid>
` + "```" + `

### During a session
Deposit new learnings whenever you discover something worth remembering:

- User preferences (coding style, commit conventions, tools they like)
- Project facts (architecture decisions, file layout, key dependencies)
- Identity info (names, GitHub handles, workspace paths)
- Codebase patterns (how tests are structured, naming conventions)

` + "```sh" + `
sediment deposit --content "..." --tags "..." --source "session-context"
` + "```" + `

### When facts change
If you learn something that contradicts an existing memory, resolve it:

` + "```sh" + `
sediment resolve --action update --id <uuid> --content "corrected fact"
sediment resolve --action supersede --id <uuid> --content "new truth"
` + "```" + `

### Periodically
Run decay to let stale memories fade naturally:

` + "```sh" + `
sediment erode
` + "```" + `

## Guidelines

- **Do not deposit trivial or ephemeral facts.** Only store things that would be useful in a future session.
- **Use meaningful tags.** They help filter and group memories later.
- **Keep content concise.** One clear statement per memory, not paragraphs.
- **Run erode occasionally**, not every session. Once a week or when memories accumulate.
- **Do not dump the full strata output to the user** unless asked. Use it silently to inform your responses.
- **The .sediment.db file should be gitignored.** It contains local agent context, not project source.
`

const globalUsage = `sediment - AI memory lifecycle manager

Memories are deposited, decay over time, get reinforced through access,
compressed when redundant, and resolved when contradictory.
All output is JSON. All commands accept --db <path> (default: .sediment.db).

Commands:
  init      Initialise a new database
  status    Show memory counts by lifecycle state
  deposit   Store a new memory
  strata    List memories, optionally filtered by state
  excavate  Retrieve a memory by ID (reinforces its confidence)
  erode     Run decay cycle: fade confidence, transition stale memories
  compact   List compression candidates, or merge memories into one
  resolve   Apply a contradiction resolution (update, supersede, or keep)
  setup     Interactive setup wizard for your coding assistant

Run 'sediment <command> --help' for command-specific usage.`

var commandHelp = map[string]string{
	"init": `Usage: sediment init [--db <path>]

Initialise a new sediment database. Safe to run on an existing database.

Output: {"status":"ok","path":"<absolute path>"}`,

	"status": `Usage: sediment status [--db <path>]

Show memory counts grouped by lifecycle state (active, dormant, archived).

Output: {"total":N,"active":N,"dormant":N,"archived":N,"path":"..."}`,

	"deposit": `Usage: sediment deposit --content <text> [--tags <a,b,c>] [--source <origin>] [--db <path>]

Store a new memory. Confidence starts at 1.0, state at active.

Flags:
  --content  (required) The text content of the memory
  --tags     Comma-separated labels for categorisation
  --source   Where the memory came from (e.g. conversation ID)

Output: the full memory object as JSON`,

	"strata": `Usage: sediment strata [--state <active|dormant|archived>] [--db <path>]

List memories ordered by confidence (descending). Without --state, lists all.

Flags:
  --state  Filter to a specific lifecycle state

Output: JSON array of memory objects (or null if empty)`,

	"excavate": `Usage: sediment excavate --id <uuid> [--db <path>]

Retrieve a memory and reinforce it. The act of accessing a memory boosts
its confidence (counteracting decay). Returns both the pre-boost decayed
confidence and the post-boost confidence.

Flags:
  --id  (required) UUID of the memory to excavate

Output: {"memory":{...},"decayed_confidence":N,"boosted_confidence":N,"state":"..."}`,

	"erode": `Usage: sediment erode [--db <path>]

Run a decay cycle across all memories. Applies exponential time-based decay
to each memory's confidence and transitions memories between lifecycle states:
  active (≥0.4) → dormant (≥0.1) → archived (<0.1)

All updates run in a single transaction.

Output: {"processed":N,"updated":N,"transitions":[{"id":"...","old_state":"...","new_state":"...","old_confidence":N,"new_confidence":N}]}`,

	"compact": `Usage: sediment compact [--db <path>]
       sediment compact --apply <content> --sources <id1,id2,...> [--db <path>]

Without --apply: lists dormant and archived memories as compression candidates.
With --apply: replaces source memories with a single compacted memory.

Flags:
  --apply    The synthesised content for the compacted memory
  --sources  Comma-separated IDs of memories to replace (required with --apply)

Candidate output: {"candidates":[...],"count":N}
Apply output:     {"compacted":{...},"replaced":[...],"source_count":N}`,

	"resolve": `Usage: sediment resolve --action <update|supersede|keep> --id <uuid> [--content <text>] [--db <path>]

Apply a contradiction resolution to an existing memory.

Actions:
  update     Replace the memory's content in place (requires --content)
  supersede  Archive the old memory and deposit a corrected one (requires --content)
  keep       Acknowledge the contradiction but keep the existing memory

Flags:
  --action   (required) Resolution strategy
  --id       (required) UUID of the memory to resolve
  --content  New content (required for update and supersede)

Output varies by action — always includes the affected memory objects as JSON`,

	"setup": `Usage: sediment setup

Interactive setup wizard. Asks which AI agent system you use and where
to install the skill, then creates the database and updates .gitignore.

No flags needed — the wizard guides you through each step.`,
}

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

// userHomeDir wraps os.UserHomeDir. Overridable for tests.
var userHomeDir = os.UserHomeDir

// writeFile wraps os.WriteFile. Overridable for tests.
var writeFile = os.WriteFile

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func isHelp(args []string) bool {
	for _, a := range args {
		if a == "--help" || a == "-h" {
			return true
		}
	}
	return false
}

// statFile wraps os.Stat for testability.
var statFile = os.Stat

func run(args []string, w io.Writer) error {
	if len(args) == 0 {
		if _, err := statFile(defaultDBFile); os.IsNotExist(err) {
			fmt.Fprintln(w, "sediment is not set up in this directory.\n\nRun 'sediment setup' to get started.")
			return nil
		}
		fmt.Fprintln(w, globalUsage)
		return nil
	}

	cmd := args[0]
	rest := args[1:]

	if cmd == "--help" || cmd == "-h" {
		fmt.Fprintln(w, globalUsage)
		return nil
	}

	if help, ok := commandHelp[cmd]; ok && isHelp(rest) {
		fmt.Fprintln(w, help)
		return nil
	}

	switch cmd {
	case "init":
		return cmdInit(rest, w)
	case "status":
		return cmdStatus(rest, w)
	case "deposit":
		return cmdDeposit(rest, w)
	case "strata":
		return cmdStrata(rest, w)
	case "excavate":
		return cmdExcavate(rest, w)
	case "erode":
		return cmdErode(rest, w)
	case "compact":
		return cmdCompact(rest, w)
	case "resolve":
		return cmdResolve(rest, w)
	case "setup":
		return cmdSetup(rest, w)
	default:
		return fmt.Errorf("unknown command: %s (run 'sediment --help' for usage)", cmd)
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
	slices.Sort(tags)

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

// setupConfig holds the answers from the interactive setup form.
type setupConfig struct {
	System string
	Scope  string
}

// runSetupForm is the function that presents the interactive form.
// Overridable for tests.
var runSetupForm = func() (*setupConfig, error) {
	var system, scope string

	err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Which AI agent system do you use?").
				Options(
					huh.NewOption("OpenCode", "opencode"),
				).
				Value(&system),

			huh.NewSelect[string]().
				Title("Where should the skill be installed?").
				Options(
					huh.NewOption("Global (~/.agents/skills/) — available in all repos", "global"),
					huh.NewOption("Workspace (.agents/skills/) — this project only", "workspace"),
				).
				Value(&scope),
		),
	).Run()
	if err != nil {
		return nil, err
	}
	return &setupConfig{System: system, Scope: scope}, nil
}

func cmdSetup(args []string, w io.Writer) error {
	cfg, err := runSetupForm()
	if err != nil {
		return fmt.Errorf("setup cancelled: %w", err)
	}

	var skillDir string
	switch cfg.Scope {
	case "global":
		home, err := userHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home directory: %w", err)
		}
		skillDir = filepath.Join(home, ".agents", "skills", "sediment")
	case "workspace":
		skillDir = filepath.Join(".agents", "skills", "sediment")
	}

	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return fmt.Errorf("create skill directory: %w", err)
	}

	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := writeFile(skillPath, []byte(skillContent), 0o644); err != nil {
		return fmt.Errorf("write skill file: %w", err)
	}

	absSkillPath, _ := filepath.Abs(skillPath)

	db, absDBPath, err := openAndMigrate(args)
	if err != nil {
		return err
	}
	db.Close()

	gitignoreUpdated := ensureGitignore()

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Setup complete!")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  Skill:     %s\n", absSkillPath)
	fmt.Fprintf(w, "  Database:  %s\n", absDBPath)
	if gitignoreUpdated {
		fmt.Fprintln(w, "  Gitignore: updated (.sediment.db added)")
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Start a new agent session — it will pick up your memories automatically.")

	return nil
}

func ensureGitignore() bool {
	const entry = ".sediment.db"
	data, err := os.ReadFile(".gitignore")
	if err != nil {
		data = []byte{}
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == entry {
			return false
		}
	}
	addition := "\n# Agent memory (repo-local, not source)\n.sediment.db\n.sediment.db-wal\n.sediment.db-shm\n"
	if len(data) > 0 && !strings.HasSuffix(string(data), "\n") {
		addition = "\n" + addition
	}
	os.WriteFile(".gitignore", append(data, []byte(addition)...), 0o644)
	return true
}
