#include "memory/arena.h"

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

}  // namespace strix::memory
