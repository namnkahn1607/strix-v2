#include "service/service_impl.h"

namespace strix::service {

grpc::Status CacheServiceImpl::CheckCache(
    grpc::ServerContext* context, const strix::v1::CheckCacheRequest* request,
    strix::v1::CheckCacheResponse* response
) {
    (void)context;
    (void)request;
    (void)response;
    return grpc::Status::OK;
}

grpc::Status CacheServiceImpl::SetCache(
    [[maybe_unused]] grpc::ServerContext* context,
    const strix::v1::SetCacheRequest*     request,
    strix::v1::SetCacheResponse*          response
) {
    (void)request;
    (void)response;
    return grpc::Status::OK;
}

}  // namespace strix::service
