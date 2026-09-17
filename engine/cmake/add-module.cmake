# cmake/add-module.cmake

function(add_module NAME)
    cmake_parse_arguments(ARG "" "" "SOURCES;DEPS_PUBLIC;DEPS_PRIVATE" ${ARGN})
    set(target engine_${NAME})

    if(ARG_SOURCES)
        add_library(${target} STATIC ${ARG_SOURCES})
        set(public_scope PUBLIC)
        set(private_scope PRIVATE)
    else()
        add_library(${target} INTERFACE)
        set(public_scope INTERFACE)
        set(private_scope INTERFACE)
    endif()

    target_include_directories(${target} ${public_scope} "${PUBLIC_HEADER_ROOT}")
    if(ARG_SOURCES)
        target_include_directories(${target} PRIVATE "${CMAKE_CURRENT_SOURCE_DIR}")
    endif()

    if(ARG_DEPS_PUBLIC)
        target_link_libraries(${target} ${public_scope} ${ARG_DEPS_PUBLIC})
    endif()
    if(ARG_DEPS_PRIVATE)
        target_link_libraries(${target} ${private_scope} ${ARG_DEPS_PRIVATE})
    endif()
endfunction()
