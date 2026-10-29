// clang-format off
#include "memory/arena.h"
// clang-format on

#include <absl/log/check.h>

#include <algorithm>
#include <atomic>

#include "memory/control_block.h"
#include "memory/meta_node.h"
#include "memory/state.h"
#include "payload_header.h"

namespace strix::memory {

void Arena::GarbageCollection(const std::atomic<bool>& shutdown_req) noexcept {
    constexpr std::chrono::milliseconds kSweepInterval{5'000u};

    auto last_sweep = Clock::now();
    while (!shutdown_req.load(std::memory_order_relaxed)) {
        auto now = Clock::now();
        if (now - last_sweep >= kSweepInterval) {
            SweepStalePending(now);
            last_sweep = now;
        }

        const auto tail = read_tail_.load(std::memory_order_relaxed);

        const auto tail_index = ActualIndex(tail);
        if (const auto padding = payload_buf_size - tail_index; padding < sizeof(PayloadHeader)) {
            read_tail_.fetch_add(padding, std::memory_order_relaxed);
            continue;
        }

        const auto [identifier, nid, text_len] =
            *reinterpret_cast<const PayloadHeader*>(payload_buf_ + tail_index);
        if (identifier != PayloadHeader::kValidIdentifier) {
            // Reading an invalid PayloadHeader. Advance by 1 byte and retry.
            read_tail_.fetch_add(1, std::memory_order_relaxed);
            continue;
        }

        const std::uint32_t total_len = sizeof(PayloadHeader) + text_len;

        auto& node     = nodes_[nid];
        auto  expected = node.control_block.load(std::memory_order_acquire);
        const auto [state, ref, ver, length, offset] = ControlBlock::Unpack(expected);

        if (offset != (tail & kVirtualOffsetMask)) {
            // Virtual offset no longer matches its position in buffer.
            TryReclaimSpace(nid, tail, total_len, false);
            continue;
        }

        if (state == NodeState::kDead) {
            TryReclaimSpace(nid, tail, total_len, true);
            continue;
        }

        if (state == NodeState::kPending) {
            // New payload for this node will be committed at write_head_, not this position.
            read_tail_.fetch_add(total_len, std::memory_order_relaxed);
            continue;
        }

        if (ref == RefBit::kCold) {
            // Cold node: evict immeditately.
            const auto desired = ControlBlock::Pack(NodeState::kDead, ref, ver, length, offset);
            if (node.control_block.compare_exchange_strong(
                    expected, desired, std::memory_order_release, std::memory_order_relaxed)) {
                TryReclaimSpace(nid, tail, total_len, true);
                continue;
            }
            // CAS failed. A READ worker promoted this node from COLD -> HOT.
            // Fallthrough the HOT case below.
        }

        // Hot node: grant Second Chance while cooling its reference bit.
        const auto opt_offset = TryAllocateSpace(text_len);
        if (!opt_offset.has_value()) {
            // No room to rescue. Force eviction.
            const auto desired =
                ControlBlock::Pack(NodeState::kDead, RefBit::kCold, ver, length, offset);

            // This CAS can't fail. Once the node is READY, only the GC can change its
            // state/ref-bit. Eviction hasn't happened for an aquisition to make changes
            // to version, length and virtual offset. [*]
            CHECK(node.control_block.compare_exchange_strong(
                expected, desired, std::memory_order_release, std::memory_order_relaxed))
                << "Force-evict CAS shouldn't fail. Check concurrency invariants.";
            TryReclaimSpace(nid, tail, total_len, true);
            continue;
        }

        // Payload rescue: copy bytes to a new position, cool the reference bit, and
        // update new virtual offset.
        const auto rescued_offset = opt_offset.value();
        const auto rescued_index  = ActualIndex(rescued_offset);

        const PayloadHeader rescued_header{PayloadHeader::kValidIdentifier, nid, text_len};
        std::memcpy(payload_buf_ + rescued_index, &rescued_header, sizeof(PayloadHeader));

        auto src_idx = ActualIndex(tail + sizeof(PayloadHeader));
        auto dst_idx = ActualIndex(rescued_index + sizeof(PayloadHeader));

        std::uint64_t bytes_left = text_len;
        while (bytes_left > 0) {
            const auto src_cont = payload_buf_size - src_idx;
            const auto dst_cont = payload_buf_size - dst_idx;
            const auto chunk    = std::min({bytes_left, src_cont, dst_cont});

            std::memcpy(payload_buf_ + dst_idx, payload_buf_ + src_idx, chunk);
            bytes_left -= chunk;
            src_idx = ActualIndex(src_idx + chunk);
            dst_idx = ActualIndex(dst_idx + chunk);
        }

        const auto desired = ControlBlock::Pack(state, RefBit::kCold, ver, length, offset);

        // This CAS shouldn't fail for the same reasons as [*].
        CHECK(node.control_block.compare_exchange_strong(
            expected, desired, std::memory_order_release, std::memory_order_relaxed))
            << "Rescue CAS shouldn't fail. Check concurrency invariants.";
        TryReclaimSpace(nid, tail, total_len, false);
        continue;
    }
}

void Arena::SweepStalePending(TimePoint now) noexcept {
    for (std::uint32_t nid = 0; nid < num_slots; ++nid) {
        auto& node = nodes_[nid];

        auto expected = node.control_block.load(std::memory_order_acquire);
        const auto [state, ref, ver, length, offset] = ControlBlock::Unpack(expected);

        if (state != NodeState::kPending) continue;

        const auto ts = node.created_at.load(std::memory_order_acquire);
        if (now - ts <= kPendingLifespan) continue;

        const auto desired =
            ControlBlock::Pack(NodeState::kDead, RefBit::kCold, ver, length, offset);
        if (node.control_block.compare_exchange_strong(
                expected, desired, std::memory_order_release, std::memory_order_relaxed)) {
            on_node_freed_(nid);
        }
        // CAS failed. A worker committed payload for this node and landed a
        // PENDING -> READY publish.
    }
}

void Arena::TryReclaimSpace(
    std::uint32_t nid, std::uint64_t tail, std::uint32_t total_len, bool free) noexcept {
    if (hazard_table_.Overlaps(tail, total_len)) return;
    if (free) on_node_freed_(nid);
    read_tail_.fetch_add(total_len, std::memory_order_relaxed);
}

}  // namespace strix::memory
