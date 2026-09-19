#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"  # strix/

log()  { echo "[INFO] $*" >&2; }
warn() { echo "[WARN] $*" >&2; }
die()  { echo "[ERROR] $*" >&2; exit 1; }


PROTOC_PLUGIN_URL="google.golang.org/protobuf/cmd/protoc-gen-go"
GRPC_PLUGIN_URL="google.golang.org/grpc/cmd/protoc-gen-go-grpc"

BINARY_DIR="build"
PROTOC_PLUGIN="$BINARY_DIR/protoc-gen-go"
GRPC_PLUGIN="$BINARY_DIR/protoc-gen-go-grpc"

PROTO_DEF_DIR="../proto/v1"
PROTO_DEF_FILE="$PROTO_DEF_DIR/cache.proto"
PROTO_GEN_DIR="internal/gateway/pb"

gen_grpc_stubs() {
    log "Install and build gRPC plugins"
    go get -tool "$PROTOC_PLUGIN_URL"@latest
    go get -tool "$GRPC_PLUGIN_URL"@latest

    mkdir -p "$BINARY_DIR"
    go build -o "$PROTOC_PLUGIN" "$PROTOC_PLUGIN_URL"
    go build -o "$GRPC_PLUGIN" "$GRPC_PLUGIN_URL"

    local protoc_exec
    protoc_exec="$REPO_ROOT/tools/protoc/bin/protoc"
    if [[ ! -x "$protoc_exec" ]]; then
        die "Protobuf executable not found at $protoc_exec.
            Run 'bash scripts/install/protoc.sh' first"
    fi

    log "Generate gRPC stubs"
    mkdir -p "$PROTO_GEN_DIR"

    "$protoc_exec" \
        --plugin=protoc-gen-go="$PROTOC_PLUGIN" \
        --plugin=protoc-gen-go-grpc="$GRPC_PLUGIN" \
        --go_out="$PROTO_GEN_DIR" --go_opt=paths=source_relative \
        --go-grpc_out="$PROTO_GEN_DIR" --go-grpc_opt=paths=source_relative \
        --proto_path "$PROTO_DEF_DIR" "$PROTO_DEF_FILE"
}

main() {
    gen_grpc_stubs
}

main "$@"
