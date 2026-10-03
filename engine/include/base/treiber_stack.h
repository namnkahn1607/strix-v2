#pragma once

#include <atomic>
#include <cstdint>
#include <memory>

namespace strix {

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

    explicit TreiberStack(std::uint32_t capacity);

    TreiberStack(const TreiberStack&)           = delete;
    TreiberStack& operator=(const TreiberStack) = delete;
    TreiberStack(TreiberStack&&)                = delete;
    TreiberStack& operator=(TreiberStack&&)     = delete;

    void Push(std::uint32_t id) noexcept;

    std::uint32_t Pop() noexcept;

private:
    std::atomic<TaggedHead>          head_;
    std::unique_ptr<std::uint32_t[]> next_;
};

static_assert(sizeof(std::atomic<TaggedHead>) == 8);
static_assert(std::atomic<TaggedHead>::is_always_lock_free);

}  // namespace strix
