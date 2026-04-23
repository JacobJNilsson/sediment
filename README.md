# sediment

Persistent, per-repository memory for AI coding agents. Memories are deposited, decay over time when not accessed, and erode away unless reinforced — like sediment layers.

## How it works

Each project gets a local `.sediment.db` (SQLite). The AI agent deposits facts worth remembering across sessions. Memories have a **confidence** that decays exponentially over time. How fast depends on **hardness** — a Mohs-scale value from 1 (ephemeral) to 10 (permanent):

| Hardness | Mohs mineral | Use for |
|---|---|---|
| 1–3 | Talc–Calcite | Ephemeral context, one-off decisions |
| 4–6 | Fluorite–Feldspar | Preferences, architectural choices |
| 7–10 | Quartz–Diamond | Conventions, team rules, testing patterns |

Memories move through three lifecycle states as confidence falls:

```
active → dormant → archived
```

Accessing a memory (excavating it) boosts its confidence and can reverse decay.

## Install

Download the latest binary from [GitHub releases](https://github.com/JacobJNilsson/sediment/releases), then set up a project:

```sh
cd your-project
sediment setup
```

`setup` initialises the database, installs the agent skill, and appends `.sediment.db` to `.gitignore`.

## Commands

```
sediment deposit --content "text" [--hardness 1-10] [--tags tag1,tag2]
sediment strata  [--state active|dormant|archived]
sediment excavate --id <uuid>
sediment erode   [--auto]
sediment compact [--apply "summary" --sources id1,id2]
sediment resolve --action update|supersede|keep --id <uuid> --content "text"
sediment status
```

All commands accept `--db <path>` to override the default `.sediment.db`. All output is JSON.

## OpenCode plugin

The `sediment setup` wizard installs the plugin globally to `~/.config/opencode/plugins/sediment.ts`. It runs in every OpenCode session but only activates when a `.sediment.db` exists in the working directory, so repos that haven't been set up are unaffected.

On each session in a set-up repo, the plugin:

1. Runs `erode --auto` to apply decay since the last session.
2. Loads active and dormant memories into the system prompt.
3. Instructs the agent to deposit new reusable knowledge via the `sediment_deposit` tool.
