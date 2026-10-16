#pragma once

#include <string>

namespace strix::config {

struct GrpcOptions {
    std::string listen_addr;
    int         num_threads;
};

}  // namespace strix::config
