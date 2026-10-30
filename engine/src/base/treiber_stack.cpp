#include "base/treiber_stack.h"

namespace strix {

TreiberStack::TreiberStack(std::uint32_t capacity) {
    if (capacity == 0) {
        throw std::invalid_argument("Stack capacity must be positive");
    }

    head_.store({0, 0}, std::memory_order_relaxed);

    next_ = std::make_unique_for_overwrite<std::uint32_t[]>(capacity);
    for (std::uint32_t i = 0; i < capacity - 1; ++i) {
        next_[i] = i + 1;
    }
    next_[capacity - 1] = kEmpty;
}

void TreiberStack::Push(std::uint32_t id) noexcept {
    TaggedHead old_head = head_.load(std::memory_order_relaxed);
    TaggedHead new_head;

    do {
        next_[id] = old_head.head_id;
        new_head  = {id, old_head.tag + 1};
    } while (!head_.compare_exchange_weak(
        old_head, new_head, std::memory_order_release, std::memory_order_relaxed));
}

std::uint32_t TreiberStack::Pop() noexcept {
    TaggedHead old_head = head_.load(std::memory_order_acquire);

    while (true) {
        if (old_head.head_id == kEmpty) {
            return kEmpty;
        }

        const std::uint32_t next_id = next_[old_head.head_id];
        const TaggedHead    new_head{next_id, old_head.head_id + 1};

        if (head_.compare_exchange_weak(
                old_head, new_head, std::memory_order_acquire, std::memory_order_acquire)) {
            return old_head.head_id;
        }
    }
}

}  // namespace strix
