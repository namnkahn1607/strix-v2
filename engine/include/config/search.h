#pragma once

#include <cstdint>

namespace strix::config {

struct SearchOptions {
    float         similarity_threshold;
    std::uint32_t top_k;
};

}  // namespace strix::config
