#!/usr/bin/env node
// Assemble the npm packages for a release from goreleaser's dist/ output.
//   node scripts/npm/build.mjs [dist-dir] [out-dir]     (defaults: dist, dist/npm)
// Reads dist/artifacts.json (entries of type "Binary": goos, goarch, path) and
// dist/metadata.json (version). Writes one directory per package under out-dir:
//   adg/                @d3i-infra/adg            shim + optionalDependencies on the six below
//   adg-<os>-<cpu>/     @d3i-infra/adg-<os>-<cpu> one binary, os/cpu fields
// The release workflow then runs `npm publish` on each. Pure file assembly: no
// network, no scripts in any package.json.
import { readFileSync, mkdirSync, copyFileSync, writeFileSync, chmodSync, existsSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(here, "..", "..");
const SCOPE = "@d3i-infra";
const HOMEPAGE = "https://github.com/d3i-infra/adg";
const DESCRIPTION = "Compile Architectural Decision Records into per-change architecture briefs for coding agents";
const NODE_OS = { darwin: "darwin", linux: "linux", windows: "win32" };
const NODE_CPU = { amd64: "x64", arm64: "arm64" };
// npm's provenance check refuses a publish whose repository.url does not match the
// repository the workflow runs in, so take it from Actions' GITHUB_REPOSITORY (which
// is d3i-infra/adg for every real release; the fallback serves local runs and tests).
const repoSlug = process.env.GITHUB_REPOSITORY || "d3i-infra/adg";
const common = {
  license: "Apache-2.0",
  homepage: HOMEPAGE,
  repository: { type: "git", url: `git+https://github.com/${repoSlug}.git` },
  publishConfig: { access: "public" },
};

export function platformPackages(artifacts, version) {
  return artifacts
    // goreleaser's artifacts.json carries TWO kinds of type:"Binary" row per
    // platform: the compiled build output (extra.Builder: "go") we want, and a
    // raw-binary upload row from the `formats: [binary]` archive (extra.Format:
    // "binary") that points at the same path but is not itself a build we
    // should package. Filter to the builder rows so each platform contributes
    // exactly one package.
    .filter((a) => a.type === "Binary" && a.extra?.Builder === "go")
    .map((a) => {
      const os = NODE_OS[a.goos];
      const cpu = NODE_CPU[a.goarch];
      if (!os || !cpu) throw new Error(`unmapped platform ${a.goos}/${a.goarch}`);
      return { name: `${SCOPE}/adg-${os}-${cpu}`, os, cpu, ext: os === "win32" ? ".exe" : "", source: a.path, version };
    });
}

export function platformManifest(p) {
  return {
    name: p.name,
    version: p.version,
    description: `${DESCRIPTION} (${p.os}-${p.cpu} binary for @d3i-infra/adg)`,
    ...common,
    os: [p.os],
    cpu: [p.cpu],
    files: ["bin/"],
  };
}

export function mainManifest(platforms, version) {
  return {
    name: `${SCOPE}/adg`,
    version,
    description: DESCRIPTION,
    ...common,
    bin: { adg: "bin/adg.cjs" },
    files: ["bin/"],
    engines: { node: ">=18" },
    optionalDependencies: Object.fromEntries(platforms.map((p) => [p.name, version])),
  };
}

function writeJson(path, obj) { writeFileSync(path, JSON.stringify(obj, null, 2) + "\n"); }

export function build(distDir, outDir) {
  const artifactsPath = join(distDir, "artifacts.json");
  if (!existsSync(artifactsPath)) throw new Error(`${artifactsPath} not found: run goreleaser first`);
  const artifacts = JSON.parse(readFileSync(artifactsPath, "utf8"));
  const { version } = JSON.parse(readFileSync(join(distDir, "metadata.json"), "utf8"));
  const platforms = platformPackages(artifacts, version);
  if (platforms.length !== 6) throw new Error(`expected 6 platform binaries, found ${platforms.length}`);
  const license = join(repoRoot, "LICENSE");

  for (const p of platforms) {
    const dir = join(outDir, p.name.split("/")[1]);
    mkdirSync(join(dir, "bin"), { recursive: true });
    const target = join(dir, "bin", `adg${p.ext}`);
    copyFileSync(p.source, target);
    chmodSync(target, 0o755);
    copyFileSync(license, join(dir, "LICENSE"));
    writeJson(join(dir, "package.json"), platformManifest(p));
    writeFileSync(join(dir, "README.md"),
      `# ${p.name}\n\nThe ${p.os}-${p.cpu} binary for [@d3i-infra/adg](https://www.npmjs.com/package/@d3i-infra/adg). Install that package, not this one; npm selects this one automatically for your platform.\n`);
  }

  const main = join(outDir, "adg");
  mkdirSync(join(main, "bin"), { recursive: true });
  copyFileSync(join(here, "adg.cjs"), join(main, "bin", "adg.cjs"));
  chmodSync(join(main, "bin", "adg.cjs"), 0o755);
  copyFileSync(license, join(main, "LICENSE"));
  writeFileSync(join(main, "README.md"),
    `# @d3i-infra/adg\n\n${DESCRIPTION}.\n\n` +
    "```sh\npnpm add -g @d3i-infra/adg   # or: npm install -g @d3i-infra/adg\nadg --version\n```\n\n" +
    "This package is a thin launcher; the binary for your platform arrives through one of the\n" +
    "optional `@d3i-infra/adg-<os>-<cpu>` packages. Documentation, other install routes, and the\n" +
    `source: ${HOMEPAGE}\n`);
  writeJson(join(main, "package.json"), mainManifest(platforms, version));
  return { version, platforms };
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const [distDir = "dist", outDir = "dist/npm"] = process.argv.slice(2);
  const r = build(distDir, outDir);
  process.stderr.write(`assembled ${r.platforms.length + 1} npm packages at ${r.version} in ${outDir}\n`);
}
