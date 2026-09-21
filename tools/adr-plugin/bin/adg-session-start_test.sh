#!/usr/bin/env sh
# Runs adg-session-start.sh with adg missing from PATH and asserts the install
# advice names each package manager. Exit 1 on the first failed assertion.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/repo/docs/decisions" "$tmp/plugin/.claude-plugin" "$tmp/emptybin"
cp "$here/adg-session-start.sh" "$tmp/plugin/"
printf '{"name":"write-adr","version":"9.9.9"}\n' > "$tmp/plugin/.claude-plugin/plugin.json"

out=$(cd "$tmp/repo" && PATH="$tmp/emptybin:/usr/bin:/bin" CLAUDE_PLUGIN_ROOT="$tmp/plugin" sh "$tmp/plugin/adg-session-start.sh")

fail() { echo "FAIL: $1" >&2; echo "$out" >&2; exit 1; }
printf '%s' "$out" | grep -q '"systemMessage"' || fail "missing adg should produce the JSON envelope"
printf '%s' "$out" | grep -q 'pnpm add -g @d3i-infra/adg' || fail "no npm line"
printf '%s' "$out" | grep -q 'adg-bin' || fail "no AUR line"
printf '%s' "$out" | grep -q 'raw.githubusercontent.com/d3i-infra/adg/main/install.sh' || fail "no fallback line"
printf '%s' "$out" | grep -q 'rides along' && fail "wrapper wording must be gone"
printf '%s' "$out" | python3 -c 'import json,sys; json.loads(sys.stdin.read())' || fail "output is not valid JSON"

# Version-compare cases: plugin.json ships for 4.0.0; an installed prerelease (4.0.0-rc1)
# must count as older than its release, not newer (GNU sort -V ranks 4.0.0-rc1 after 4.0.0).
mkdir -p "$tmp/plugin4/.claude-plugin"
cp "$here/adg-session-start.sh" "$tmp/plugin4/"
printf '{"name":"write-adr","version":"4.0.0"}\n' > "$tmp/plugin4/.claude-plugin/plugin.json"

run_with_version() {
    ver="$1"
    mkdir -p "$tmp/fakebin"
    printf '#!/bin/sh\necho "adg %s"\n' "$ver" > "$tmp/fakebin/adg"
    chmod +x "$tmp/fakebin/adg"
    (cd "$tmp/repo" && PATH="$tmp/fakebin:/usr/bin:/bin" CLAUDE_PLUGIN_ROOT="$tmp/plugin4" sh "$tmp/plugin4/adg-session-start.sh")
}

out=$(run_with_version "3.9.0")
printf '%s' "$out" | grep -q '"systemMessage"' || fail "installed 3.9.0 should produce the JSON envelope"
printf '%s' "$out" | grep -q 'pnpm add -g @d3i-infra/adg@latest' || fail "installed 3.9.0 should advise the upgrade command"

out=$(run_with_version "4.0.0-rc1")
printf '%s' "$out" | grep -q '"systemMessage"' || fail "installed 4.0.0-rc1 (prerelease of the shipped version) should produce the JSON envelope"
printf '%s' "$out" | grep -q 'pnpm add -g @d3i-infra/adg@latest' || fail "installed 4.0.0-rc1 should advise the upgrade command"

out=$(run_with_version "4.0.0")
printf '%s' "$out" | grep -q '"systemMessage"' && fail "installed 4.0.0 matches what the plugin ships for; no upgrade advice expected"

echo "ok"
