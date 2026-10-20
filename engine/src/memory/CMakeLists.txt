add_module(memory
    SOURCES arena.cc
    DEPS_PRIVATE absl::cord engine_base engine_config)
