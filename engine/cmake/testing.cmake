# cmake/testing.cmake

function(create_test target_name)
    cmake_parse_arguments(ARG "" "" "SOURCES;LIBS" ${ARGN})
    add_executable(${target_name} ${ARG_SOURCES})
    target_link_libraries(${target_name}
        PRIVATE ${ARG_LIBS} GTest::gtest_main GTest::gtest)
    gtest_discover_tests(${target_name})
endfunction()

function(create_unit_test target_name module_name)
    cmake_parse_arguments(ARG "" "" "SOURCES;LIBS" ${ARGN})
    create_test(${target_name} ${ARGN})
    target_include_directories(${target_name}
        PRIVATE "${CMAKE_SOURCE_DIR}/src/${module_name}")
endfunction()
