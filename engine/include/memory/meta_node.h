#pragma once

#include <atomic>
#include <chrono>
#include <cstdint>

#include "control_block.h"

namespace strix::memory {

using Clock     = std::chrono::steady_clock;
using TimePoint = Clock::time_point;

static_assert(std::atomic<TimePoint>::is_always_lock_free);

// Contains metadata of a node slot.
struct alignas(64) Node {
    // Last time this node was acquired.
    std::atomic<TimePoint>     created_at;
    std::atomic<std::uint64_t> control_block;

    // Loads and decodes the control block.
    ControlBlock LoadCtrl(
        std::memory_order order = std::memory_order_acquire
    ) const noexcept {
        return ControlBlock::Unpack(control_block.load(order));
    }
};

}  // namespace strix::memory
