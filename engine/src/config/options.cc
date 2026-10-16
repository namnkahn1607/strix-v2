#include "config/options.h"

#include <algorithm>
#include <charconv>
#include <cstddef>
#include <cstdint>
#include <initializer_list>
#include <limits>
#include <optional>
#include <stdexcept>
#include <string>
#include <string_view>
#include <type_traits>

#include "config/arena.h"
#include "config/calibration.h"
#include "config/grpc.h"
#include "config/ivf.h"
#include "config/search.h"

namespace strix::config {

namespace {

using json = nlohmann::json;

namespace defaults {

constexpr Seconds kShutdownTimeout{3};

constexpr char kSocketAddress[] = "unix:///tmp/strix.sock";

constexpr std::uint32_t kNumClusters           = 1024;
constexpr std::uint32_t kMaxClusterSize        = 1024;
constexpr std::uint32_t kStagingBufferCapacity = 4096;

constexpr std::uint32_t kSampleSize              = 1 << 16;
constexpr std::uint32_t kRecalibrateTriggerCount = 1 << 14;
constexpr std::uint32_t kMiniBatchSize           = 1024;
constexpr std::uint32_t kMaxLloydIteration       = 16;
constexpr double        kMinConvergence          = 1e-4;
constexpr std::uint32_t kGracePeriodSeconds      = 1;

constexpr std::uint32_t kTopK = 8;

constexpr std::uint32_t kMaxSlots           = 1 << 19;
constexpr std::size_t   kPayloadBufferBytes = std::size_t{1} << 32;  // 4 GiB
constexpr std::uint32_t kBufferPoolCapacity = 64;

}  // namespace defaults

[[noreturn]] void Fail(const std::string& path, std::string_view msg) {
    throw std::invalid_argument(
        "config: " + (path.empty() ? "<root>" : path) + ": " + std::string(msg)
    );
}

std::string Join(const std::string& parent, std::string_view key) {
    return parent.empty() ? std::string(key)
                          : (parent + "." + std::string(key));
}

const json* Find(const json& obj, std::string_view key) {
    const auto it = obj.find(std::string(key));
    return it == obj.end() ? nullptr : &*it;
}

// A missing section is treated as an empty mapping so defaults apply.
// Otherwise a present but not a mapping section is an error.
const json& GetSection(
    const json& parent, const std::string& parent_path, std::string_view key
) {
    static const json kEmpty = json::object();

    const json* v = Find(parent, key);
    if (v == nullptr) return kEmpty;

    if (!v->is_object()) Fail(Join(parent_path, key), "must be a mapping");
    return *v;
}

// Typos like `top_K` should not fallback silently to defaults.
void RejectUnknownKeys(
    const json& obj, const std::string& path,
    std::initializer_list<std::string_view> allowed
) {
    for (auto it = obj.begin(); it != obj.end(); ++it) {
        const std::string& k = it.key();
        if (std::find(allowed.begin(), allowed.end(), std::string_view{k}) ==
            allowed.end()) {
            Fail(Join(path, k), "unknown key");
        }
    }
}

template <typename T>
T ParsePositive(const json& v, const std::string& path) {
    static_assert(std::is_integral_v<T>);
    if (!v.is_number_integer()) Fail(path, "must be an integer");

    std::uint64_t u = 0;
    if (v.is_number_unsigned()) {
        u = v.get<std::uint64_t>();
    } else {
        const auto s = v.get<std::int64_t>();
        if (s <= 0) Fail(path, "must be positive");
        u = static_cast<std::uint64_t>(s);
    }
    if (u == 0) Fail(path, "must be positive");

    constexpr auto kMax =
        static_cast<std::uint64_t>(std::numeric_limits<T>::max());
    if (u > kMax) Fail(path, "exceeds maximum " + std::to_string(kMax));
    return static_cast<T>(u);
}

template <typename T>
T GetPositive(
    const json& obj, const std::string& parent, std::string_view key,
    std::optional<T> fallback = std::nullopt
) {
    const std::string path = Join(parent, key);
    if (const json* v = Find(obj, key); v != nullptr) {
        return ParsePositive<T>(*v, path);
    }
    if (!fallback.has_value()) Fail(path, "is required");
    return fallback.value();
}

double GetDouble(
    const json& obj, const std::string& parent, std::string_view key,
    std::optional<double> fallback = std::nullopt
) {
    const std::string path = Join(parent, key);
    if (const json* v = Find(obj, key); v != nullptr) {
        if (!v->is_number()) Fail(path, "must be a number");
        return v->get<double>();
    }
    if (!fallback.has_value()) Fail(path, "is required");
    return fallback.value();
}

std::string GetNonEmptyString(
    const json& obj, const std::string& parent, std::string_view key,
    std::string_view fallback
) {
    const std::string path = Join(parent, key);
    if (const json* v = Find(obj, key); v != nullptr) {
        if (!v->is_string()) Fail(path, "must be a string");
        std::string s = v->get<std::string>();
        if (s.empty()) Fail(path, "must not be empty");
        return s;
    }
    return std::string(fallback);
}

// Accepts either a bare integer or `<digits>s`. Any other units are rejected.
Seconds GetDuration(
    const json& obj, const std::string& parent, std::string_view key,
    Seconds fallback
) {
    const std::string path = Join(parent, key);
    if (const json* v = Find(obj, key); v != nullptr) {
        if (v->is_number()) {
            return Seconds{ParsePositive<std::uint32_t>(*v, path)};
        }
        if (!v->is_string()) {
            Fail(path, "must be either an integer or string-like \"3s\"");
        }

        const std::string& s = v->get_ref<const std::string&>();
        if (s.size() < 2 || s.back() != 's') {
            Fail(path, "only the second suffix (s) is supported, e.g. \"3s\"");
        }

        std::uint32_t n     = 0;
        const char*   first = s.data();
        const char*   last  = first + s.size() - 1;  // exclude the trailing 's'
        const auto [ptr, ec] = std::from_chars(first, last, n);
        if (ec != std::errc{} || ptr != last) {
            Fail(path, "must be digits followed by 's', e.g. \"3s\"");
        }
        if (n == 0) Fail(path, "must be positive");
        return Seconds{n};
    }
    return fallback;
}

}  // namespace

Options::Options(
    ArenaOptions arena, GrpcOptions grpc, IvfOptions ivf,
    CalibrationOptions calibration, SearchOptions search,
    Seconds shutdown_timeout
)
    : arena_{arena}
    , grpc_{std::move(grpc)}
    , ivf_{ivf}
    , calibration_{calibration}
    , search_{search}
    , shutdown_timeout_{shutdown_timeout} {}

Options Options::FromJson(const nlohmann::json& j) {
    if (!j.is_object()) Fail("", "must be a mapping");
    RejectUnknownKeys(j, "", {"ipc", "engine"});

    ArenaOptions       arena{};
    GrpcOptions        grpc{};
    IvfOptions         ivf{};
    CalibrationOptions calib{};
    SearchOptions      search{};

    Seconds shutdown_timeout;

    // --- ipc ---
    const json& ipc = GetSection(j, "", "ipc");
    RejectUnknownKeys(ipc, "ipc", {"socket_address"});

    grpc.listen_addr = GetNonEmptyString(
        ipc, "ipc", "socket_address", defaults::kSocketAddress
    );

    // --- engine ---
    const json& engine = GetSection(j, "", "engine");
    RejectUnknownKeys(
        engine, "engine",
        {"shutdown_timeout", "rpc_server", "ivf", "calibration", "search",
         "storage"}
    );

    shutdown_timeout = GetDuration(
        engine, "engine", "shutdown_timeout", defaults::kShutdownTimeout
    );

    // --- engine.rpc_server ---
    const json& rpc = GetSection(engine, "engine", "rpc_server");
    RejectUnknownKeys(rpc, "engine.rpc_server", {"worker_pool_size"});

    grpc.num_threads = GetPositive<int>(
        rpc, "engine.rpc_server", "worker_pool_size"
    );  // REQUIRED

    // --- engine.ivf ---
    const json& ivf_j = GetSection(engine, "engine", "ivf");
    RejectUnknownKeys(
        ivf_j, "engine.ivf",
        {"num_clusters", "max_cluster_size", "staging_buffer_capacity"}
    );

    ivf.num_clusters = GetPositive<std::uint32_t>(
        ivf_j, "engine.ivf", "num_clusters", defaults::kNumClusters
    );
    ivf.max_cluster_size = GetPositive<std::uint32_t>(
        ivf_j, "engine.ivf", "max_cluster_size", defaults::kMaxClusterSize
    );
    ivf.staging_buffer_capacity = GetPositive<std::uint32_t>(
        ivf_j, "engine.ivf", "staging_buffer_capacity",
        defaults::kStagingBufferCapacity
    );

    // --- engine.calibration ---
    const json& calib_j = GetSection(engine, "engine", "calibration");
    RejectUnknownKeys(
        calib_j, "engine.calibration",
        {"sample_size", "recalibrate_trigger_count", "mini_batch_size",
         "max_lloyd_iteration", "min_convergence", "grace_period_seconds"}
    );

    calib.sample_size = GetPositive<std::uint32_t>(
        calib_j, "engine.calibration", "sample_size", defaults::kSampleSize
    );
    calib.recalibrate_trigger_count = GetPositive<std::uint32_t>(
        calib_j, "engine.calibration", "recalibrate_trigger_count",
        defaults::kRecalibrateTriggerCount
    );
    calib.mini_batch_size = GetPositive<std::uint32_t>(
        calib_j, "engine.calibration", "mini_batch_size",
        defaults::kMiniBatchSize
    );
    calib.max_lloyd_iteration = GetPositive<std::uint32_t>(
        calib_j, "engine.calibration", "max_lloyd_iteration",
        defaults::kMaxLloydIteration
    );
    calib.min_convergence = GetDouble(
        calib_j, "engine.calibration", "min_convergence",
        defaults::kMinConvergence
    );
    calib.grace_period = Seconds{GetPositive<std::uint32_t>(
        calib_j, "engine.calibration", "grace_period_seconds",
        defaults::kGracePeriodSeconds
    )};

    // --- engine.search ---
    const json& search_j = GetSection(engine, "engine", "search");
    RejectUnknownKeys(
        search_j, "engine.search", {"similarity_threshold", "top_k"}
    );

    search.similarity_threshold = static_cast<float>(
        GetDouble(search_j, "engine.search", "similarity_threshold")
    );  // REQUIRED
    search.top_k = GetPositive<std::uint32_t>(
        search_j, "engine.search", "top_k", defaults::kTopK
    );

    // --- engine.storage ---
    const json& storage_j = GetSection(engine, "engine", "storage");
    RejectUnknownKeys(
        storage_j, "engine.storage",
        {"max_slots", "payload_buffer_bytes", "buffer_pool_capacity"}
    );

    arena.max_slots = GetPositive<std::uint32_t>(
        storage_j, "engine.storage", "max_slots", defaults::kMaxSlots
    );
    arena.payload_buf_size = GetPositive<std::size_t>(
        storage_j, "engine.storage", "payload_buffer_bytes",
        defaults::kPayloadBufferBytes
    );
    arena.buf_pool_cap = GetPositive<std::uint32_t>(
        storage_j, "engine.storage", "buffer_pool_capacity",
        defaults::kBufferPoolCapacity
    );

    Options opts{arena, std::move(grpc), ivf, calib, search, shutdown_timeout};
    opts.Validate();
    return opts;
}

void Options::Validate() const {
    if (!(calibration_.min_convergence > 0.0) ||
        !std::isfinite(calibration_.min_convergence)) {
        Fail(
            "engine.calibration.min_convergence", "must be a finite number > 0"
        );
    }
    if (!(search_.similarity_threshold > 0.0f &&
          search_.similarity_threshold < 1.0f)) {
        Fail(
            "engine.search.similarity_threshold",
            "must be in the open interval (0, 1)"
        );
    }

    if (static_cast<std::uint64_t>(ivf_.max_cluster_size) * ivf_.num_clusters <
        arena_.max_slots) {
        Fail(
            "engine.ivf.max_cluster_size",
            "num_clusters * max_cluster_size must be >= max_slots "
            "(clusters cannot hold every slot)"
        );
    }

    if (ivf_.staging_buffer_capacity >= arena_.max_slots) {
        Fail(
            "engine.ivf.staging_buffer_capacity", "must be < storage.max_slots"
        );
    }
    if (calibration_.sample_size >= arena_.max_slots) {
        Fail("engine.calibration.sample_size", "must be < storage.max_slots");
    }
    if (calibration_.recalibrate_trigger_count >= arena_.max_slots) {
        Fail(
            "engine.calibration.recalibrate_trigger_count",
            "must be < storage.max_slots"
        );
    }

    if (ivf_.num_clusters > calibration_.sample_size) {
        Fail("engine.ivf.num_clusters", "must be <= calibration.sample_size");
    }
    if (calibration_.mini_batch_size > calibration_.sample_size) {
        Fail(
            "engine.calibration.mini_batch_size",
            "must be <= calibration.sample_size"
        );
    }
}

}  // namespace strix::config
