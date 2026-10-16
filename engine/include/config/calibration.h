#pragma once

#include <chrono>
#include <cstdint>

namespace strix::config {

struct CalibrationOptions {
    std::uint32_t        sample_size;
    std::uint32_t        recalibrate_trigger_count;
    std::uint32_t        mini_batch_size;
    std::uint32_t        max_lloyd_iteration;
    double               min_convergence;
    std::chrono::seconds grace_period;
};

}  // namespace strix::config
