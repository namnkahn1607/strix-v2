#!/usr/bin/env bash
# PostToolUse: format the code file agent just wrote (gofmt / clang-format).

set -u
trap 'printf "{}\n"' EXIT
log() { echo "$*" >&2; }

command -v jq >/dev/null || { log "jq missing"; exit 0; }
command -v realpath >/dev/null || { log "realpath missing"; exit 0; }
payload=$(cat)

name=$(jq -r '.toolCall.name // ""' <<<"$payload" 2>/dev/null)
raw=$(jq -r '.toolCall.args.TargetFile // ""' <<<"$payload" 2>/dev/null)
[ -n "$raw" ] || { log "no TargetFile for tool '$name' (schema drift?)"; exit 0; }
 
conv=$(jq -r '.conversationId // "unknown"' <<<"$payload" 2>/dev/null); conv=${conv//[^A-Za-z0-9-]/_}
mapfile -t roots < <(jq -r '.workspacePaths[]?' <<<"$payload" 2>/dev/null)
if [ "${#roots[@]}" -eq 0 ]; then   # fallback: git toplevel
    roots=("$(git -C "$(dirname "$raw")" rev-parse --show-toplevel 2>/dev/null || true)")
fi
 
# Relative paths are taken relative to the first workspace root.
case "$raw" in /*) ;; *) raw="${roots[0]:-.}/$raw"; log "relative path resolved: $raw" ;; esac
 
# Canonicalize, then require it to sit inside a workspace root.
file=$(realpath -m -- "$raw") || exit 0
root=""
for r in "${roots[@]}"; do
    [ -n "$r" ] || continue
    rr=$(realpath -m -- "$r")
    case "$file" in "$rr"/*) root=$rr; break ;; esac
done
[ -n "$root" ] || { log "outside workspace, skipped: $file"; exit 0; }
[ -f "$file" ] || exit 0
case "/${file#"$root"/}" in */vendor/*|*/third_party/*|*/build/*|*/.git/*) exit 0 ;; esac
 
before=$(cksum <"$file")
case "$file" in
  *.go)
    command -v gofmt >/dev/null || exit 0
    gofmt -w "$file" >&2 ;;
  *.c|*.cc|*.cpp|*.cxx|*.h|*.hh|*.hpp|*.hxx)
    command -v clang-format >/dev/null || exit 0
    # No fallback (LLVM) style: without .clang-format, do nothing.
    clang-format -i --style=file --fallback-style=none "$file" >&2 ;;
  *) exit 0 ;;
esac
after=$(cksum <"$file")
 
if [ "$before" != "$after" ]; then
  d="${TMPDIR:-/tmp}/agy-reformat-$(id -u)"
  mkdir -p "$d" && printf '%s\n' "$file" >> "$d/$conv"
fi
exit 0
