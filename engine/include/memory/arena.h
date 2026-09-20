#pragma once

#include <atomic>
#include <cstddef>
#include <cstdint>

#include "meta_node.h"

namespace strix::memory {

// Primary memory region.
// Thread-safe as each API has its own synchronization guarantees.
class Arena final {
public:
    explicit Arena(std::uint32_t num_slots, std::size_t payload_buf_size);
    ~Arena();

    Arena(const Arena&)           = delete;
    Arena operator=(const Arena&) = delete;
    Arena(Arena&&)                = delete;
    Arena operator=(Arena&&)      = delete;

    const std::uint32_t num_slots;
    const std::size_t   payload_buf_size;

private:
    Node*         nodes_;    // Node slot array
    float*        vectors_;  // Vector array
    std::uint8_t* payload_buf_;

    std::atomic<std::uint64_t> write_head;
    std::atomic<std::uint64_t> read_tail;
};

}  // namespace strix::memory
