#include "memory/arena.h"

#include <atomic>
#include <cstring>
#include <optional>
#include <stdexcept>

#include "allocator.h"
#include "buffer_pool.h"
#include "inference/vector.h"
#include "memory/control_block.h"
#include "memory/state.h"
#include "payload_header.h"

namespace strix::memory {

Arena::Arena(std::uint32_t num_slots, std::size_t payload_buf_size)
    : num_slots{num_slots}, payload_buf_size{payload_buf_size} {
    if (num_slots == 0) {
        throw std::invalid_argument("Arena slots must be non-zero");
    }

    nodes_   = Alloc<Node>(num_slots);
    vectors_ = Alloc<float>(num_slots * inference::kVectorDim);

    if (payload_buf_size == 0) {
        throw std::invalid_argument("Payload buffer size must be non-zero");
    } else if ((payload_buf_size & (payload_buf_size - 1)) != 0) {
        throw std::invalid_argument("Payload buffer size must be a power of 2");
    }

    payload_buf_ = Alloc<std::uint8_t>(payload_buf_size);
}

Arena::~Arena() {
    Dealloc<Node>(nodes_, num_slots);
    Dealloc<float>(vectors_, num_slots * inference::kVectorDim);
    Dealloc<std::uint8_t>(payload_buf_, payload_buf_size);
}

std::optional<absl::Cord> Arena::ReadPayload(
    std::uint32_t node_id
) const noexcept {
    auto& node = nodes_[node_id];
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
    std::uint32_t node_id, const std::uint8_t* in, std::uint32_t length
) noexcept {
    if (node_id >= num_slots) {
        return false;
    }
    auto& node = nodes_[node_id];

    const auto [state, ref, ver, old_len, old_off] = node.LoadCtrl();
    if (state != NodeState::kPending) {
        return false;
    }

    const auto opt_offset = TryAllocateSpace(length);
    if (!opt_offset.has_value()) {
        return false;
    }

    const auto header_offset = opt_offset.value();
    const auto header_index  = ActualIndex(header_offset);

    const PayloadHeader header{
        PayloadHeader::kValidIdentifier, node_id, length
    };
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

        const auto text_len  = header->length;
        const auto total_len = sizeof(PayloadHeader) + text_len;

        auto& node = nodes_[header->node_id];

        const auto [state, ref, ver, length, offset] = node.LoadCtrl();
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
