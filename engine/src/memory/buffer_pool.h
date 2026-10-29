// Buffers payload for readers without extra allocation.

#pragma once

#include <absl/strings/cord.h>
#include <absl/strings/string_view.h>

#include <cstdint>
#include <memory>
#include <vector>

#include "base/treiber_stack.h"
#include "memory/control_block.h"

namespace strix::memory {

class BufPool {
public:
    explicit BufPool(std::uint32_t capacity) : stack_{capacity} {
        buffers_.reserve(capacity);
        for (std::uint32_t i = 0; i < capacity; ++i) {
            buffers_.push_back(std::make_unique_for_overwrite<uint8_t[]>(kMaxPayloadLength));
        }
    }

    BufPool(const BufPool&)            = delete;
    BufPool& operator=(const BufPool&) = delete;
    BufPool(BufPool&&)                 = delete;
    BufPool& operator=(BufPool&&)      = delete;

    // Acquires a raw buffer. Returns `nullptr` in case the pool is exhausted.
    // Upon success, caller must invokes `Wrap()` right after finish writing
    // data onto the buffer.
    std::uint8_t* Acquire(std::uint32_t* id) noexcept {
        const auto got = stack_.Pop();
        if (got == TreiberStack::kEmpty) {
            return nullptr;
        }

        *id = got;
        return buffers_[got].get();
    }

    // Wraps a buffer previously returned by `Acquire()` with an `absl::Cord`.
    // The buffer is released back into pool once its `ref_count` reaches 0.
    absl::Cord Wrap(std::uint8_t* buf, std::uint32_t id, std::uint32_t length) noexcept {
        return absl::MakeCordFromExternal(
            absl::string_view{reinterpret_cast<const char*>(buf), length},
            [this, id]() noexcept { this->stack_.Push(id); });
    }

private:
    TreiberStack stack_;

    // Backing array containing discrete buffer pointers.
    std::vector<std::unique_ptr<uint8_t[]>> buffers_;
};

}  // namespace strix::memory
