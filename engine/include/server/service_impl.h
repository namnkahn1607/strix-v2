#pragma once

#include <grpcpp/server_context.h>
#include <grpcpp/support/status.h>

#include "cache.grpc.pb.h"
#include "cache.pb.h"
#include "inference/encoder.h"

namespace strix::service {

class CacheServiceImpl final : public strix::v1::CacheService::Service {
public:
    explicit CacheServiceImpl(const inference::Encoder& encoder);

    CacheServiceImpl(const CacheServiceImpl&)           = delete;
    CacheServiceImpl operator=(const CacheServiceImpl&) = delete;

    grpc::Status CheckCache(grpc::ServerContext* context,
        const strix::v1::CheckCacheRequest*      request,
        strix::v1::CheckCacheResponse*           response) override;

    grpc::Status SetCache(grpc::ServerContext* context, const strix::v1::SetCacheRequest* request,
        strix::v1::SetCacheResponse* response) override;

private:
    const inference::Encoder& encoder_;
};

}  // namespace strix::service
