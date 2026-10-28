#pragma once

#include <cstdint>

namespace strix::worker {

// Must be initialized before any thread invoking `Register()`.
// Abort upon second invocation or `num_workers == 0`.
void Init(int num_workers);

void Register() noexcept;

std::uint32_t ThreadID() noexcept;

}  // namespace strix::worker
