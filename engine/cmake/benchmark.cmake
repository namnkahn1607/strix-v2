# cmake/benchmark.cmake

function(create_bench target_name)
    cmake_parse_arguments(ARG "" "" "SOURCES;LIBS" ${ARGN})
    add_executable(${target_name} ${ARG_SOURCES})
    target_link_libraries(${target_name}
        PRIVATE ${ARG_LIBS} benchmark::benchmark)
endfunction()

function(create_micro_bench target_name module_name)
    cmake_parse_arguments(ARG "" "" "SOURCES;LIBS" ${ARGN})
    create_bench(${target_name} ${ARGN})
    target_include_directories(${target_name}
        PRIVATE "${CMAKE_SOURCE_DIR}/src/${module_name}")
endfunction()
