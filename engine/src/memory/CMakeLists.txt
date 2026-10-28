add_module(memory
    SOURCES arena.cpp arena_gc.cpp
    DEPS_PRIVATE absl::cord engine_base engine_config engine_worker)
