#!/usr/bin/env sh
# adg-session-start — SessionStart hook. Two model-independent jobs, so a session knows
# the governance exists even before any lean record does (the whole-corpus brief is empty
# until records carry applies_to, and a read-only/exploring session meets no other hook):
#   1. Greet — when this repo has an ADR model (docs/decisions/), announce that the
#      write-adr governance is active and where its entry points are.
#   2. Version check — if the system `adg` is missing or older than this plugin ships for,
#      surface the install one-liner.
# Plain stdout is added to the agent's context. When the *user* has to act (install or
# update adg) the output switches to the JSON envelope so the advice also reaches them
# directly as a systemMessage — the bundled hooks fail loudly without adg (deliberate:
# governance must not degrade silently), and this message turns that noise into
# instructions. Fail-open: any error prints nothing.
set -eu

# The plugin is global; only greet where an ADR model actually exists.
[ -d docs/decisions ] || exit 0

ctx="This repo is governed by the write-adr plugin: architecture decisions live as lean ADRs in docs/decisions/, enforced by \`adg\` and Claude Code hooks. Entry points — pull the brief for files you'll touch (\`adg lean brief --model docs/decisions <paths>\`); author / migrate / review records with the write-lean-adr skill (\`adg lean new\`); obey an injected brief with follow-adr-governance. If the routing hooks stay silent, the lean model may not be populated yet (records need \`applies_to\` frontmatter) — bootstrap the lean records before relying on the brief."

# Permission check — the agent hooks (commit judge, record review) run `adg` in a
# subagent whose Bash calls consult the project allowlist; a denied call deadlocks the
# hook agent until its timeout. Warn once when no adg allow rule is visible.
if ! grep -qs '"Bash(adg' .claude/settings.json .claude/settings.local.json 2>/dev/null; then
    ctx="$ctx
NOTE: the commit-compliance judge and record-review agent hooks need \`adg\` allowed for subagent Bash calls. Add \"Bash(adg:*)\" to permissions.allow in .claude/settings.json (or settings.local.json), or the judge silently times out instead of judging."
fi

# Version check.
# Install advice, one line per platform. The plugin no longer downloads adg itself:
# adg is a system dependency installed through a package manager (see the repo README).
install='pnpm add -g @d3i-infra/adg (or: npm install -g @d3i-infra/adg) · Arch: yay -S adg-bin (not yet available) · Go: go install github.com/d3i-infra/adg/v4@latest · no package manager: curl -fsSL https://raw.githubusercontent.com/d3i-infra/adg/main/install.sh | sh'
root="${CLAUDE_PLUGIN_ROOT:-}"
need=""
if [ -n "$root" ] && [ -f "$root/.claude-plugin/plugin.json" ]; then
    need=$(sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$root/.claude-plugin/plugin.json" 2>/dev/null | head -1) || need=""
fi
have=$(adg --version 2>/dev/null | awk '{print $NF}') || have=""

msg=""
if [ -z "$have" ]; then
    msg="The write-adr plugin needs the adg CLI, which is not installed — this repo's governance hooks will keep erroring until it is. Install it with your package manager, then start a new session (or /clear). $install"
    ctx="$ctx
NOTE: the \`adg\` CLI these hooks depend on is not on PATH — the governance hooks will keep erroring visibly (\`adg: command not found\`) until it is installed, and nothing is being checked in the meantime. The user has been shown the install commands; if they ask about the hook errors, point them at the line for their platform: $install"
elif [ -n "$need" ] && [ "$have" != "$need" ]; then
    # Compare version cores; a prerelease (4.0.0-rc1) counts as older than its release (4.0.0).
    have_core=${have%%-*}; need_core=${need%%-*}
    older=$(printf '%s\n%s\n' "$have_core" "$need_core" | sort -V | head -1)
    if [ "$older" = "$have_core" ] && { [ "$have_core" != "$need_core" ] || [ "$have" != "$have_core" ]; }; then
        msg="adg is v$have but the write-adr plugin ships for v$need — the governance hooks misbehave on the old version. Upgrade with your package manager (pnpm add -g @d3i-infra/adg@latest · yay -Syu · go install github.com/d3i-infra/adg/v4@latest)."
        ctx="$ctx
NOTE: the system \`adg\` is v$have but this plugin ships for v$need — the governance hooks misbehave on the old version. The user has been shown the upgrade commands (pnpm add -g @d3i-infra/adg@latest · yay -Syu · go install github.com/d3i-infra/adg/v4@latest)."
    fi
fi

# No user action needed: plain stdout (context injection only), as before.
if [ -z "$msg" ]; then
    printf '%s\n' "$ctx"
    exit 0
fi

# User action needed: JSON envelope — systemMessage shows to the user, context to the agent.
esc() {
    printf '%s' "$1" | awk 'BEGIN{ORS=""} NR>1{printf "\\n"} {gsub(/\\/,"\\\\"); gsub(/"/,"\\\""); printf "%s", $0}'
}
printf '{"systemMessage":"%s","hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"%s"}}\n' \
    "$(esc "$msg")" "$(esc "$ctx")"
exit 0
