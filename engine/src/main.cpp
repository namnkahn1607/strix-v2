#include <grpcpp/health_check_service_interface.h>
#include <grpcpp/resource_quota.h>
#include <grpcpp/server_builder.h>
#include <poll.h>
#include <signal.h>
#include <sys/poll.h>
#include <sys/signalfd.h>
#include <unistd.h>

#include <algorithm>
#include <charconv>
#include <csignal>
#include <cstddef>
#include <cstdlib>
#include <iostream>
#include <nlohmann/json.hpp>
#include <stdexcept>
#include <string>
#include <system_error>

#include "cache.grpc.pb.h"
#include "config/grpc.h"
#include "config/options.h"
#include "inference/encoder.h"
#include "server/service_impl.h"
#include "worker/identity.h"

using namespace strix;

namespace {

// Matches the capacity of a Unix pipe (16 KiB).
constexpr std::size_t kMaxPipeBytes = 1u << 16;

[[noreturn]] void ThrowErrno(int en, const char* what) {
    throw std::system_error(en, std::generic_category(), what);
}

class UniqueFd {
public:
    explicit UniqueFd(int fd) noexcept : fd_{fd} {};
    ~UniqueFd() { Reset(); }

    [[nodiscard]] int Get() const noexcept { return fd_; }

private:
    void Reset() noexcept {
        if (fd_ >= 0) {
            ::close(fd_);
            fd_ = -1;
        }
    }

    int fd_ = -1;
};

std::string RequireEnv(const char* name) {
    const char* val = std::getenv(name);  // NOLINT(concurrency-mt-unsafe)
    if (val == nullptr) throw std::runtime_error(std::string{name} + " not set");
    return val;
}

int ParseFd(const std::string& s) {
    int         fd  = -1;
    const char* end = s.data() + s.size();

    const auto [ptr, ec] = std::from_chars(s.data(), end, fd);
    if (ec != std::errc{} || ptr != end || fd < 0) {
        throw std::runtime_error("PARENT_PIPE_FD is not a valid fd: '" + s + "'");
    }

    return fd;
}

struct Env {
    std::string tok_path;
    std::string bert_path;
    int         pipe_fd;

    static Env Load() {
        return Env{
            .tok_path  = RequireEnv("TOKENIZER_PATH"),
            .bert_path = RequireEnv("TRANSFORMER_PATH"),
            .pipe_fd   = ParseFd("PARENT_PIPE_FD")
        };
    }
};

UniqueFd BlockSignalAndOpenFd() {
    sigset_t mask;
    sigemptyset(&mask);
    sigaddset(&mask, SIGINT);
    sigaddset(&mask, SIGTERM);

    const int rc = pthread_sigmask(SIG_BLOCK, &mask, nullptr);
    if (rc != 0) ThrowErrno(rc, "pthread_sigmask");

    const int fd = signalfd(-1, &mask, SFD_CLOEXEC);
    if (fd == -1) ThrowErrno(errno, "signalfd");

    return UniqueFd{fd};
}

nlohmann::json ReadConfig(int fd) {
    std::string line;
    char        chunk[4'096];

    while (true) {
        const ssize_t n = ::read(fd, chunk, sizeof(chunk));
        if (n == -1) {
            if (errno == EINTR) continue;
            ThrowErrno(errno, "read(parent pipe)");
        }
        if (n == 0) throw std::runtime_error("parent closed pipe before sending config");

        const char* const end = chunk + n;
        line.append(chunk);
        const char* const nl = std::find(static_cast<const char*>(chunk), end, '\n');
        line.append(nl);

        if (line.size() > kMaxPipeBytes) throw std::runtime_error("config exceeds size limit");
        if (nl != end) break;
    }

    return nlohmann::json::parse(line);
}

enum class Stop : std::uint8_t { kOsSignal, kParentSignal };

const char* Describe(Stop s) {
    return s == Stop::kOsSignal ? "OS termination signal" : "parent death signal";
}

void Configure(grpc::ServerBuilder& builder, const config::GrpcOptions& opt) {
    builder.AddListeningPort(opt.listen_addr, grpc::InsecureServerCredentials());

    grpc::ResourceQuota quota;
    quota.SetMaxThreads(opt.num_threads);

    builder.SetResourceQuota(quota)
        .SetSyncServerOption(grpc::ServerBuilder::SyncServerOption::NUM_CQS, 1)
        .SetSyncServerOption(grpc::ServerBuilder::SyncServerOption::MIN_POLLERS, opt.num_threads)
        .SetSyncServerOption(grpc::ServerBuilder::SyncServerOption::MAX_POLLERS, opt.num_threads);
}

Stop WaitForStop(int sig_fd, int pipe_fd) {
    pollfd fds[2] = {
        {.fd = sig_fd, .events = POLLIN, .revents = 0},
        {.fd = pipe_fd, .events = 0, .revents = 0},
    };

    while (true) {
        if (::poll(fds, 2, -1) == -1) {
            if (errno == EINTR) continue;
            ThrowErrno(errno, "poll");
        }
        if ((fds[0].revents & POLLIN) != 0) return Stop::kOsSignal;
        if ((fds[1].revents & (POLLHUP | POLLERR)) != 0) return Stop::kParentSignal;
    }
}

Stop Run(
    service::CacheServiceImpl& cache_svc, const config::Options& opt, int sig_fd, int pipe_fd
) {
    grpc::ServerBuilder builder;
    Configure(builder, opt.Grpc());
    builder.RegisterService(&cache_svc);

    const std::unique_ptr<grpc::Server> server{builder.BuildAndStart()};
    if (server == nullptr) {
        throw std::runtime_error("failed to start gRPC server on " + opt.Grpc().listen_addr);
    }

    grpc::HealthCheckServiceInterface* health = server->GetHealthCheckService();
    if (health != nullptr) {
        health->SetServingStatus(v1::CacheService::service_full_name(), true);
    }

    std::cout << "gRPC server listening on " << opt.Grpc().listen_addr << '\n' << std::flush;

    auto shutdown = [&] {
        if (health != nullptr) health->Shutdown();
        server->Shutdown(std::chrono::system_clock::now() + opt.ShutdownTimeout());
    };

    Stop why{};
    try {
        why = WaitForStop(sig_fd, pipe_fd);
    } catch (...) {
        shutdown();
        throw;
    }

    shutdown();
    return why;
}

}  // namespace

int main() {
    try {
        const UniqueFd sig_fd = BlockSignalAndOpenFd();

        const auto [tok_path, bert_path, pipe_fd] = Env::Load();
        const UniqueFd parent_pipe{pipe_fd};

        const auto opts = config::Options::FromJson(ReadConfig(parent_pipe.Get()));

        grpc::EnableDefaultHealthCheckService(true);  // Process-global

        worker::Init(opts.Grpc().num_threads);
        inference::Encoder        encoder{tok_path.c_str(), bert_path.c_str()};
        service::CacheServiceImpl cache_svc{encoder};

        const Stop why = Run(cache_svc, opts, sig_fd.Get(), parent_pipe.Get());
        std::cerr << "Shutting down: " << Describe(why) << '\n';

    } catch (const std::exception& e) {
        std::cerr << "fatal: " << e.what() << '\n';
        return 1;
    }

    return 0;
}
