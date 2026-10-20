#include "memory/arena.h"

#include <algorithm>
#include <atomic>
#include <cstring>
#include <memory>
#include <optional>

#include "allocator.h"
#include "buffer_pool.h"
#include "config/arena.h"
#include "inference/vector.h"
#include "memory/control_block.h"
#include "memory/state.h"
#include "payload_header.h"

namespace strix::memory {

Arena::Arena(config::ArenaOptions& opt)
    : num_slots{opt.max_slots}, payload_buf_size{opt.payload_buf_size} {
    nodes_       = Alloc<Node>(num_slots);
    vectors_     = Alloc<float>(num_slots * inference::kVectorDim);
    payload_buf_ = Alloc<std::uint8_t>(payload_buf_size);
    buf_pool_    = std::make_unique<BufPool>(opt.buf_pool_cap);
}

Arena::~Arena() {
    Dealloc<Node>(nodes_, num_slots);
    Dealloc<float>(vectors_, num_slots * inference::kVectorDim);
    Dealloc<std::uint8_t>(payload_buf_, payload_buf_size);
}

std::optional<absl::Cord> Arena::ReadPayload(std::uint32_t nid) const noexcept {
    auto& node = nodes_[nid];
    auto  ctrl = node.control_block.load(std::memory_order_acquire);

    auto [state, ref, ver, length, offset] = ControlBlock::Unpack(ctrl);
    if (state != NodeState::kReady) {
        return std::nullopt;
    }

    std::uint32_t pool_id;
    std::uint8_t* dst = buf_pool_->Acquire(&pool_id);
    if (dst == nullptr) {
        return std::nullopt;
    }

    const auto text_index = ActualIndex(offset);
    if (payload_buf_size - text_index >= length) {
        std::memcpy(dst, payload_buf_ + text_index, length);
    } else {
        const auto chunk1 = payload_buf_size - text_index;
        const auto chunk2 = length - chunk1;
        std::memcpy(dst, payload_buf_ + text_index, chunk1);
        std::memcpy(dst + chunk1, payload_buf_, chunk2);
    }

    // What if node being evicted mid-read?
    auto res = buf_pool_->Wrap(dst, pool_id, length);

    auto       expected = ctrl;
    const auto desired  = ControlBlock::Pack(
        NodeState::kReady, RefBit::kHot, ver, length, offset
    );
    node.control_block.compare_exchange_strong(
        expected, desired, std::memory_order_release, std::memory_order_relaxed
    );

    return res;
}

bool Arena::WritePayload(
    std::uint32_t nid, const std::uint8_t* in, std::uint32_t length
) noexcept {
    if (nid >= num_slots) {
        return false;
    }
    auto& node = nodes_[nid];

    const auto [state, ref, ver, old_len, old_off] = node.Load();
    if (state != NodeState::kPending) {
        return false;
    }

    const auto opt_offset = TryAllocateSpace(length);
    if (!opt_offset.has_value()) {
        return false;
    }

    const auto header_offset = opt_offset.value();
    const auto header_index  = ActualIndex(header_offset);

    const PayloadHeader header{PayloadHeader::kValidIdentifier, nid, length};
    std::memcpy(payload_buf_ + header_index, &header, sizeof(PayloadHeader));

    const auto text_index = header_index + sizeof(PayloadHeader);
    if (payload_buf_size - text_index >= length) {
        std::memcpy(payload_buf_ + text_index, in, length);
    } else {
        const auto chunk1 = payload_buf_size - text_index;
        const auto chunk2 = length - chunk1;
        std::memcpy(payload_buf_ + text_index, in, chunk1);
        std::memcpy(payload_buf_, in + chunk1, chunk2);
    }
    return true;
}

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

std::optional<std::uint64_t> Arena::TryAllocateSpace(
    std::uint32_t length
) noexcept {
    const std::size_t total_size = sizeof(PayloadHeader) + length;
    std::uint64_t     curr_write = write_head_.load(std::memory_order_relaxed);

    while (true) {
        std::uint64_t curr_read = read_tail_.load(std::memory_order_relaxed);
        if (curr_write + total_size - curr_read >= payload_buf_size) {
            return std::nullopt;  // Exhausted payload buffer
        }

        const auto header_index = ActualIndex(curr_write);
        const auto space_left   = payload_buf_size - header_index;

        auto padding = (space_left < sizeof(PayloadHeader)) ? space_left : 0;
        const std::uint64_t alloc_offset = curr_write + padding;

        const std::uint64_t next_write = alloc_offset + total_size;
        if (write_head_.compare_exchange_weak(
                curr_write, next_write, std::memory_order_relaxed
            )) {
            return alloc_offset;
        }
    }
}

}  // namespace strix::memory
