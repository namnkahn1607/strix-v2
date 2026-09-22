#pragma once

#include <atomic>
#include <cstdint>
#include <memory>
#include <stdexcept>

namespace strix::memory {

// Tagging disambiguates a cycle back to the old head value, hence
// resolving ABA problem of a plain Treiber stack.
struct alignas(8) TaggedHead {
    std::uint32_t head_id;
    // Monotonic counter incrementing on every push/pop.
    std::uint32_t tag;
};

class TreiberStack {
public:
    static constexpr std::uint32_t kEmpty = 0xFFFFFFFFu;

    explicit TreiberStack(std::uint32_t capacity) {
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

    TreiberStack(const TreiberStack&)           = delete;
    TreiberStack& operator=(const TreiberStack) = delete;
    TreiberStack(TreiberStack&&)                = delete;
    TreiberStack& operator=(TreiberStack&&)     = delete;

    void Push(std::uint32_t id) noexcept {
        TaggedHead old_head = head_.load(std::memory_order_relaxed);
        TaggedHead new_head;

        do {
            next_[id] = old_head.head_id;
            new_head  = {id, old_head.tag + 1};
        } while (!head_.compare_exchange_weak(
            old_head, new_head, std::memory_order_release,
            std::memory_order_relaxed
        ));
    }

    std::uint32_t Pop() noexcept {
        TaggedHead old_head = head_.load(std::memory_order_acquire);

        while (true) {
            if (old_head.head_id == kEmpty) {
                return kEmpty;
            }

            const std::uint32_t next_id = next_[old_head.head_id];
            const TaggedHead    new_head{next_id, old_head.head_id + 1};

            if (head_.compare_exchange_weak(
                    old_head, new_head, std::memory_order_acquire,
                    std::memory_order_acquire
                )) {
                return old_head.head_id;
            }
        }
    }

private:
    std::atomic<TaggedHead>          head_;
    std::unique_ptr<std::uint32_t[]> next_;
};

static_assert(sizeof(std::atomic<TaggedHead>) == 8);
static_assert(std::atomic<TaggedHead>::is_always_lock_free);

}  // namespace strix::memory
