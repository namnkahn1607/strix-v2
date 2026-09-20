// Custom allocator/deallocator based on mmap/munmap.

#pragma once

#include <absl/log/check.h>
#include <absl/log/log.h>
#include <sys/mman.h>

#include <cstddef>
#include <stdexcept>
#include <system_error>

namespace strix::memory {

// Allocates an anonymous private read-write mapping that's aligned to
// system page size (typically 4 KiB on Linux x86-64).
// Caller owns the memory lifetime, and must pair with `Dealloc<T>` using
// exact argument value `count`.
template <typename T>
T* Alloc(std::size_t count) {
    if (count == 0) {
        throw std::invalid_argument("item count must be positive");
    }
    const std::size_t size = count * sizeof(T);

    int   flags = MAP_ANONYMOUS | MAP_PRIVATE | MAP_POPULATE;
    void* ptr   = mmap(nullptr, size, PROT_READ | PROT_WRITE, flags, -1, 0);
    if (ptr == MAP_FAILED) {
        throw std::system_error(errno, std::generic_category(), "mmap failed");
    }

    return static_cast<T*>(ptr);
}

// Deallocates a memory region previously allocated via `Alloc<T>`.
// Type argument `T` and value `count` must match ones passed onto `Alloc<T>`.
template <typename T>
void Dealloc(T* ptr, std::size_t count) noexcept {
    CHECK(ptr != nullptr && count != 0)
        << "invalid ptr/size pair (ptr=" << ptr << ", count=" << count << ")";
    const std::size_t size = count * sizeof(T);

    if (munmap(ptr, size) != 0) {
        LOG(FATAL) << "munmap failed: "
                   << std::error_code(errno, std::generic_category()).message()
                   << " (ptr=" << ptr << ", size=" << size << ")";
    }
}

}  // namespace strix::memory
