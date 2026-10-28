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
    ControlBlock Load(std::memory_order order = std::memory_order_acquire) const noexcept {
        return ControlBlock::Unpack(control_block.load(order));
    }

    // Extracts only the version field.
    // Used on both sides of a seqlock-style version check.
    std::uint8_t LoadVersion(std::memory_order order = std::memory_order_acquire) const noexcept {
        return static_cast<std::uint8_t>(
            (control_block.load(order) >> kVersionShift) & kVersionMask
        );
    }
};

}  // namespace strix::memory
