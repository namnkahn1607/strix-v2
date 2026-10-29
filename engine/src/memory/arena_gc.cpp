// clang-format off
#include "memory/arena.h"
// clang-format on

#include <algorithm>
#include <atomic>

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

        const std::uint64_t tail = read_tail_.load(std::memory_order_relaxed);

        const auto tail_index = ActualIndex(tail);
        if (const auto padding = payload_buf_size - tail_index; padding < sizeof(PayloadHeader)) {
            read_tail_.fetch_add(padding, std::memory_order_relaxed);
        }

        const auto* header = reinterpret_cast<const PayloadHeader*>(payload_buf_ + tail_index);
        if (header->identifier != PayloadHeader::kValidIdentifier) {
            // Reading an invalid PayloadHeader.
            // Reluctantly advance by 1 byte and retry.
            read_tail_.fetch_add(1, std::memory_order_relaxed);
            continue;
        }

        const auto nid       = header->node_id;
        const auto text_len  = header->length;
        const auto total_len = sizeof(PayloadHeader) + text_len;

        auto& node = nodes_[nid];

        const auto [state, ref, ver, length, offset] = node.Load();
        auto expected = ControlBlock::Pack(state, ref, ver, length, offset);

        if (state == NodeState::kDead) {
            read_tail_.fetch_add(total_len, std::memory_order_relaxed);
            on_node_freed_(nid);
            continue;
        }

        if (state == NodeState::kPending) {
            // New payload for this node will be committed at write_head_, not
            // at this position.
            read_tail_.fetch_add(total_len, std::memory_order_relaxed);
            continue;
        }

        if (ref == RefBit::kCold) {
            // Cold node: evict immeditately.
            const auto desired = ControlBlock::Pack(NodeState::kDead, ref, ver, length, offset);
            if (node.control_block.compare_exchange_strong(
                    expected, desired, std::memory_order_release, std::memory_order_relaxed)) {
                read_tail_.fetch_add(total_len, std::memory_order_relaxed);
                on_node_freed_(nid);
                continue;
            }
            // Should this CAS fail?
        }

        // Hot node: grant Second Chance while cooling its reference bit.
        const auto opt_offset = TryAllocateSpace(text_len);
        if (!opt_offset.has_value()) {
            // No room to rescue. Force eviction.
            const auto desired =
                ControlBlock::Pack(NodeState::kDead, RefBit::kCold, ver, length, offset);
            if (node.control_block.compare_exchange_strong(
                    expected, desired, std::memory_order_release, std::memory_order_relaxed)) {
                read_tail_.fetch_add(total_len, std::memory_order_relaxed);
                on_node_freed_(nid);
                continue;
            }
            // Again: should this CAS fail?
        }

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
        if (node.control_block.compare_exchange_strong(
                expected, desired, std::memory_order_release, std::memory_order_relaxed)) {
            read_tail_.fetch_add(total_len, std::memory_order_relaxed);
        }
        // Should this CAS fail?
    }
}

void Arena::SweepStalePending(TimePoint now) noexcept {
    for (std::uint32_t node_id = 0; node_id < num_slots; ++node_id) {
        auto& node = nodes_[node_id];

        const std::uint64_t ctrl = node.control_block.load(std::memory_order_acquire);
        const auto [state, ref, ver, length, offset] = ControlBlock::Unpack(ctrl);

        if (state != NodeState::kPending) {
            continue;
        }

        const auto ts = node.created_at.load(std::memory_order_acquire);
        if (now - ts <= kPendingLifespan) {
            continue;
        }

        auto       expected = ctrl;
        const auto desired =
            ControlBlock::Pack(NodeState::kDead, RefBit::kCold, ver, length, offset);
        node.control_block.compare_exchange_strong(
            expected, desired, std::memory_order_release, std::memory_order_relaxed);
        // CAS failed. A worker committed payload for this node and landed a
        // PENDING -> READY publish.
    }
}

}  // namespace strix::memory
