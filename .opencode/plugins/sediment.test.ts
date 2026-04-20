import { describe, test, expect } from "bun:test"
import { $ } from "bun"
import { unlinkSync } from "fs"

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
