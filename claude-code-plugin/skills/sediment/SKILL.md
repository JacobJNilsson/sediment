---
name: sediment
description: Manage persistent memory across sessions. Use this skill at the start of every session to load context, and whenever you learn something worth remembering about the user, project, or codebase. Also use it when facts contradict what you previously stored.
allowed-tools: Bash(sediment *)
---

# Sediment - Persistent Agent Memory

`sediment` is a CLI for persistent memory across sessions. Each repo gets its
own `.sediment.db` (SQLite). Memories decay over time unless reinforced by
access, so stale knowledge fades naturally. Run `sediment --help` for full usage.

## Session start

If the plugin hooks are active, erosion and memory loading happen
automatically. Otherwise, run these yourself:

```sh
sediment erode --auto
sediment strata
```

Excavate specific memories you plan to use (reinforces their confidence):

```sh
sediment excavate --id <uuid>
```

## During a session

Deposit new learnings whenever you discover something worth remembering:

- User preferences (coding style, commit conventions, tools they like)
- Project facts (architecture decisions, file layout, key dependencies)
- Identity info (names, GitHub handles, workspace paths)
- Codebase patterns (how tests are structured, naming conventions)

```sh
sediment deposit --content "..." --tags "..." --source "session-context"
```

If a new fact contradicts an existing memory, resolve it:

```sh
sediment resolve --action update --id <uuid> --content "corrected fact"
sediment resolve --action supersede --id <uuid> --content "new truth"
```

## Guidelines

- **Do not deposit trivial or ephemeral facts.** Only store things useful in a future session.
- **Use meaningful tags.** They help filter and group memories later.
- **Keep content concise.** One clear statement per memory, not paragraphs.
- **Do not dump the full strata output to the user** unless asked. Use it silently to inform your responses.
