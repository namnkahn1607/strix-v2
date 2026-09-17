#include <grpcpp/server.h>
#include <grpcpp/server_builder.h>

#include <chrono>
#include <memory>

#include "service/service_impl.h"

using namespace strix;

int main() {
    constexpr std::chrono::seconds kShutdownTimeout{5};

    const char* kServerAddr = "unix:///tmp/strix.sock";
    const char* kSocketPath = "/tmp/strix.sock";

    unlink(kSocketPath);

    service::CacheServiceImpl service;

    grpc::ServerBuilder builder;
    builder.AddListeningPort(kServerAddr, grpc::InsecureServerCredentials());
    builder.RegisterService(&service);

    const std::unique_ptr<grpc::Server> server{builder.BuildAndStart()};
    if (!server) {
        std::cerr << "Failed to start gRPC server\n";
        return 1;
    }

    std::cout << "Opening gRPC server - listening on " << kServerAddr << '\n';

    const auto deadline = std::chrono::system_clock::now() + kShutdownTimeout;
    server->Shutdown(deadline);

    std::cout << "Closing gRPC server" << '\n';

    return 0;
}
