#include "memory/arena.h"

#include <atomic>
#include <cstring>
#include <optional>
#include <stdexcept>

#include "allocator.h"
#include "inference/vector.h"

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

    const auto text_offset  = opt_offset.value();
    const auto actual_index = ActualIndex(text_offset);
    if (payload_buf_size - actual_index >= length) {
        std::memcpy(payload_buf_ + actual_index, in, length);
    } else {
        const auto chunk1 = payload_buf_size - actual_index;
        const auto chunk2 = length - chunk1;
        std::memcpy(payload_buf_ + actual_index, in, chunk1);
        std::memcpy(payload_buf_, in + chunk1, chunk2);
    }
    return true;
}

std::optional<std::uint64_t> Arena::TryAllocateSpace(
    std::uint32_t length
) noexcept {
    std::uint64_t curr_write = write_head.load(std::memory_order_relaxed);
    std::uint64_t next_write = curr_write + length;

    while (true) {
        std::uint64_t curr_read = read_tail.load(std::memory_order_relaxed);
        if (curr_write + length - curr_read >= payload_buf_size) {
            return std::nullopt;  // Exhausted payload buffer
        }

        if (write_head.compare_exchange_weak(
                curr_write, next_write, std::memory_order_relaxed
            )) {
            return curr_write;
        }
    }
}

}  // namespace strix::memory
