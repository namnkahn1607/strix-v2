#pragma once

#include <cstdint>

namespace strix::config {

struct IvfOptions {
    std::uint32_t num_clusters;
    std::uint32_t max_cluster_size;
    std::uint32_t staging_buffer_capacity;
};

}  // namespace strix::config
