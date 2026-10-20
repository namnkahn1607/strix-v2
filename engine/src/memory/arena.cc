#include "memory/arena.h"

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
