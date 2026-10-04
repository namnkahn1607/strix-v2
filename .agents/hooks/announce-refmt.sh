#!/usr/bin/env bash
# PreInvocation: if auto-formatters rewrote files since the last agent call, tell the agent
# (ephemeralMessage) so it re-reads instead of editing from a stale in-context copy.

set -u
command -v jq >/dev/null || { echo '{}'; exit 0; }
payload=$(cat)
conv=$(jq -r '.conversationId // "unknown"' <<<"$payload" 2>/dev/null); conv=${conv//[^A-Za-z0-9-]/_}
f="${TMPDIR:-/tmp}/agy-reformat-$(id -u)/$conv"
if [ -s "$f" ]; then
    list=$(sort -u "$f" | head -n 10 | paste -sd, -)
    rm -f "$f"
    jq -n --arg m "Auto-formatters (gofmt/clang-format) rewrote these files on disk after your last write, so your in-context copy is stale: $list. Re-read them before any further edit." \
    '{injectSteps:[{ephemeralMessage:$m}]}'
else
    echo '{}'
fi
