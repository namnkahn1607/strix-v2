#include "worker/identity.h"

#include <absl/log/check.h>

#include <atomic>

#include "base/treiber_stack.h"

namespace strix::worker {

namespace {

std::atomic<TreiberStack*> g_stack{nullptr};

class Slot {
public:
    Slot() = default;
    ~Slot() {
        if (owner_ != nullptr) owner_->Push(id_);
    }

    Slot(const Slot&)            = delete;
    Slot& operator=(const Slot&) = delete;

    void Bind(TreiberStack* owner, std::uint32_t id) {
        owner_ = owner;
        id_    = id;
    }

    bool Bound() const { return owner_ != nullptr; }

    std::uint32_t id() const { return id_; }

private:
    TreiberStack* owner_ = nullptr;
    std::uint32_t id_    = TreiberStack::kEmpty;
};

thread_local Slot tls_slot;

}  // namespace

void Init(int num_workers) {
    CHECK(num_workers > 0) << "num_workers into worker::Init() must be positive";
    auto* s = new TreiberStack(static_cast<std::uint32_t>(num_workers));

    TreiberStack* expected = nullptr;
    CHECK(g_stack.compare_exchange_strong(expected, s, std::memory_order_acq_rel))
        << "worker::Init() called twice";
}

void Register() noexcept {
    if (tls_slot.Bound()) return;
    auto* s = g_stack.load(std::memory_order_acquire);
    CHECK(s != nullptr) << "worker::Register() before Init()";
    const auto wid = s->Pop();
    CHECK(wid != TreiberStack::kEmpty) << "Out of worker ID";
    tls_slot.Bind(s, wid);
}

}  // namespace strix::worker
