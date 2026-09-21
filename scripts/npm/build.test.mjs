import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, writeFileSync, chmodSync, statSync, readFileSync, existsSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";
import { platformPackages, mainManifest, platformManifest, build } from "./build.mjs";

const here = dirname(fileURLToPath(import.meta.url));
const VERSION = "9.9.9";
const GO = [["linux","amd64"],["linux","arm64"],["darwin","amd64"],["darwin","arm64"],["windows","amd64"],["windows","arm64"]];

// Real goreleaser output carries TWELVE type:"Binary" entries per release: the six
// compiled build outputs (extra.Builder: "go") and six raw-binary upload rows from
// the `formats: [binary]` archive (extra.Format: "binary", extra.Checksum) that
// point at the same path. fakeDist() reproduces both kinds so platformPackages'
// filter is actually exercised, not merely satisfied by a fixture that only ever
// had six rows.
function fakeDist() {
  const dist = mkdtempSync(join(tmpdir(), "adg-dist-"));
  const artifacts = GO.flatMap(([goos, goarch]) => {
    const dir = join(dist, `adg_${goos}_${goarch}_v1`); mkdirSync(dir);
    const ext = goos === "windows" ? ".exe" : "";
    const path = join(dir, `adg${ext}`);
    writeFileSync(path, `#!/bin/sh\necho "adg version ${VERSION}"\nexit 3\n`); chmodSync(path, 0o755);
    return [
      { name: `adg_${goos}_${goarch}`, path, goos, goarch, type: "Binary", extra: { Binary: "adg", Builder: "go", Ext: ext, ID: "adg" } },
      { name: `adg_${goos}_${goarch}`, path, goos, goarch, type: "Binary", extra: { Binary: "adg", Format: "binary", Checksum: "sha256:0", ID: "adg" } },
    ];
  });
  artifacts.push({ name: "checksums.txt", path: join(dist, "checksums.txt"), type: "Checksum" });
  writeFileSync(join(dist, "artifacts.json"), JSON.stringify(artifacts));
  writeFileSync(join(dist, "metadata.json"), JSON.stringify({ version: VERSION }));
  return dist;
}

test("platformPackages maps Go platforms to npm os/cpu and skips non-binaries", () => {
  const dist = fakeDist();
  const ps = platformPackages(JSON.parse(readFileSync(join(dist, "artifacts.json"), "utf8")), VERSION);
  assert.equal(ps.length, 6);
  assert.deepEqual(ps.map((p) => p.name).sort(), [
    "@d3i-infra/adg-darwin-arm64", "@d3i-infra/adg-darwin-x64",
    "@d3i-infra/adg-linux-arm64", "@d3i-infra/adg-linux-x64",
    "@d3i-infra/adg-win32-arm64", "@d3i-infra/adg-win32-x64",
  ]);
  assert.equal(ps.find((p) => p.os === "win32").ext, ".exe");
  assert.throws(() => platformPackages([{ type: "Binary", goos: "plan9", goarch: "amd64", path: "x", extra: { Builder: "go" } }], VERSION), /unmapped/);
});

test("manifests pin every platform package to the exact version, public, no scripts", () => {
  const ps = platformPackages(JSON.parse(readFileSync(join(fakeDist(), "artifacts.json"), "utf8")), VERSION);
  const main = mainManifest(ps, VERSION);
  assert.equal(main.name, "@d3i-infra/adg");
  assert.equal(main.version, VERSION);
  assert.deepEqual(main.bin, { adg: "bin/adg.cjs" });
  assert.equal(Object.keys(main.optionalDependencies).length, 6);
  assert.ok(Object.values(main.optionalDependencies).every((v) => v === VERSION));
  assert.equal(main.publishConfig.access, "public");
  assert.equal(main.scripts, undefined);
  const plat = platformManifest(ps.find((p) => p.name === "@d3i-infra/adg-linux-x64"));
  assert.deepEqual(plat.os, ["linux"]);
  assert.deepEqual(plat.cpu, ["x64"]);
  assert.equal(plat.scripts, undefined);
  assert.equal(main.repository.url, `git+https://github.com/${process.env.GITHUB_REPOSITORY || "d3i-infra/adg"}.git`);
});

test("repository.url follows GITHUB_REPOSITORY so npm provenance verifies before and after the move", () => {
  const r = spawnSync(process.execPath, ["--input-type=module", "-e",
    `import { mainManifest } from ${JSON.stringify(join(here, "build.mjs"))}; console.log(mainManifest([], "1.0.0").repository.url)`],
    { env: { ...process.env, GITHUB_REPOSITORY: "someone/elsewhere" }, encoding: "utf8" });
  assert.equal(r.stdout.trim(), "git+https://github.com/someone/elsewhere.git");
});

test("build writes seven packages with executable binaries", () => {
  const dist = fakeDist();
  const out = mkdtempSync(join(tmpdir(), "adg-npm-"));
  const r = build(dist, out);
  assert.equal(r.version, VERSION);
  assert.equal(r.platforms.length, 6);
  for (const p of r.platforms) {
    const dir = join(out, p.name.split("/")[1]);
    assert.ok(existsSync(join(dir, "package.json")), `${dir}/package.json`);
    assert.ok(existsSync(join(dir, "LICENSE")));
    assert.ok(existsSync(join(dir, "README.md")), `${dir}/README.md`);
    assert.ok(statSync(join(dir, "bin", `adg${p.ext}`)).mode & 0o111, "binary is executable");
  }
  assert.ok(existsSync(join(out, "adg", "bin", "adg.cjs")));
  assert.ok(existsSync(join(out, "adg", "LICENSE")));
  assert.ok(existsSync(join(out, "adg", "README.md")));
  assert.throws(() => build(mkdtempSync(join(tmpdir(), "adg-empty-")), out), /artifacts.json/);
});

test("shim resolves the platform package and passes through stdio and exit code", () => {
  const root = mkdtempSync(join(tmpdir(), "adg-shim-"));
  const key = `${process.platform}-${process.arch}`;
  const pkgDir = join(root, "node_modules", "@d3i-infra", `adg-${key}`, "bin"); mkdirSync(pkgDir, { recursive: true });
  const bin = join(pkgDir, "adg");
  writeFileSync(bin, `#!/bin/sh\necho "out:$@"\necho "err" >&2\nexit 3\n`); chmodSync(bin, 0o755);
  const r = spawnSync(process.execPath, [join(here, "adg.cjs"), "lean", "--quiet"], { env: { ...process.env, NODE_PATH: join(root, "node_modules") }, encoding: "utf8" });
  assert.equal(r.status, 3);
  assert.equal(r.stdout, "out:lean --quiet\n");
  assert.equal(r.stderr, "err\n");
  const missing = spawnSync(process.execPath, [join(here, "adg.cjs"), "--version"], { env: { ...process.env, NODE_PATH: mkdtempSync(join(tmpdir(), "adg-none-")) }, encoding: "utf8" });
  assert.equal(missing.status, 1);
  assert.match(missing.stderr, new RegExp(`adg-${key}`));
});
