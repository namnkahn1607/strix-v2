#pragma once

#include <cstdint>

namespace strix::memory {

// Header prepended to every payload in buffer.
// Enables constant reverse-lookup from payload to its `Node`.
struct alignas(4) PayloadHeader {
    static constexpr std::uint32_t kValidIdentifier = 0xDEADBEEFu;

    // Caller-supplied tag; used to verify header integrity.
    const std::uint32_t identifier;
    const std::uint32_t node_id;
    const std::uint32_t length;
};

static_assert(sizeof(PayloadHeader) == 12);

}  // namespace strix::memory
