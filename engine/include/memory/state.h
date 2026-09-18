#pragma once

#include <chrono>
#include <cstdint>

namespace strix::memory {

// Lifecycle state of a node slot.
enum class NodeState : std::uint8_t {
    kDead    = 0,
    kPending = 1,  // Vector ready (searchable), no payload yet
    kReady   = 2,  // Payload committed
};

// Crossing this deadline makes node being treated as expired/stale.
inline constexpr std::chrono::seconds kPendingLifespan{30};

// CLOCK reference bit of a node slot.
enum class EvictState : std::uint8_t {
    kCold = 0,
    kHot  = 1,
};

}  // namespace strix::memory
