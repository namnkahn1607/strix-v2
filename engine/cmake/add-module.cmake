# cmake/add-module.cmake

function(add_module NAME)
    cmake_parse_arguments(ARG "" "" "SOURCES;DEPS_PUBLIC;DEPS_PRIVATE" ${ARGN})
    if(NOT ARG_SOURCES)
        message(FATAL_ERROR "[add-module] SOURCES not specified")
    endif()
    if()
        message(FATAL_ERROR "PUBLIC_HEADER_ROOT is not set")
    endif()

    set(target engine_${NAME})

    add_library(${target} STATIC ${ARG_SOURCES})
    target_include_directories(${target}
        PUBLIC  $<BUILD_INTERFACE:${PUBLIC_HEADER_ROOT}>
        PRIVATE $<BUILD_INTERFACE:${CMAKE_CURRENT_SOURCE_DIR}>)
    target_link_libraries(${target} PRIVATE engine_options)

    if(ARG_DEPS_PUBLIC)
        target_link_libraries(${target} PUBLIC ${ARG_DEPS_PUBLIC})
    endif()
    if(ARG_DEPS_PRIVATE)
        target_link_libraries(${target} PRIVATE ${ARG_DEPS_PRIVATE})
    endif()
endfunction()
