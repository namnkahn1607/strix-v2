#pragma once

#include <absl/strings/cord.h>

#include <atomic>
#include <cstddef>
#include <cstdint>
#include <functional>
#include <memory>
#include <optional>

#include "config/arena.h"
#include "hazard_offset.h"
#include "meta_node.h"

namespace strix::memory {

class BufPool;

// Primary memory region.
// Thread-safe as each API has its own synchronization guarantees.
class Arena final {
public:
    using NodeFreedCallback = std::function<void(std::uint32_t)>;

    explicit Arena(config::ArenaOptions& opt, std::size_t num_workers);
    ~Arena();

    Arena(const Arena&)            = delete;
    Arena& operator=(const Arena&) = delete;
    Arena(Arena&&)                 = delete;
    Arena& operator=(Arena&&)      = delete;

    std::optional<absl::Cord> ReadPayload(
        std::uint32_t nid, std::uint8_t exp_ver, TimePoint now) noexcept;

    bool WritePayload(std::uint32_t nid, const std::uint8_t* in, std::uint32_t length) noexcept;

    void GarbageCollection(const std::atomic<bool>& shutdown_req) noexcept;

    void SetNodeFreedCallback(NodeFreedCallback cb) { on_node_freed_ = std::move(cb); }

    const std::uint32_t num_slots;
    const std::size_t   payload_buf_size;

private:
    std::size_t ActualIndex(std::uint64_t offset) const noexcept {
        return offset & (payload_buf_size - 1);
    }

    // Returns the virtual offset at which caller can begin writing to.
    std::optional<std::uint64_t> TryAllocateSpace(std::uint32_t length) noexcept;

    // Node is released back to free-list upon `free = true`.
    void TryReclaimSpace(
        std::uint32_t nid, std::uint64_t tail, std::uint32_t total_len, bool free) noexcept;

    void SweepStalePending(TimePoint now) noexcept;

    Node*         nodes_;    // Node slot array
    float*        vectors_;  // Vector array
    std::uint8_t* payload_buf_;

    std::atomic<std::uint64_t> write_head_{0};
    std::atomic<std::uint64_t> read_tail_{0};

    // Callback hook invoked by the GC when a node is ready for reuse.
    NodeFreedCallback on_node_freed_;

    // Managing table of published hazard zones.
    HazardTable hazard_table_;

    std::unique_ptr<BufPool> buf_pool_;
};

}  // namespace strix::memory
