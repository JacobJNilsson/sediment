import type { Plugin, Hooks } from "@opencode-ai/plugin"
import { tool } from "@opencode-ai/plugin"
import { existsSync } from "fs"

const DEPOSIT_INSTRUCTION = `## Memory Protocol
You have persistent memory via the \`sediment_deposit\` tool. After each response,
consider: did I learn something about the user, project, or codebase worth
remembering in a future session? If yes, call sediment_deposit with:
- content: concise single-statement fact
- hardness: 1-3 for ephemeral, 4-6 for decisions/preferences, 7-10 for conventions/patterns
- tags: relevant categories
- supersedes_id: ID of contradicted memory (if applicable)

Most turns require zero deposits. Only deposit genuinely new, reusable knowledge.
Current memories are available in context — check for contradictions before depositing.`

export const SedimentPlugin: Plugin = async ({ $, directory }) => {
  const dbPath = `${directory}/.sediment.db`

  if (!existsSync(dbPath)) {
    return {} satisfies Hooks
  }

  const sediment = async (args: string[]) => {
    const result = await $`sediment ${args} --db ${dbPath}`.quiet()
    return result.text().trim()
  }

  let activeMemories: string | null = null

  try {
    await sediment(["erode", "--auto"])
    activeMemories = await sediment(["strata"])
  } catch {
    // sediment CLI not installed — degrade gracefully
  }

  return {
    "experimental.chat.system.transform": async (_input, output) => {
      output.system.push(DEPOSIT_INSTRUCTION)
      if (activeMemories) {
        output.system.push(
          `## Sediment Memories (persistent across sessions)\n${activeMemories}`,
        )
      }
    },

    "experimental.session.compacting": async (_input, output) => {
      try {
        activeMemories = await sediment(["strata"])
      } catch {
        // fall back to cached
      }
      if (activeMemories) {
        output.context.push(
          `## Sediment Memories (persistent across sessions)\n${activeMemories}`,
        )
      }
    },

    tool: {
      sediment_deposit: tool({
        description: [
          "Store a memory for future sessions. Use this after learning something worth",
          "remembering about the user, project, or codebase.",
          "",
          "Hardness uses Mohs scale (1-10):",
          "  1-3 (Talc-Calcite): situational, one-off comments, ephemeral context",
          "  4-6 (Fluorite-Feldspar): decisions, preferences, architectural choices",
          "  7-10 (Quartz-Diamond): conventions, patterns, testing approach, team rules",
          "",
          "Contradiction handling: if the new memory contradicts an existing one,",
          "set supersedes_id to the ID of the old memory. The old memory will be",
          "archived and replaced.",
        ].join("\n"),
        args: {
          content: tool.schema.string(),
          tags: tool.schema.array(tool.schema.string()).optional(),
          hardness: tool.schema.number().min(1).max(10).optional(),
          supersedes_id: tool.schema.string().optional(),
        },
        async execute(args) {
          if (args.supersedes_id) {
            try {
              await sediment([
                "resolve",
                "--action",
                "supersede",
                "--id",
                args.supersedes_id,
                "--content",
                args.content,
              ])
              return `Superseded memory ${args.supersedes_id} with: ${args.content}`
            } catch (e) {
              return `Failed to supersede: ${e}`
            }
          }

          const depositArgs = [
            "deposit",
            "--content",
            args.content,
            "--hardness",
            String(args.hardness ?? 5),
          ]
          if (args.tags?.length) {
            depositArgs.push("--tags", args.tags.join(","))
          }
          try {
            return await sediment(depositArgs)
          } catch (e) {
            return `Failed to deposit: ${e}`
          }
        },
      }),

      sediment_recall: tool({
        description:
          "Retrieve all active and dormant memories from previous sessions. " +
          "Use at the start of a session if memories were not auto-loaded, " +
          "or to refresh context mid-session.",
        args: {},
        async execute() {
          try {
            return await sediment(["strata"])
          } catch (e) {
            return `Failed to recall: ${e}`
          }
        },
      }),
    },
  } satisfies Hooks
}
