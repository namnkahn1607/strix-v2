cmake_minimum_required(VERSION 3.28)
project(strix_engine VERSION 1.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

find_package(Protobuf CONFIG REQUIRED)
find_package(gRPC CONFIG REQUIRED)

include(cmake/add-module.cmake)

set(PUBLIC_HEADER_ROOT "${CMAKE_SOURCE_DIR}/include")

add_library(engine_options INTERFACE)
target_compile_options(engine_options INTERFACE
    -march=x86-64-v3 -Wall -Wextra -Wpedantic -Wshadow -Wconversion -Wunreachable-code
)

add_subdirectory(src/inference)
add_subdirectory(src/service)

add_executable(strix_engine src/main.cc)
target_link_libraries(strix_engine PRIVATE engine_options engine_service)

if(CMAKE_EXPORT_COMPILE_COMMANDS)
    file(CREATE_LINK "${CMAKE_BINARY_DIR}/compile_commands.json"
                     "${CMAKE_SOURCE_DIR}/compile_commands.json" SYMBOLIC)
endif()
