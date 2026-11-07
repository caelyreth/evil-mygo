// Sets or checks the fork release version, which the Go module
// (mygo.Version), the CLI and the npm packages share:
//
//   bun scripts/version.ts 0.2.0            # set it everywhere, bun.lock too
//   bun scripts/version.ts --check v0.2.0   # check that everything has it
//   bun scripts/version.ts --upstream 0.8.0 <full-commit>  # after integrating upstream
import { readFile, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { platformDir, platformName, releaseInfo, versionFile, writeManifests } from "../packages/cli/build.ts";
import { platforms } from "../packages/cli/index.js";

const root = join(import.meta.dir, "..");

const versionPattern = /^const Version = "([^"]*)"$/m;
const semver = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$/;

function validVersion(v: string): boolean {
  if (!semver.test(v)) return false;
  const prerelease = v.includes("-") ? v.slice(v.indexOf("-") + 1) : "";
  return !prerelease.split(".").some((id) => /^0\d+$/.test(id));
}

async function setGoConstant(name: string, value: string): Promise<void> {
  const file = join(root, versionFile);
  const source = await readFile(file, "utf8");
  const pattern = new RegExp(`^const ${name} = "[^"]*"$`, "m");
  if (!pattern.test(source)) throw new Error(`${versionFile} declares no ${name}`);
  await writeFile(file, source.replace(pattern, `const ${name} = "${value}"`));
}

/** The official plugins' JavaScript packages, whose peer @caelyreth/evil-mygo-runtime is. */
export const pluginPackages = ["plugins/fetch", "plugins/websocket", "plugins/sqlite", "plugins/watch"];

/** package.json files of the npm packages. */
const packages = [
  "packages/runtime",
  ...pluginPackages,
  "packages/cli",
  ...platforms.map((p) => platformDir(p).slice(root.length + 1)),
];

/**
 * Matches the version that @caelyreth/evil-mygo-cli's optionalDependencies give a platform
 * package in bun.lock, the only entry of its name with a string value.
 */
function lockedPlatform(platform: string): RegExp {
  return new RegExp(`("${platformName(platform)}": )"([^"]*)"`);
}

/**
 * Updates bun.lock to the versions of the packages. bun install keeps the
 * versions of @caelyreth/evil-mygo-cli's optionalDependencies, which are workspaces, so
 * they are set here.
 */
async function updateLockfile(v: string): Promise<void> {
  const install = Bun.spawnSync([process.execPath, "install", "--lockfile-only"], {
    cwd: root,
    stdout: "inherit",
    stderr: "inherit",
  });
  if (!install.success) throw new Error("bun install --lockfile-only failed");
  const file = join(root, "bun.lock");
  let lock = await readFile(file, "utf8");
  for (const p of platforms) {
    if (!lockedPlatform(p).test(lock)) throw new Error(`bun.lock has no version of ${platformName(p)}`);
    lock = lock.replace(lockedPlatform(p), `$1"${v}"`);
  }
  await writeFile(file, lock);
}

async function versions(): Promise<Map<string, string>> {
  const found = new Map<string, string>();
  found.set(versionFile, (await readFile(join(root, versionFile), "utf8")).match(versionPattern)?.[1] ?? "(none)");
  for (const dir of packages) {
    const pkg = JSON.parse(await readFile(join(root, dir, "package.json"), "utf8"));
    found.set(`${dir}/package.json`, pkg.version);
    for (const [name, v] of Object.entries<string>(pkg.optionalDependencies ?? {})) {
      found.set(`${dir}/package.json: ${name}`, v);
    }
    for (const deps of ["peerDependencies", "devDependencies"]) {
      const runtime = pkg[deps]?.["@caelyreth/evil-mygo-runtime"];
      if (runtime) found.set(`${dir}/package.json: ${deps} @caelyreth/evil-mygo-runtime`, runtime.replace(/^\^/, ""));
    }
  }
  const lock = await readFile(join(root, "bun.lock"), "utf8");
  for (const p of platforms) {
    found.set(`bun.lock: ${platformName(p)}`, lock.match(lockedPlatform(p))?.[2] ?? "(none)");
  }
  return found;
}

if (import.meta.main) {
  const args = process.argv.slice(2);
  if (args.length === 2 && args[0] === "--check" && args[1]) {
    const want = args[1].replace(/^v/, "");
    const info = await releaseInfo();
    if (!validVersion(want) || !validVersion(info.upstreamVersion) || !/^[0-9a-f]{40}$/.test(info.upstreamRevision)) {
      throw new Error("expected plain SemVer versions and a full upstream commit SHA");
    }
    const wrong = [...(await versions())].filter(([, v]) => v !== want);
    for (const [where, v] of wrong) console.error(`${where} has ${v}, not ${want}`);
    if (wrong.length > 0) process.exit(1);
    console.log(`evil-mygo ${want}; upstream MyGo ${info.upstreamVersion} @ ${info.upstreamRevision}`);
  } else if (args.length === 3 && args[0] === "--upstream" && validVersion(args[1]!) && /^[0-9a-f]{40}$/.test(args[2]!)) {
    await setGoConstant("UpstreamVersion", args[1]!);
    await setGoConstant("UpstreamRevision", args[2]!);
    console.log(`recorded upstream MyGo ${args[1]} @ ${args[2]}; fork release version unchanged`);
  } else if (args.length === 1 && validVersion(args[0]!)) {
    const v = args[0]!;
    await setGoConstant("Version", v);
    for (const dir of ["packages/runtime", ...pluginPackages]) {
      const file = join(root, dir, "package.json");
      const pkg = JSON.parse(await readFile(file, "utf8"));
      pkg.version = v;
      for (const deps of [pkg.peerDependencies, pkg.devDependencies]) {
        if (deps?.["@caelyreth/evil-mygo-runtime"]) deps["@caelyreth/evil-mygo-runtime"] = `^${v}`;
      }
      await writeFile(file, JSON.stringify(pkg, null, 2) + "\n");
    }
    await writeManifests(v);
    await updateLockfile(v);
    console.log(`set the version to ${v}: commit, then push the tag v${v} to release`);
  } else {
    console.error("usage: bun scripts/version.ts <version> | --check <tag> | --upstream <version> <full-commit>");
    process.exit(2);
  }
}
