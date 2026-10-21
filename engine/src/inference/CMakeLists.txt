set(ORT_ROOT "${CMAKE_SOURCE_DIR}/ort")
add_library(ONNXRuntime SHARED IMPORTED)
set_target_properties(ONNXRuntime PROPERTIES
    IMPORTED_LOCATION "${ORT_ROOT}/lib/libonnxruntime.so"
    INTERFACE_INCLUDE_DIRECTORIES "${ORT_ROOT}/include"
    INTERFACE_SYSTEM_INCLUDE_DIRECTORIES "${ORT_ROOT}/include")

add_module(inference
    SOURCES     encoder.cpp
    DEPS_PUBLIC ONNXRuntime)
