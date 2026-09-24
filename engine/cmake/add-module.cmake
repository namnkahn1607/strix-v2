# cmake/add-module.cmake

function(add_module NAME)
    cmake_parse_arguments(ARG "" "" "SOURCES;DEPS_PUBLIC;DEPS_PRIVATE" ${ARGN})
    set(target engine_${NAME})

    if(ARG_SOURCES)
        add_library(${target} STATIC ${ARG_SOURCES})
        target_include_directories(${target}
            PUBLIC  $<BUILD_INTERFACE:${PUBLIC_HEADER_ROOT}>
            PRIVATE $<BUILD_INTERFACE:${CMAKE_CURRENT_SOURCE_DIR}>)

        if(ARG_DEPS_PUBLIC)
            target_link_libraries(${target} PUBLIC ${ARG_DEPS_PUBLIC})
        endif()
        if(ARG_DEPS_PRIVATE)
            target_link_libraries(${target} PRIVATE ${ARG_DEPS_PRIVATE})
        endif()
    else()
        message(FATAL_ERROR "Module source unspecified")
    endif()
endfunction()
