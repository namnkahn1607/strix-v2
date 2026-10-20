// clang-format off
#include "memory/arena.h"
// clang-format on

#include <algorithm>

#include "payload_header.h"

namespace strix::memory {

void Arena::GarbageCollection() noexcept {
    while (true) {
        const std::uint64_t tail = read_tail_.load(std::memory_order_relaxed);

        const auto tail_index = ActualIndex(tail);
        if (const auto padding = payload_buf_size - tail_index;
            padding < sizeof(PayloadHeader)) {
            read_tail_.fetch_add(padding, std::memory_order_relaxed);
        }

        const auto* header =
            reinterpret_cast<const PayloadHeader*>(payload_buf_ + tail_index);
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
            const auto desired =
                ControlBlock::Pack(NodeState::kDead, ref, ver, length, offset);
            if (node.control_block.compare_exchange_strong(
                    expected, desired, std::memory_order_release,
                    std::memory_order_relaxed
                )) {
                read_tail_.fetch_add(total_len, std::memory_order_relaxed);
                continue;
            }
            // Should this CAS fail?
        }

        // Hot node: grant Second Chance while cooling its reference bit.
        const auto opt_offset = TryAllocateSpace(text_len);
        if (!opt_offset.has_value()) {
            // No room to rescue. Force eviction.
            const auto desired = ControlBlock::Pack(
                NodeState::kDead, RefBit::kCold, ver, length, offset
            );
            if (node.control_block.compare_exchange_strong(
                    expected, desired, std::memory_order_release,
                    std::memory_order_relaxed
                )) {
                read_tail_.fetch_add(total_len, std::memory_order_relaxed);
                continue;
            }
            // Again: should this CAS fail?
        }

        const auto rescued_offset = opt_offset.value();
        const auto rescued_index  = ActualIndex(rescued_offset);

        const PayloadHeader rescued_header{
            PayloadHeader::kValidIdentifier, nid, text_len
        };
        std::memcpy(
            payload_buf_ + rescued_index, &rescued_header, sizeof(PayloadHeader)
        );

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

        const auto desired =
            ControlBlock::Pack(state, RefBit::kCold, ver, length, offset);
        if (node.control_block.compare_exchange_strong(
                expected, desired, std::memory_order_release,
                std::memory_order_relaxed
            )) {
            read_tail_.fetch_add(total_len, std::memory_order_relaxed);
        }
        // Should this CAS fail?
    }
}

}  // namespace strix::memory
