#include "node_buf.h"

#include <atomic>
#include <memory>

namespace strix::catalog {

NodeBuf::NodeBuf(std::uint32_t capacity) : capacity{capacity} {
    slots_ = std::make_unique_for_overwrite<std::atomic<std::uint32_t>[]>(capacity);
    for (std::uint32_t i = 0; i < capacity; ++i) {
        slots_[i].store(kEmpty, std::memory_order_relaxed);
    }
}

bool NodeBuf::TryEnqueue(std::uint32_t nid) noexcept {
    std::uint32_t curr_push = push_head_.load(std::memory_order_relaxed);

    while (true) {
        if (curr_push - pop_tail_.load(std::memory_order_relaxed) >= capacity) return false;

        const auto next_push = curr_push + 1;
        if (push_head_.compare_exchange_weak(
                curr_push, next_push, std::memory_order_relaxed, std::memory_order_relaxed)) {
            break;
        }
        // CAS failed means another producer won the race.
        // curr_push is updated to its latest value; loop re-checks capacity
        // and retries again.
    }

    const auto ring_pos = curr_push & (capacity - 1);
    slots_[ring_pos].store(nid, std::memory_order_release);
    return true;
}

std::uint32_t NodeBuf::TryDequeue() noexcept {
    const std::uint32_t curr_pop = pop_tail_.load(std::memory_order_relaxed);

    if (curr_pop == push_head_.load(std::memory_order_relaxed)) return kEmpty;

    const auto ring_pos = curr_pop & (capacity - 1);
    const auto nid      = slots_[ring_pos].load(std::memory_order_acquire);
    slots_[ring_pos].store(kEmpty, std::memory_order_relaxed);

    // Only one consumer. A direct store is sufficient.
    pop_tail_.store(curr_pop + 1, std::memory_order_relaxed);
    return nid;
}

}  // namespace strix::catalog
