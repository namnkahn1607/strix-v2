#include <grpcpp/health_check_service_interface.h>
#include <grpcpp/server.h>
#include <grpcpp/server_builder.h>

#include <chrono>
#include <cstdlib>
#include <memory>

#include "inference/encoder.h"
#include "server/service_impl.h"

using namespace strix;

int main() {
    constexpr std::chrono::seconds kShutdownTimeout{5};

    const char* kServerAddr = "unix:///tmp/strix.sock";
    const char* kSocketPath = "/tmp/strix.sock";

    // NOLINTBEGIN(concurrency-mt-unsafe)
    const char* tok_path  = std::getenv("TOKENIZER_PATH");
    const char* bert_path = std::getenv("TRANSFORMER_PATH");
    // NOLINTEND(concurrency-mt-unsafe)

    inference::Encoder        encoder{tok_path, bert_path};
    service::CacheServiceImpl service{encoder};

    grpc::EnableDefaultHealthCheckService(true);

    grpc::ServerBuilder builder;
    builder.AddListeningPort(kServerAddr, grpc::InsecureServerCredentials());
    builder.RegisterService(&service);

    unlink(kSocketPath);

    const std::unique_ptr<grpc::Server> server{builder.BuildAndStart()};
    if (!server) {
        std::cerr << "Failed to start gRPC server\n";
        return 1;
    }

    std::cout << "Opening gRPC server - listening on " << kServerAddr << '\n';

    grpc::HealthCheckServiceInterface* health_service =
        server->GetHealthCheckService();
    if (health_service) {
        // Mark service as SERVING - ready to accept requests.
        health_service->SetServingStatus("strix.v1.CacheService", true);
    }

    const auto deadline = std::chrono::system_clock::now() + kShutdownTimeout;
    server->Shutdown(deadline);

    std::cout << "Closing gRPC server\n";

    return 0;
}
