add_module(memory
    SOURCES arena.cc arena_gc.cc
    DEPS_PRIVATE absl::cord engine_base engine_config)
