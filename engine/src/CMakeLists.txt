add_subdirectory(inference)
add_subdirectory(ann)
add_subdirectory(server)

add_executable(strix_engine main.cc)
target_link_libraries(strix_engine
    PRIVATE engine_options engine_inference engine_server)
