#!/usr/bin/env bash
# Fetch + build model artifacts of all-MiniLM-L6-v2.
#
#   tokenizer.json
#   config.json       # locally generated
#   transformer.onnx  # UINT8 quantized
#   tokenizer.onnx    # generated from tokenizer.json
#
# Source: https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2

set -euo pipefail

CWD="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(git -C "$CWD" rev-parse --show-toplevel)"  # strix/
cd "$REPO_ROOT"

SCRIPT_DIR="$REPO_ROOT/scripts/"

MODEL_DIR="$REPO_ROOT/model/all-MiniLM-L6-v2"
mkdir -p "$MODEL_DIR"

# Pin to a commit SHA for reproducibility ("main" is a mutable ref).
REVISION="${MINILM_REVISION:-main}"
BASE_URL="https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/$REVISION"

TOKENIZER_JSON="$MODEL_DIR/tokenizer.json"
CONFIG_JSON="$MODEL_DIR/config.json"
TRANSFORMER_ONNX="$MODEL_DIR/transformer.onnx"
TOKENIZER_ONNX="$MODEL_DIR/tokenizer.onnx"

TRANSFORMER_SHA256="b941bf19f1f1283680f449fa6a7336bb5600bdcd5f84d10ddc5cd72218a0fd21"
TOKENIZER_JSON_SHA256=""  # fill in once REVISION is pinned; empty = JSON-validity check only

TMP_FILES=()
cleanup() { [ "${#TMP_FILES[@]}" -gt 0 ] && rm -f "${TMP_FILES[@]}"; return 0; }
trap cleanup EXIT

log()  { echo "[INFO] $*" >&2; }
warn() { echo "[WARN] $*" >&2; }
die()  { echo "[ERROR] $*" >&2; exit 1; }

sha256_of() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{ print $1 }'
    else
        die "Cannot find sha256sum"
    fi
}

# download <url> <tmp_dest>
download() {
    curl -fSL --progress-bar --retry 3 --retry-connrefused --connect-timeout 15 -o "$2" "$1"
}

valid_json() { python3 -c 'import json, sys; json.load(open(sys.argv[1]))' "$1" >/dev/null 2>&1; }

command -v curl    >/dev/null 2>&1 || die "curl not found"
command -v python3 >/dev/null 2>&1 || die "python3 not found"
command -v uv      >/dev/null 2>&1 || die "uv not found"

# Step 1. tokenizer.json
tokenizer_json_ok() {
    [ -f "$TOKENIZER_JSON" ] && valid_json "$TOKENIZER_JSON" || return 1
    [ -z "$TOKENIZER_JSON_SHA256" ] || [ "$(sha256_of "$TOKENIZER_JSON")" = "$TOKENIZER_JSON_SHA256" ]
}

if tokenizer_json_ok; then
    log "tokenizer.json already valid, skipping"
else
    log "Downloading tokenizer.json"
    tmp="$TOKENIZER_JSON.part"; TMP_FILES+=("$tmp")
    download "$BASE_URL/tokenizer.json" "$tmp"
    valid_json "$tmp" || die "tokenizer.json is not valid JSON - file corrupted"
    if [ -n "$TOKENIZER_JSON_SHA256" ] && [ "$(sha256_of "$tmp")" != "$TOKENIZER_JSON_SHA256" ]; then
        die "tokenizer.json checksum mismatch"
    fi
    mv -f "$tmp" "$TOKENIZER_JSON"
fi

# Step 2. config.json (minimal: enough for AutoTokenizer to resolve BertTokenizerFast)
tmp="$CONFIG_JSON.part"; TMP_FILES+=("$tmp")
printf '{"model_type": "bert"}\n' > "$tmp"
mv -f "$tmp" "$CONFIG_JSON"

# Step 3. transformer.onnx
if [ -f "$TRANSFORMER_ONNX" ] && [ "$(sha256_of "$TRANSFORMER_ONNX")" = "$TRANSFORMER_SHA256" ]; then
    log "transformer.onnx already valid, skipping"
else
    log "Downloading transformer.onnx"
    tmp="$TRANSFORMER_ONNX.part"; TMP_FILES+=("$tmp")
    download "$BASE_URL/onnx/model_quint8_avx2.onnx" "$tmp"
    [ "$(sha256_of "$tmp")" = "$TRANSFORMER_SHA256" ] || die "transformer.onnx checksum mismatch - file corrupted"
    mv -f "$tmp" "$TRANSFORMER_ONNX"
fi

# Step 4. tokenizer.onnx (rebuilt only if missing or older than its inputs)
if [ -f "$TOKENIZER_ONNX" ] && [ "$TOKENIZER_ONNX" -nt "$TOKENIZER_JSON" ] && [ "$TOKENIZER_ONNX" -nt "$CONFIG_JSON" ]; then
    log "tokenizer.onnx up to date, skipping"
else
    log "Generating tokenizer.onnx"
    tmp="$TOKENIZER_ONNX.part"; TMP_FILES+=("$tmp")
    cd "$SCRIPT_DIR"
    HF_HUB_OFFLINE=1 uv run python - "$MODEL_DIR" "$tmp" <<'PY'
import sys
from pathlib import Path
 
import onnx
from onnxruntime_extensions import gen_processing_models
from transformers import AutoTokenizer
 
model_dir, out_tmp = Path(sys.argv[1]), sys.argv[2]
 
tokenizer = AutoTokenizer.from_pretrained(str(model_dir))
tok_model, _ = gen_processing_models(tokenizer, pre_kwargs={})
if tok_model is None:
    sys.exit("gen_processing_models returned None - tokenizer.json / config.json invalid?")
 
onnx.save(tok_model, out_tmp)
print("Inputs :", [i.name for i in tok_model.graph.input], file=sys.stderr)
print("Outputs:", [o.name for o in tok_model.graph.output], file=sys.stderr)
PY
    [ -s "$tmp" ] || die "tokenizer.onnx generation produced no output"
    mv -f "$tmp" "$TOKENIZER_ONNX"
fi
 
log "Done. Artifacts in $MODEL_DIR"
