#include "server/service_impl.h"

#include "cache.pb.h"
#include "inference/encoder.h"
#include "inference/vector.h"

namespace strix::service {

CacheServiceImpl::CacheServiceImpl(const inference::Encoder& encoder) : encoder_{encoder} {}

grpc::Status CacheServiceImpl::CheckCache([[maybe_unused]] grpc::ServerContext* context,
    const strix::v1::CheckCacheRequest*                                         request,
    [[maybe_unused]] strix::v1::CheckCacheResponse*                             response) {
    if (request->prompt().empty()) {
        return {grpc::StatusCode::INVALID_ARGUMENT, "Empty Prompt"};
    }

    inference::Vector query_vec;

    auto encode_err = encoder_.Encode(request->prompt().c_str(), query_vec);
    if (encode_err.has_value()) {
        switch (encode_err.value()) {
            case inference::EncodeErr::kTokenLimitExceeded:
                response->set_check_state(strix::v1::OVERFLOWN);
                return grpc::Status::OK;

            case inference::EncodeErr::kDegeneratedVector:
                response->set_check_state(strix::v1::DEGENERATED);
                return grpc::Status::OK;
        }
    }

    return grpc::Status::OK;
}

grpc::Status CacheServiceImpl::SetCache([[maybe_unused]] grpc::ServerContext* context,
    [[maybe_unused]] const strix::v1::SetCacheRequest*                        request,
    [[maybe_unused]] strix::v1::SetCacheResponse*                             response) {
    return grpc::Status::OK;
}

}  // namespace strix::service
