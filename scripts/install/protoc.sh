#!/usr/bin/env bash
# Install a vendored protoc binary.
#
# Fetches port version based on builtin-baseline hash at engine/vcpkg.json.
# The version tag is written to .protoc-version post installation.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(git -C "$SCRIPT_DIR" rev-parse --show-toplevel)"  # strix/
cd "$REPO_ROOT"

VCPKG_JSON="$REPO_ROOT/engine/vcpkg.json"
VENDOR_DIR="$REPO_ROOT/tools/protoc"
VERSION_FILE="$REPO_ROOT/.protoc-version"

log()  { echo "[INFO] $*" >&2; }
warn() { echo "[WARN] $*" >&2; }
die()  { echo "[ERROR] $*" >&2; exit 1; }

for tool in curl python3 unzip; do
    command -v "$tool" >/dev/null 2>&1 || die "Required '$tool' not found"
done

[[ -f "$VCPKG_JSON" ]] || die "$VCPKG_JSON not found"

# Step 1. Read vcpkg baseline commit hash
BASELINE_HASH=$(grep -oP '"builtin-baseline"\s*:\s*"\K[a-f0-9]{40}' "$VCPKG_JSON") \
    || die "Could not find builtin-baseline in $VCPKG_JSON"
log "builtin-baseline: $BASELINE_HASH"

# Step 2. Fetch protobuf port version at that vcpkg baseline
fetch_protobuf_version() {
    local hash="$1"
    local url="https://raw.githubusercontent.com/microsoft/vcpkg/${hash}/versions/baseline.json"

    local raw_version
    raw_version="$(curl -fsSL "$url" | python3 -c '
import json, sys
data = json.load(sys.stdin)
print(data["default"]["protobuf"]["baseline"])
')" || die "Failed to fetch/parse vcpkg baseline.json at commit ${hash}. Is the hash valid and reachable?"

    # Strip vcpkg-internal revision suffix (e.g. "33.4.0#1" -> "33.4.0").
    clean_version="${raw_version%%#*}"

    # Strip language major version (e.g. "6.33.4" -> "33.4").
    echo "${clean_version#*.}"
}

TARGET_VERSION="$(fetch_protobuf_version "$BASELINE_HASH")"
log "Baseline protobuf port version: $TARGET_VERSION"

# Step 3. Resolve the matching protoc release tag on GitHub
resolve_protoc_release_tag() {
    local version="$1"
    local candidates=("v${version}")
    [[ "$version" == *.0 ]] && candidates+=("v${version%.0}")

    local tag http_code
    for tag in "${candidates[@]}"; do
        http_code="$(curl -fsSL -o /dev/null -w '%{http_code}' \
            "https://api.github.com/repos/protocolbuffers/protobuf/releases/tags/${tag}" || true)"
        if [[ "$http_code" == "200" ]]; then
            echo "$tag"
            return 0
        fi
    done

    die "Could not resolve GitHub release tag for protobuf ${version}. Tried: ${candidates[*]}. 
         Check https://github.com/protocolbuffers/protobuf/releases manually."
}

RELEASE_TAG="$(resolve_protoc_release_tag "$TARGET_VERSION")"
log "Protoc release tag: $RELEASE_TAG"

# Step 4. Skip fetching if already installed & version matched
PROTOC_BIN="$VENDOR_DIR/bin/protoc"
CURRENT_VERSION=""
if [[ -x "$PROTOC_BIN" ]]; then
    CURRENT_VERSION="$("$PROTOC_BIN" --version | awk '{print $2}')"
fi

if [[ "$CURRENT_VERSION" == "$TARGET_VERSION" ]]; then
    log "protoc $TARGET_VERSION already installed and matches builtin-baseline. Skipping fetch."
else
    log "Installing protoc $TARGET_VERSION (current: '${CURRENT_VERSION:-none}')"

    ARCHIVE="protoc-${TARGET_VERSION}-linux-x86_64.zip"
    mkdir -p "$VENDOR_DIR"
    curl -fsSL -o "/tmp/$ARCHIVE" \
        "https://github.com/protocolbuffers/protobuf/releases/download/${RELEASE_TAG}/${ARCHIVE}" \
        || die "Failed to download $ARCHIVE from tag $RELEASE_TAG"

    rm -rf "${VENDOR_DIR:?}/bin" "$VENDOR_DIR/include"
    unzip -q "/tmp/$ARCHIVE" -d "$VENDOR_DIR" 'bin/protoc' 'include/*'
    rm -f "/tmp/$ARCHIVE"
    chmod +x "$PROTOC_BIN"

    INSTALLED_VERSION="$("$PROTOC_BIN" --version | awk '{print $2}')"
    [[ "$INSTALLED_VERSION" == "$TARGET_VERSION" ]] \
        || die "Post-install verification failed: installed '$INSTALLED_VERSION', expected '$TARGET_VERSION'."
    log "Installed protoc $INSTALLED_VERSION at $PROTOC_BIN"
fi

# Step 5. Pin protoc version to .protoc-version
EXISTING_PIN="$(cat "$VERSION_FILE" 2>/dev/null || true)"
if [[ "$EXISTING_PIN" != "$TARGET_VERSION" ]]; then
    echo "$TARGET_VERSION" > "$VERSION_FILE"
    log "Updated $VERSION_FILE -> $TARGET_VERSION (was: '${EXISTING_PIN:-none}')"
else
    log "$VERSION_FILE already up to date ($TARGET_VERSION)."
fi

log "Finished!"
