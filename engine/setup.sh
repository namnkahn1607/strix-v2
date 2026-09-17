#!/usr/bin/env bash
# Setup environment for local development in strix/engine.

set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"  # strix/
PROJECT_ROOT="$REPO_ROOT/engine"
cd "$PROJECT_ROOT"

log()  { echo "[INFO] $*" >&2; }
warn() { echo "[WARN] $*" >&2; }
die()  { echo "[ERROR] $*" >&2; exit 1; }


CMAKE_MIN_MAJOR=3
CMAKE_MIN_MINOR=28

check_cmake() {
    log "Check CMake compatibility"

    if command -v cmake >/dev/null 2>&1; then
        local curr_ver curr_major curr_minor
        curr_ver="$(cmake --version | head -1 | grep -oP '\d+\.\d+\.\d+')"
        curr_major="${curr_ver%%.*}"
        curr_minor="$(echo "$curr_ver" | cut -d. -f2)"

        if (( curr_major < CMAKE_MIN_MAJOR || (curr_major == CMAKE_MIN_MAJOR && curr_minor < CMAKE_MIN_MINOR) )); then
            die "Incompatible CMake $curr_ver (need >= $CMAKE_MIN_MAJOR.$CMAKE_MIN_MINOR)"
        fi
    else
        die "Error detecting CMake (need >= $CMAKE_MIN_MAJOR.$CMAKE_MIN_MINOR)"
    fi

    cmake_bin="$(command -v cmake)" || die "Cannot resolve CMake absolute path"
    echo "$cmake_bin"
}


ORT_PATCH_DIR="$PROJECT_ROOT/vendor/patches/onnxruntime"
ORT_SRC_DIR="$PROJECT_ROOT/vendor/onnxruntime"
ORT_HARVEST_DIR="$PROJECT_ROOT/ort"
ORT_BUILT_MARKER="$ORT_HARVEST_DIR/.built_commit"

bootstrap_ort() {
    log "Applying Clang 18 compatibility patch series"
    shopt -s nullglob
    local patches=("$ORT_PATCH_DIR"/*.diff)
    shopt -u nullglob

    if [[ ${#patches[@]} -eq 0 ]]; then
        warn "No patches found at $ORT_PATCH_DIR. Skipping patch"
    else
        for p in "${patches[@]}"; do
            git -C "$ORT_SRC_DIR" apply --check "$p" || die "Patch $p failed"
        done
        for p in "${patches[@]}"; do
            git -C "$ORT_SRC_DIR" apply "$p"
        done
    fi

    local cmake_bin
    cmake_bin="$(check_cmake)"

    log "Building onnxruntime. Might take a while"
    (
        cd "$ORT_SRC_DIR"
        export CC=/usr/bin/clang-18
        export CXX=/usr/bin/clang++-18
        ./build.sh \
            --cmake_path "$cmake_bin" \
            --cmake_generator Ninja \
            --config Release \
            --build_shared_lib \
            --parallel 4 \
            --target onnxruntime \
            --skip_tests \
            --skip_submodule_sync \
            --compile_no_warning_as_error \
            --use_extensions \
            --cmake_extra_defines \
                CMAKE_CXX_FLAGS="-Wno-error -O3 -march=x86-64-v3" \
                CMAKE_C_FLAGS="-Wno-error -O3 -march=x86-64-v3" \
                onnxruntime_BUILD_UNIT_TESTS=OFF \
                onnxruntime_ENABLE_PYTHON=OFF \
                onnxruntime_USE_TELEMETRY=OFF \
                onnxruntime_ENABLE_TRAINING=OFF \
    ) || die "Build failed."

    log "Harvesting artifacts & onnxruntime headers"
    mkdir -p "$ORT_HARVEST_DIR/lib" "$ORT_HARVEST_DIR/include"
    cp -a "$ORT_SRC_DIR"/build/Linux/Release/libonnxruntime.so* "$ORT_HARVEST_DIR/lib/"
    cp -a "$ORT_SRC_DIR"/include/onnxruntime/core/session/. "$ORT_HARVEST_DIR/include/"
    
    local ort_actual_commit
    ort_actual_commit="$(git -C "$ORT_SRC_DIR" rev-parse HEAD)"
    echo "$ort_actual_commit" > "$ORT_BUILT_MARKER"

    log "Revert to pristine state after applying patches"
    git -C "$ORT_SRC_DIR" reset --hard HEAD
    git -C "$ORT_SRC_DIR" clean -fdx
    log "onnxruntime ready"
}

main() {
    bootstrap_ort
}

main "$@"
