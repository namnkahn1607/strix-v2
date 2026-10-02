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
set(GCC_CLANG_WARNINGS -march=x86-64-v3 -Wall -Wextra -Wpendatic -Wshadow -Wconversion)
set(CLANG_ONLY_WARNINGS -Wunreachable-code)
target_compile_options(engine_options INTERFACE
    "$<$<COMPILE_LANG_AND_ID:CXX,GNU,Clang,AppleClang>:${GCC_CLANG_WARNINGS}>"
    "$<$<COMPILE_LANG_AND_ID:CXX,Clang,AppleClang>:${CLANG_ONLY_WARNINGS}>")

add_subdirectory(src)

if(CMAKE_EXPORT_COMPILE_COMMANDS)
    file(CREATE_LINK "${CMAKE_BINARY_DIR}/compile_commands.json"
                     "${CMAKE_SOURCE_DIR}/compile_commands.json" SYMBOLIC)
endif()
