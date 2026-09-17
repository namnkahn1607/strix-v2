#pragma once

#include <grpcpp/server_context.h>
#include <grpcpp/support/status.h>

#include "cache.grpc.pb.h"
#include "cache.pb.h"

namespace strix::service {

class CacheServiceImpl final : public strix::v1::CacheService::Service {
public:
    explicit CacheServiceImpl() = default;
    ~CacheServiceImpl()         = default;

    grpc::Status CheckCache(
        grpc::ServerContext*                context,
        const strix::v1::CheckCacheRequest* request,
        strix::v1::CheckCacheResponse*      response
    ) override;

    grpc::Status SetCache(
        grpc::ServerContext* context, const strix::v1::SetCacheRequest* request,
        strix::v1::SetCacheResponse* response
    ) override;
};

}  // namespace strix::service
