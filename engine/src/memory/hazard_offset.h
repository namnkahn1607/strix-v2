// Hazard offsets: Hazard pointers 4 byte ranges.

#pragma once

#include <array>
#include <atomic>
#include <optional>

#include "memory/control_block.h"

namespace strix::memory {

// "Hazard zone" structure: `[offset, offset + length)`.
// Used by payload reader to establish protection against GC eviction.
struct HazardRange {
    std::uint64_t offset;
    std::uint32_t length;
};

// `N` should equal the number of workers in gRPC thread pool.
template <size_t N>
class HazardTable final {
public:
    // Publishes a new "hazard zone" in payload buffer.
    void Publish(std::uint32_t wid, std::uint64_t offset, std::uint32_t length) noexcept {
        slots_[wid].value.store(Pack(offset, length), std::memory_order_release);
    }

    // Detaches an existing "hazard zone" in payload buffer.
    void Clear(std::uint32_t wid) noexcept {
        slots_[wid].value.store(0, std::memory_order_release);
    }

    // Checks if the specified range `[offset, offset + length)` intersects with
    // any published hazard zones.
    bool Overlaps(std::uint64_t offset, std::uint32_t length) const noexcept {
        for (auto& s : slots_) {
            auto h = Unpack(s.value.load(std::memory_order_acquire));
            if (h.has_value() && Intersect(offset, length, h.value().offset, h.value().length)) {
                return true;
            }
        }
        return false;
    }

private:
    // 8-byte representation of `HazardRange` in table.
    struct alignas(64) Slot {
        std::atomic<std::uint64_t> value{0};
    };

    static constexpr std::uint32_t kHazardActiveShift = 63;
    static constexpr std::uint32_t kHazardLengthShift = 42;
    static constexpr std::uint64_t kHazardLengthMask  = 0x1FFFFFull;

    // Packs `offset` (36-bit) and `length` (21-bit) into a 64-bit word.
    static std::uint64_t Pack(std::uint64_t offset, std::uint32_t length) noexcept {
        return (1ull << kHazardActiveShift) |
               ((static_cast<std::uint64_t>(length) & kHazardLengthMask) << kHazardLengthShift) |
               (offset & kVirtualOffsetMask);
    }

    // Tries unpacking a 64-bit word into a `HazardRange` instance.
    static std::optional<HazardRange> Unpack(std::uint64_t word) noexcept {
        if ((word >> kHazardActiveShift) == 0) {
            return std::nullopt;
        }
        return HazardRange{
            /*offset=*/word & kVirtualOffsetMask,
            /*length=*/static_cast<uint32_t>((word >> kHazardLengthShift) & kHazardLengthMask)
        };
    }

    // Checks if 2 ranges `[ao, ao + al)` and `[bo, bo + bl)` intersect.
    static bool Intersect(
        std::uint64_t ao, std::uint32_t al, std::uint64_t bo, std::uint32_t bl
    ) noexcept {
        return ao < bo + bl && bo < ao + al;
    }

    std::array<Slot, N> slots_{};
};

}  // namespace strix::memory
