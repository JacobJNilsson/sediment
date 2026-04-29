#!/usr/bin/env bash
set -euo pipefail

DB_PATH="$CLAUDE_PROJECT_DIR/.sediment.db"

if [ ! -f "$DB_PATH" ]; then
  exit 0
fi

sediment erode --auto --db "$DB_PATH" >/dev/null 2>&1 || true

MEMORIES=$(sediment strata --db "$DB_PATH" 2>/dev/null) || exit 0

if [ "$MEMORIES" = "null" ] || [ -z "$MEMORIES" ]; then
  exit 0
fi

CONTEXT="## Sediment Memories (persistent across sessions)
$MEMORIES

## Memory Protocol
You have persistent memory via the sediment CLI. After each response,
consider: did I learn something about the user, project, or codebase worth
remembering in a future session? If yes, deposit it:

sediment deposit --content \"...\" --hardness <1-10> --tags \"...\" --db $DB_PATH

Hardness uses the Mohs scale (1-10):
  1-3 (Talc-Calcite): situational, one-off comments, ephemeral context
  4-6 (Fluorite-Feldspar): decisions, preferences, architectural choices
  7-10 (Quartz-Diamond): conventions, patterns, testing approach, team rules

If a new fact contradicts an existing memory, use:
sediment resolve --action supersede --id <uuid> --content \"...\" --db $DB_PATH

Most turns require zero deposits. Only deposit genuinely new, reusable knowledge."

ESCAPED=$(printf '%s' "$CONTEXT" | awk '
  BEGIN { ORS="" }
  {
    gsub(/\\/, "\\\\")
    gsub(/"/, "\\\"")
    gsub(/\t/, "\\t")
    if (NR > 1) printf "\\n"
    print
  }
')

printf '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"%s"}}' "$ESCAPED"
