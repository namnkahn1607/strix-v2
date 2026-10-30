#pragma once

#include <atomic>
#include <cstdint>
#include <memory>

namespace strix::catalog {

// L0-tier node buffer implemented under MPSC model, where acquiring workers
// are producers, while compaction (background) worker is the sole consumer.
class NodeBuf {
public:
    static constexpr std::uint32_t kEmpty = 0xFFFFFFFFu;

    explicit NodeBuf(std::uint32_t capacity);

    NodeBuf(const NodeBuf&)            = delete;
    NodeBuf& operator=(const NodeBuf&) = delete;
    NodeBuf(NodeBuf&&)                 = delete;
    NodeBuf& operator=(NodeBuf&&)      = delete;

    bool TryEnqueue(std::uint32_t nid) noexcept;

    std::uint32_t TryDequeue() noexcept;

    // Rightmost margin for the L0-tier search routine.
    std::uint32_t SnapPushHead() const noexcept {
        return push_head_.load(std::memory_order_relaxed);
    }

    // Leftmost margin for the L0-tier search routine.
    std::uint32_t SnapPopTail() const noexcept {
        return pop_tail_.load(std::memory_order_relaxed);  // dope
    }

    const std::uint32_t capacity;

private:
    alignas(64) std::atomic<std::uint32_t> push_head_{0};
    alignas(64) std::atomic<std::uint32_t> pop_tail_{0};

    std::unique_ptr<std::atomic<std::uint32_t>[]> slots_;
};

}  // namespace strix::catalog
