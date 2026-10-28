add_subdirectory(base)
add_subdirectory(config)
add_subdirectory(inference)
add_subdirectory(worker)
add_subdirectory(memory)
add_subdirectory(ann)
add_subdirectory(server)

add_executable(strix_engine main.cpp)
target_link_libraries(strix_engine
    PRIVATE engine_options engine_config engine_inference engine_worker engine_server)
