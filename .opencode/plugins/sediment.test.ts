import { describe, test, expect, beforeEach, afterEach } from "bun:test"
import { $ } from "bun"
import { unlinkSync, mkdtempSync } from "fs"
import { tmpdir } from "os"
import { join } from "path"
import { SedimentPlugin } from "./sediment"

function cleanup(path: string) {
  try {
    unlinkSync(path)
  } catch {}
}

describe("shell helper: array interpolation", () => {
  test("content with spaces preserved", async () => {
    const dbPath = "/tmp/sediment-test-spaces.db"
    cleanup(dbPath)

    const args = ["deposit", "--content", "hello world", "--hardness", "5"]
    const output = await $`sediment ${args} --db ${dbPath}`.quiet().text()
    const parsed = JSON.parse(output)

    expect(parsed.content).toBe("hello world")
    expect(parsed.state).toBe("active")

    cleanup(dbPath)
  })

  test("content with apostrophes preserved", async () => {
    const dbPath = "/tmp/sediment-test-apos.db"
    cleanup(dbPath)

    const args = ["deposit", "--content", "it's a test", "--hardness", "5"]
    const output = await $`sediment ${args} --db ${dbPath}`.quiet().text()
    const parsed = JSON.parse(output)

    expect(parsed.content).toBe("it's a test")

    cleanup(dbPath)
  })

  test("content with quotes preserved", async () => {
    const dbPath = "/tmp/sediment-test-quotes.db"
    cleanup(dbPath)

    const args = [
      "deposit",
      "--content",
      'user said "hello"',
      "--hardness",
      "5",
    ]
    const output = await $`sediment ${args} --db ${dbPath}`.quiet().text()
    const parsed = JSON.parse(output)

    expect(parsed.content).toBe('user said "hello"')

    cleanup(dbPath)
  })

  test("db path with spaces works", async () => {
    const dbPath = "/tmp/sediment test path.db"
    cleanup(dbPath)

    const args = ["deposit", "--content", "test", "--hardness", "5"]
    const output = await $`sediment ${args} --db ${dbPath}`.quiet().text()
    const parsed = JSON.parse(output)

    expect(parsed.content).toBe("test")

    cleanup(dbPath)
  })

  test("strata command works", async () => {
    const dbPath = "/tmp/sediment-test-strata.db"
    cleanup(dbPath)

    await $`sediment deposit --content "memory one" --hardness 5 --db ${dbPath}`.quiet()
    const args = ["strata"]
    const output = await $`sediment ${args} --db ${dbPath}`.quiet().text()

    expect(output).toContain("memory one")

    cleanup(dbPath)
  })

  test("tags with commas preserved", async () => {
    const dbPath = "/tmp/sediment-test-tags.db"
    cleanup(dbPath)

    const args = [
      "deposit",
      "--content",
      "tagged memory",
      "--hardness",
      "5",
      "--tags",
      "foo,bar,baz",
    ]
    const output = await $`sediment ${args} --db ${dbPath}`.quiet().text()
    const parsed = JSON.parse(output)

    expect(parsed.content).toBe("tagged memory")
    expect(parsed.tags).toContain("foo")
    expect(parsed.tags).toContain("bar")

    cleanup(dbPath)
  })
})

describe("plugin hooks", () => {
  let tmpDir: string
  let dbPath: string

  beforeEach(() => {
    tmpDir = mkdtempSync(join(tmpdir(), "sediment-plugin-test-"))
    dbPath = join(tmpDir, ".sediment.db")
  })

  afterEach(() => {
    cleanup(dbPath)
  })

  async function makePlugin() {
    // Minimal stub for the plugin context
    const pluginCtx = {
      $,
      directory: tmpDir,
    } as any
    return SedimentPlugin(pluginCtx)
  }

  test("returns empty hooks when no .sediment.db exists", async () => {
    const hooks = await makePlugin()
    expect(hooks).toEqual({})
  })

  test("registers hooks when .sediment.db exists", async () => {
    // Create the database so the plugin activates
    await $`sediment init --db ${dbPath}`.quiet()

    const hooks = await makePlugin()
    expect(typeof hooks["experimental.chat.system.transform"]).toBe("function")
    expect(typeof hooks["experimental.session.compacting"]).toBe("function")
    expect(hooks["tool"]).toBeDefined()
  })

  test("does NOT register chat.message hook", async () => {
    await $`sediment init --db ${dbPath}`.quiet()

    const hooks = await makePlugin()
    expect(hooks["chat.message"]).toBeUndefined()
  })

  test("system transform injects deposit instruction into system array", async () => {
    await $`sediment init --db ${dbPath}`.quiet()

    const hooks = await makePlugin()
    const systemTransform = hooks["experimental.chat.system.transform"] as Function
    const output = { system: [] as string[] }
    await systemTransform({}, output)
    expect(output.system.some((s: string) => s.includes("sediment_deposit"))).toBe(true)
  })

  test("system transform injects active memories when db has entries", async () => {
    // Deposit a memory first (also creates the db)
    await $`sediment deposit --content "test memory fact" --hardness 7 --db ${dbPath}`.quiet()

    const hooks = await makePlugin()
    const systemTransform = hooks["experimental.chat.system.transform"] as Function
    const output = { system: [] as string[] }
    await systemTransform({}, output)

    const combined = output.system.join("\n")
    expect(combined).toContain("test memory fact")
  })
})

describe("regression: escaped.join() double-escapes", () => {
  test("pre-escaped args joined as string corrupt content", async () => {
    const dbPath = "/tmp/sediment-test-regression.db"
    cleanup(dbPath)

    const args = ["deposit", "--content", "hello world", "--hardness", "5"]
    const escaped = args.map((a) => $.escape(a))
    const escapedDb = $.escape(dbPath)

    let output = ""
    let failed = false
    try {
      output =
        await $`sediment ${escaped.join(" ")} --db ${escapedDb}`.quiet().text()
    } catch {
      failed = true
    }

    if (!failed) {
      const parsed = JSON.parse(output)
      expect(parsed.content).not.toBe("hello world")
    } else {
      expect(failed).toBe(true)
    }

    cleanup(dbPath)
  })
})
