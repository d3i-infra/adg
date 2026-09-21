#!/usr/bin/env node
"use strict";
// adg — the npm entry point for @d3i-infra/adg. npm installs exactly one of the
// optional @d3i-infra/adg-<os>-<cpu> packages (matched by its os/cpu fields);
// this shim finds it and runs the real binary. stdout, stderr, and the exit
// code pass through untouched (ADR-0008: machine output on stdout, status on
// stderr). No downloads, no postinstall.
const { spawnSync } = require("node:child_process");

const key = `${process.platform}-${process.arch}`;
const pkg = `@d3i-infra/adg-${key}`;
const ext = process.platform === "win32" ? ".exe" : "";

let bin;
try {
  bin = require.resolve(`${pkg}/bin/adg${ext}`);
} catch {
  process.stderr.write(
    `adg: no binary for ${key}: the platform package ${pkg} is not installed.\n` +
    `Reinstall with \`pnpm add -g @d3i-infra/adg\`, or use another install route: https://github.com/d3i-infra/adg#install\n`);
  process.exit(1);
}

const r = spawnSync(bin, process.argv.slice(2), { stdio: "inherit", windowsHide: true });
if (r.error) {
  process.stderr.write(`adg: ${r.error.message}\n`);
  process.exit(1);
}
if (r.signal) process.kill(process.pid, r.signal);
process.exit(r.status === null ? 1 : r.status);
