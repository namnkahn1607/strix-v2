#include "inference/sentence_encoder.h"

#include <array>
#include <cmath>
#include <cstddef>
#include <cstring>
#include <stdexcept>
#include <string>
#include <vector>

#include "inference/vector.h"

namespace strix::inference {

Ort::SessionOptions Encoder::CustomizeOptions() {
    Ort::SessionOptions options;

    options.EnableOrtCustomOps();
    options.SetGraphOptimizationLevel(ORT_ENABLE_ALL);

    // Single-threaded ORT execution: avoid spawning too many threads, which
    // introduces unnecessary context-switch overhead.
    options.SetInterOpNumThreads(1);
    options.SetIntraOpNumThreads(1);

    return options;
}

Encoder::Encoder(const char* tok_path, const char* bert_path)
    : env_{Ort::Env(ORT_LOGGING_LEVEL_ERROR, "onnx-env")}
    , options_{CustomizeOptions()}
    , tok_session_{env_, tok_path, options_}
    , bert_session_{env_, bert_path, options_} {}

std::optional<EncodeErr> Encoder::Encode(
    const char* prompt, Vector& out
) const {
    const Ort::AllocatorWithDefaultOptions allocator;

    // PHASE 1: Tokenization
    // Tokenize the input text into word piece sequence.

    const std::vector<std::int64_t> input_shape{1};

    auto text_tensor = Ort::Value::CreateTensor(
        allocator, input_shape.data(), input_shape.size(),
        ONNX_TENSOR_ELEMENT_DATA_TYPE_STRING
    );
    text_tensor.FillStringTensor(&prompt, 1);

    const char* tok_input_name[]{"text"};
    const char* tok_output_names[]{
        "input_ids", "attention_mask", "token_type_ids"
    };
    auto tok_output = tok_session_.Run(
        Ort::RunOptions{nullptr}, tok_input_name, &text_tensor, 1,
        tok_output_names, 3
    );

    // PHASE 2: Boundary validation
    // input_ids has shape int64[seq_length] (dynamic axis).
    // Reject inputs that exceed the model's context window.

    const Ort::Value& input_ids_tensor = tok_output.front();
    const auto shape = input_ids_tensor.GetTensorTypeAndShapeInfo().GetShape();

    const auto seq_length = static_cast<std::size_t>(shape.front());
    if (seq_length > kMaxTokens) {
        return EncodeErr::kTokenLimitExceeded;
    }

    // PHASE 3: Reshape transformer input
    // From int64[seq_length] to int64[1, seq_length] (batch_size = 1).

    const std::array<std::int64_t, 2> bert_input_shape{
        1, static_cast<std::int64_t>(seq_length)
    };

    // Reuse allocator info of existing tensor so the new views
    // point to the same memory.
    const auto mem_info = input_ids_tensor.GetTensorMemoryInfo();

    std::vector<Ort::Value> bert_inputs;
    bert_inputs.reserve(3);
    for (std::size_t i = 0; i < 3; ++i) {
        auto* raw_data = tok_output[i].GetTensorMutableData<std::int64_t>();
        bert_inputs.push_back(
            Ort::Value::CreateTensor<std::int64_t>(
                mem_info, raw_data, seq_length, bert_input_shape.data(),
                bert_input_shape.size()
            )
        );
    }

    // PHASE 4: Transformation
    // Turn the word piece sequence to array of embeddings.

    const char* bert_input_names[]{
        "input_ids", "attention_mask", "token_type_ids"
    };
    const char* bert_output_names[]{"last_hidden_state"};
    const auto  bert_output = bert_session_.Run(
        Ort::RunOptions{nullptr}, bert_input_names, bert_inputs.data(), 3,
        bert_output_names, 1
    );

    // PHASE 5: Output shape validation
    // Expected shape: float[1, seq_length, kVectorDim].

    const Ort::Value& output_tensor = bert_output.front();
    const auto        output_shape =
        output_tensor.GetTensorTypeAndShapeInfo().GetShape();

    if (output_shape.size() != 3) {
        throw std::runtime_error(
            "Unexpected output rank from transformer: expected 3, got " +
            std::to_string(output_shape.size())
        );
    }
    if (output_shape.front() != 1) {
        throw std::runtime_error(
            "Unexpected batch dimension from transformer: expected 1, got " +
            std::to_string(output_shape.front())
        );
    }

    const auto actual_dim = static_cast<std::size_t>(output_shape[2]);
    if (actual_dim != kVectorDim) {
        throw std::runtime_error(
            "Transformer output dimension mismatch: expected " +
            std::to_string(kVectorDim) + ", got " + std::to_string(actual_dim)
        );
    }

    // PHASE 6: Mean pooling
    // Collapse float[1, seq_length, vector_dim] to float[vector_dim] by
    // averaging across sequence dimension.

    const float* __restrict src = output_tensor.GetTensorData<float>();

    float* dst = out.data();
    std::memset(dst, 0, actual_dim * sizeof(float));

    const float inv_seq_len = 1.0f / static_cast<float>(seq_length);
    for (std::size_t i = 0; i < seq_length; ++i) {
        for (std::size_t j = 0; j < actual_dim; ++j) {
            dst[j] += src[i * actual_dim + j] * inv_seq_len;
        }
    }

    // PHASE 7: Normalization
    // Scale ||embedding vector|| to 1.0.

    float sum_sq = 0.0f;
    for (std::size_t i = 0; i < actual_dim; ++i) {
        sum_sq += dst[i] * dst[i];
    }

    if (sum_sq < 1e-9f) {
        // A near-zero norm indicates a degenerated vector.
        return EncodeErr::kDegeneratedVector;
    }

    const float inv_norm = 1.0f / std::sqrt(sum_sq);
    for (std::size_t i = 0; i < actual_dim; ++i) {
        dst[i] *= inv_norm;
    }

    return std::nullopt;
}

}  // namespace strix::inference
