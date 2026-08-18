package schemas

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBifrostLLMUsage_OriginalAndCompressedPromptTokensSerialization
// verifies that OriginalPromptTokens and CompressedPromptTokens fields:
// 1. Appear with correct JSON keys when set (non-nil)
// 2. Coexist with PromptTokens in the same JSON output
// 3. Are omitted when nil (omitempty behavior)
func TestBifrostLLMUsage_OriginalAndCompressedPromptTokensSerialization(t *testing.T) {
	orig := 1000
	comp := 300

	usage := BifrostLLMUsage{
		PromptTokens:            100,
		OriginalPromptTokens:    &orig,
		CompressedPromptTokens:  &comp,
		CompletionTokens:        50,
		TotalTokens:             150,
	}

	data, err := Marshal(usage)
	require.NoError(t, err)
	jsonStr := string(data)

	// All three fields appear in JSON
	assert.Contains(t, jsonStr, `"prompt_tokens":100`)
	assert.Contains(t, jsonStr, `"original_prompt_tokens":1000`)
	assert.Contains(t, jsonStr, `"compressed_prompt_tokens":300`)
	assert.Contains(t, jsonStr, `"completion_tokens":50`)
	assert.Contains(t, jsonStr, `"total_tokens":150`)

	// Roundtrip: unmarshal back and verify values
	var decoded BifrostLLMUsage
	err = Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, 100, decoded.PromptTokens)
	require.NotNil(t, decoded.OriginalPromptTokens)
	assert.Equal(t, 1000, *decoded.OriginalPromptTokens)
	require.NotNil(t, decoded.CompressedPromptTokens)
	assert.Equal(t, 300, *decoded.CompressedPromptTokens)
	assert.Equal(t, 50, decoded.CompletionTokens)
	assert.Equal(t, 150, decoded.TotalTokens)
}

// TestBifrostLLMUsage_NilCompressionFieldsOmitted verifies that when
// OriginalPromptTokens and CompressedPromptTokens are nil, the marshalled
// JSON does NOT contain the corresponding keys (omitempty behavior).
func TestBifrostLLMUsage_NilCompressionFieldsOmitted(t *testing.T) {
	usage := BifrostLLMUsage{
		PromptTokens:     200,
		CompletionTokens: 100,
		TotalTokens:      300,
	}

	data, err := Marshal(usage)
	require.NoError(t, err)
	jsonStr := string(data)

	// Existing fields still present
	assert.Contains(t, jsonStr, `"prompt_tokens":200`)
	assert.Contains(t, jsonStr, `"completion_tokens":100`)
	assert.Contains(t, jsonStr, `"total_tokens":300`)

	// New fields omitted when nil (omitempty)
	assert.NotContains(t, jsonStr, "original_prompt_tokens")
	assert.NotContains(t, jsonStr, "compressed_prompt_tokens")
}

// TestBifrostLLMUsage_CompressionFieldsRoundtripFull verifies a full
// roundtrip with all fields populated, including the new compression fields.
func TestBifrostLLMUsage_CompressionFieldsRoundtripFull(t *testing.T) {
	orig := 5000
	comp := 1200
	usage := BifrostLLMUsage{
		PromptTokens:            5000,
		OriginalPromptTokens:    &orig,
		CompressedPromptTokens:  &comp,
		CompletionTokens:        400,
		TotalTokens:             5400,
	}

	data, err := Marshal(usage)
	require.NoError(t, err)

	var decoded BifrostLLMUsage
	err = Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, usage.PromptTokens, decoded.PromptTokens)
	require.NotNil(t, decoded.OriginalPromptTokens)
	assert.Equal(t, *usage.OriginalPromptTokens, *decoded.OriginalPromptTokens)
	require.NotNil(t, decoded.CompressedPromptTokens)
	assert.Equal(t, *usage.CompressedPromptTokens, *decoded.CompressedPromptTokens)
	assert.Equal(t, usage.CompletionTokens, decoded.CompletionTokens)
	assert.Equal(t, usage.TotalTokens, decoded.TotalTokens)
}

// TestBifrostLLMUsage_PromptTokensCoexistWithNewFields verifies that
// PromptTokens continues to work correctly alongside the new pointer fields.
// The new fields should not affect the serialization of PromptTokens.
func TestBifrostLLMUsage_PromptTokensCoexistWithNewFields(t *testing.T) {
	t.Run("with_compression_fields_set", func(t *testing.T) {
		orig := 999
		comp := 333
		usage := BifrostLLMUsage{
			PromptTokens:           999,
			OriginalPromptTokens:   &orig,
			CompressedPromptTokens: &comp,
			CompletionTokens:       1,
			TotalTokens:            1000,
		}

		data, err := Marshal(usage)
		require.NoError(t, err)
		jsonStr := string(data)

		// PromptTokens value is the same as OriginalPromptTokens
		assert.Contains(t, jsonStr, `"prompt_tokens":999`)
		assert.Contains(t, jsonStr, `"original_prompt_tokens":999`)
		assert.Contains(t, jsonStr, `"compressed_prompt_tokens":333`)
		assert.Contains(t, jsonStr, `"completion_tokens":1`)
		assert.Contains(t, jsonStr, `"total_tokens":1000`)
	})

	t.Run("with_compression_fields_nil", func(t *testing.T) {
		usage := BifrostLLMUsage{
			PromptTokens:     777,
			CompletionTokens: 222,
			TotalTokens:      999,
		}

		data, err := Marshal(usage)
		require.NoError(t, err)
		jsonStr := string(data)

		// PromptTokens is present, compression fields are absent
		assert.Contains(t, jsonStr, `"prompt_tokens":777`)
		assert.NotContains(t, jsonStr, "original_prompt_tokens")
		assert.NotContains(t, jsonStr, "compressed_prompt_tokens")
	})
}