#pragma once

#include <chrono>
#include <nlohmann/json.hpp>

#include "arena.h"
#include "calibration.h"
#include "grpc.h"
#include "ivf.h"
#include "search.h"

namespace strix::config {

using Seconds = std::chrono::seconds;

// Immutable, fully-validated Engine configurations.
class Options {
public:
    // Throws `std::invalid_argument("config: <dotted.path>: <reason>")` upon
    // any parse or validation failure.
    static Options FromJson(const nlohmann::json& j);

    const ArenaOptions&       Arena() const noexcept { return arena_; }
    const GrpcOptions&        Grpc() const noexcept { return grpc_; }
    const IvfOptions&         Ivf() const noexcept { return ivf_; }
    const SearchOptions&      Search() const noexcept { return search_; }
    const CalibrationOptions& Calibration() const noexcept { return calibration_; }

    Seconds ShutdownTimeout() const noexcept { return shutdown_timeout_; }

private:
    explicit Options(
        ArenaOptions arena, GrpcOptions grpc, IvfOptions ivf, CalibrationOptions calibration,
        SearchOptions search, Seconds shutdown_timeout
    );

    void Validate() const;

    ArenaOptions       arena_;
    GrpcOptions        grpc_;
    IvfOptions         ivf_;
    CalibrationOptions calibration_;
    SearchOptions      search_;

    Seconds shutdown_timeout_;
};

}  // namespace strix::config
