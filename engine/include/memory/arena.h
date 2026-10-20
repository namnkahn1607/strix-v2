#pragma once

#include <absl/strings/cord.h>

#include <atomic>
#include <cstddef>
#include <cstdint>
#include <memory>
#include <optional>

#include "config/arena.h"
#include "meta_node.h"

namespace strix::memory {

class BufPool;

// Primary memory region.
// Thread-safe as each API has its own synchronization guarantees.
class Arena final {
public:
    explicit Arena(config::ArenaOptions& opt);
    ~Arena();

    Arena(const Arena&)           = delete;
    Arena operator=(const Arena&) = delete;
    Arena(Arena&&)                = delete;
    Arena operator=(Arena&&)      = delete;

    std::optional<absl::Cord> ReadPayload(std::uint32_t nid) const noexcept;

    bool WritePayload(std::uint32_t nid, const std::uint8_t* in, std::uint32_t length) noexcept;

    void GarbageCollection() noexcept;

    const std::uint32_t num_slots;
    const std::size_t   payload_buf_size;

private:
    std::size_t ActualIndex(std::uint64_t offset) const noexcept {
        return offset & (payload_buf_size - 1);
    }

    // Returns the virtual offset at which caller can begin writing to.
    std::optional<std::uint64_t> TryAllocateSpace(std::uint32_t length) noexcept;

    Node*         nodes_;    // Node slot array
    float*        vectors_;  // Vector array
    std::uint8_t* payload_buf_;

    std::atomic<std::uint64_t> write_head_{0};
    std::atomic<std::uint64_t> read_tail_{0};

    std::unique_ptr<BufPool> buf_pool_;
};

}  // namespace strix::memory
