// Control block bit layout (64 bits):
//   Bits 63–62  (2 bits)  : NodeState
//   Bit  61     (1 bit)   : EvictState  (CLOCK reference bit)
//   Bits 60-57  (4 bits)  : Version     (seqlock-style versioning)
//   Bits 56–36  (21 bits) : Payload length
//   Bits 35–0   (36 bits) : Virtual offset

#pragma once

#include <cstdint>

#include "state.h"

namespace strix::memory {

inline constexpr std::uint32_t kNodeStateShift = 62;
inline constexpr std::uint32_t kRefBitShift    = 61;
inline constexpr std::uint32_t kVersionShift   = 57;
inline constexpr std::uint32_t kLengthShift    = 36;

inline constexpr std::uint64_t kNodeStateMask     = 0x3;
inline constexpr std::uint64_t kRefBitMask        = 0x1;
inline constexpr std::uint64_t kVersionMask       = 0xF;
inline constexpr std::uint32_t kMaxPayloadLength  = 0x1F'FFFFu;
inline constexpr std::uint64_t kVirtualOffsetMask = 0xF'FFFF'FFFFull;

// Increments a 4-bit version counter with wrap-around.
// Use only after a worker has successfully acquired a node and committing its
// new vector data onto that node.
inline std::uint8_t NextVersion(std::uint8_t ver) noexcept {
    // This asymmetry lets a seqlock-style version check (snapshot -> read ->
    // snapshot again) detects ownership change mid-read.
    return (ver + 1) & kVersionMask;
}

// Decoded view of control block.
struct ControlBlock {
    NodeState     state;   // Current lifecycle state of the node.
    RefBit        ref;     // Reference bit used by CLOCK algorithm.
    std::uint8_t  ver;     // Seqlock-style version counter.
    std::uint32_t length;  // Payload length in bytes (max 2 MiB - 1 byte).
    std::uint64_t offset;  // Virtual buffer offset (max 64 GiB).

    static std::uint64_t Pack(
        NodeState state, RefBit ref, std::uint8_t ver, std::uint32_t length, std::uint64_t offset
    ) noexcept {
        auto state64 = static_cast<uint64_t>(state);
        auto ref64   = static_cast<uint64_t>(ref);
        auto ver64   = static_cast<uint64_t>(ver);
        auto len64   = static_cast<uint64_t>(length);
        return ((state64 & kNodeStateMask) << kNodeStateShift) |
               ((ref64 & kRefBitMask) << kRefBitShift) | ((ver64 & kVersionMask) << kVersionShift) |
               (len64 << kLengthShift) | (offset & kVirtualOffsetMask);
    }

    static ControlBlock Unpack(std::uint64_t ctrl) noexcept {
        return {
            static_cast<NodeState>((ctrl >> kNodeStateShift) & kNodeStateMask),
            static_cast<RefBit>((ctrl >> kRefBitShift) & kRefBitMask),
            static_cast<std::uint8_t>((ctrl >> kVersionShift) & kVersionMask),
            static_cast<std::uint32_t>((ctrl >> kLengthShift) & kMaxPayloadLength),
            ctrl & kVirtualOffsetMask
        };
    }
};

static_assert(kRefBitShift + 1 == kNodeStateShift, "ref bit after state");
static_assert(kVersionShift + 4 == kRefBitShift, "4-bit ver after ref bit");
static_assert(kLengthShift + 21 == kVersionShift, "21-bit length after ver");
static_assert(kLengthShift == 36, "36-bit offset at last");
static_assert(
    ((kNodeStateMask << kNodeStateShift) | (kRefBitMask << kRefBitShift) |
     (kVersionMask << kVersionShift) |
     (static_cast<std::uint64_t>(kMaxPayloadLength) << kLengthShift) | kVirtualOffsetMask) == ~0ull,
    "All fields form an exact 64-bit partition"
);

}  // namespace strix::memory
