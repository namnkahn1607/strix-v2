# cmake/vendored-protoc.cmake

execute_process(
    COMMAND git rev-parse --show-toplevel
    WORKING_DIRECTORY ${CMAKE_CURRENT_SOURCE_DIR}
    OUTPUT_VARIABLE STRIX_REPO_ROOT
    OUTPUT_STRIP_TRAILING_WHITESPACE
    RESULT_VARIABLE _strix_git_result
)
if(NOT _strix_git_result EQUAL 0)
    message(FATAL_ERROR "[vendored-protoc] Could not resolve repository root")
endif()

# Step 1. Read expected protoc version
set(_protoc_version_file "${STRIX_REPO_ROOT}/.protoc-version")
if(NOT EXISTS "${_protoc_version_file}")
    message(FATAL_ERROR "[vendored-protoc] ${_protoc_version_file} not found. "
            "Run 'bash scripts/install/protoc.sh' first")
endif()

file(READ "${_protoc_version_file}" _protoc_expected_version)
string(STRIP "${_protoc_expected_version}" _protoc_expected_version)

# Step 2. Locate the vendored executable
find_program(PROTOC_EXEC protoc PATHS "${STRIX_REPO_ROOT}/tools/protoc/bin" NO_DEFAULT_PATH)
if(NOT PROTOC_EXEC)
    message(FATAL_ERROR "[vendored-protoc] Binary executable not found. "
            "Run 'bash scripts/install/protoc.sh' first")
endif()

# Step 3. Compare expected version to actual one of binary
execute_process(
    COMMAND ${PROTOC_EXEC} --version
    OUTPUT_VARIABLE _protoc_version_output
    OUTPUT_STRIP_TRAILING_WHITESPACE
)

string(REGEX MATCH "[0-9]+\\.[0-9]+(\\.[0-9]+)?" _protoc_actual_version "${_protoc_version_output}")
if(NOT _protoc_actual_version STREQUAL _protoc_expected_version)
    message(FATAL_ERROR "[vendored-protoc] Version mismatch. Program binary emits "
            "${_protoc_actual_version}, but ${_protoc_expected_version} is pinned. "
            "Maybe bumped vcpkg without re-running 'bash scripts/install/protoc.sh'?")
endif()

message(STATUS "[vendored-protoc] Using protoc ${_protoc_actual_version}")
