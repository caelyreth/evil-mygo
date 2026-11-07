import { afterEach, expect, test } from "bun:test"
import fs from "node:fs/promises"
import os from "node:os"
import path from "node:path"
import { docsAssets } from "../website/llms"
import { rewriteHref } from "../website/src/lib/markdown-source"
import { site } from "../website/src/lib/site"

const temporary: string[] = []
afterEach(async () => {
  for (const dir of temporary.splice(0)) await fs.rm(dir, { recursive: true, force: true })
})

test("publishes every doc, with nested README paths and no repository-only instructions", async () => {
  const repo = await fs.mkdtemp(path.join(os.tmpdir(), "mygo-llms-"))
  temporary.push(repo)
  await fs.mkdir(path.join(repo, "docs/ui"), { recursive: true })
  await fs.mkdir(path.join(repo, "docs/plugins"), { recursive: true })
  const sources = {
    "README.md": "# Introduction\n\nOverview.\n\n[Native UI](ui/README.md)\n",
    "ui/README.md": "# Native UI\n\nNative overview.\n\n[Home](../README.md)\n",
    "plugins/fetch.md": `# Fetch

Fetch data.

[UI](../ui/README.md#input) and [Go file](../../app.go).
[External](https://example.com) and [Local](#options).
[Reference][ui]

[ui]: ../ui/README.md?view=all#input

<!-- repository-only:start -->

## Private instructions

Never publish this.

<!-- repository-only:end -->

## Options

| Option | Value |
| --- | --- |
| Method | GET |

~~~html
<!-- repository-only:start -->
This is a code example.
<!-- repository-only:end -->
~~~
`,
  }
  for (const [file, source] of Object.entries(sources)) await fs.writeFile(path.join(repo, "docs", file), source)
  const assets = await docsAssets(repo)
  expect([...assets.keys()]).toEqual(["/docs.md", "/docs/plugins/fetch.md", "/docs/ui.md", "/llms.txt", "/llms-full.txt"])
  const index = assets.get("/llms.txt")!
  expect(index).toStartWith(`# ${site.name}\n\n> `)
  for (const file of ["/docs.md", "/docs/plugins/fetch.md", "/docs/ui.md"]) {
    expect(index).toContain(`${site.url}${file}`)
    expect(assets.get("/llms-full.txt")).toContain(assets.get(file)!.trimEnd())
  }
  const fetch = assets.get("/docs/plugins/fetch.md")!
  expect(fetch).toContain(`${site.url}/docs/ui.md#input`)
  expect(fetch).toContain(`${site.url}/docs/ui.md?view=all#input`)
  expect(fetch).toContain("https://github.com/caelyreth/evil-mygo/blob/main/app.go")
  expect(fetch).toContain("[External](https://example.com)")
  expect(fetch).toContain("[Local](#options)")
  expect(fetch).toContain("| Method | GET")
  expect(fetch).toContain("```html\n<!-- repository-only:start -->\nThis is a code example.")
  for (const text of assets.values()) {
    expect(text).not.toContain("Private instructions")
    expect(text).not.toContain("Never publish this")
  }
  expect(assets.get("/docs/ui.md")).toContain(`${site.url}/docs.md`)

  // The dev server and subsequent builds read the current source each time.
  await fs.writeFile(path.join(repo, "docs/new.md"), "# New page\n\nNew content.\n")
  expect((await docsAssets(repo)).get("/llms.txt")).toContain(`[New page](${site.url}/docs/new.md)`)
})

test("shared link rules keep HTML docs URLs and repository links intact", () => {
  const links = { docsDir: "/repo/docs", repoDir: "/repo", repoUrl: "https://github.com/caelyreth/evil-mygo" }
  const file = "/repo/docs/ui/button.md"
  expect(rewriteHref("README.md#views", file, links)).toBe("/docs/ui#views")
  expect(rewriteHref("../README.md", file, links)).toBe("/docs")
  expect(rewriteHref("../../ui/button.go", file, links)).toBe("https://github.com/caelyreth/evil-mygo/blob/main/ui/button.go")
  expect(rewriteHref("../../examples", file, links)).toBe("https://github.com/caelyreth/evil-mygo/tree/main/examples")
  for (const href of ["#input", "/docs/ui", "https://example.com", "//example.com", "mailto:hello@example.com"]) {
    expect(rewriteHref(href, file, links)).toBe(href)
  }
})
