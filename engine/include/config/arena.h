#pragma once

#include <cstddef>
#include <cstdint>

namespace strix::config {

struct ArenaOptions {
    std::uint32_t max_slots;
    std::size_t   payload_buf_size;
    std::uint32_t buf_pool_cap;
};

}  // namespace strix::config
