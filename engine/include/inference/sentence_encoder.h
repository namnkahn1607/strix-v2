#pragma once

#include <onnxruntime_cxx_api.h>

#include <cstdint>
#include <optional>

#include "vector.h"

namespace strix::inference {

// Predictable, non-fatal encoding failures.
enum class EncodeErr : std::uint8_t { kTokenLimitExceeded, kDegeneratedVector };

// Owns 2 ORT sessions to a tokenizer and a transformer.
// Both sessions run sequentially to produce embedding vector of given prompt.
class Encoder final {
public:
    explicit Encoder(const char* tok_path, const char* bert_path);

    Encoder(const Encoder&)            = delete;
    Encoder& operator=(const Encoder&) = delete;
    Encoder(Encoder&&)                 = delete;
    Encoder& operator=(Encoder&&)      = delete;

    // Vectorizes a prompt string to produce an embedding vector.
    // Returns `EncodeErr` on expected failures, otherwise throws.
    // Thread-safe, as ORT sessions are stateless per-run provided `options_` is
    // not shared across calls.
    std::optional<EncodeErr> Encode(const char* prompt, Vector& out) const;

private:
    static Ort::SessionOptions CustomizeOptions();

    Ort::Env            env_;
    Ort::SessionOptions options_;

    mutable Ort::Session tok_session_;   // Tokenizer
    mutable Ort::Session bert_session_;  // Transformer
};

};  // namespace strix::inference
