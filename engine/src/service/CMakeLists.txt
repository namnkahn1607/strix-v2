include("${CMAKE_SOURCE_DIR}/cmake/vendored-protoc.cmake")

set(PROTO_DIR "${STRIX_REPO_ROOT}/proto/v1")
set(PROTO_FILE "${PROTO_DIR}/cache.proto")
set(PROTO_GEN_DIR "${CMAKE_CURRENT_BINARY_DIR}/generated")

file(MAKE_DIRECTORY ${PROTO_GEN_DIR})
set(GRPC_PLUGIN_EXEC "$<TARGET_FILE:gRPC::grpc_cpp_plugin>")

add_custom_command(
    OUTPUT
        "${PROTO_GEN_DIR}/cache.pb.cc" "${PROTO_GEN_DIR}/cache.pb.h"
        "${PROTO_GEN_DIR}/cache.grpc.pb.cc" "${PROTO_GEN_DIR}/cache.grpc.pb.h"
    COMMAND ${PROTOC_EXEC}
    ARGS --grpc_out="${PROTO_GEN_DIR}" --cpp_out="${PROTO_GEN_DIR}"
         --plugin=protoc-gen-grpc="${GRPC_PLUGIN_EXEC}" -I "${PROTO_DIR}" "${PROTO_FILE}"
    DEPENDS "${PROTO_FILE}" "${PROTOC_EXEC}" gRPC::grpc_cpp_plugin
    COMMENT "Generating server-side C++ gRPC stubs"
)

add_library(engine_contract STATIC
    "${PROTO_GEN_DIR}/cache.pb.cc" "${PROTO_GEN_DIR}/cache.grpc.pb.cc"
)
target_link_libraries(engine_contract PUBLIC gRPC::grpc++ protobuf::libprotobuf)
target_include_directories(engine_contract PUBLIC "${PROTO_GEN_DIR}")

add_module(service
    SOURCES service_impl.cc
    DEPS_PRIVATE engine_options
    DEPS_PUBLIC  engine_contract
)
