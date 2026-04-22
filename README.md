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

```sh
brew install jacobjnilsson/tap/sediment   # or download from GitHub releases
```

Then set up a project:

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

The plugin runs automatically when OpenCode opens the repo. On each session it:

1. Runs `erode --auto` to apply decay since last session.
2. Loads active and dormant memories into the system prompt.
3. Instructs the agent to deposit new reusable knowledge via the `sediment_deposit` tool.

> **Note:** The plugin currently lives in `.opencode/plugins/` and only activates inside this repo. Global installation (so it runs in every project) is [tracked in #17](https://github.com/JacobJNilsson/sediment/issues/17).
