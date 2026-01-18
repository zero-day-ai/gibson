package embedder

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateEmbedder_Native(t *testing.T) {
	config := EmbedderConfig{
		Provider: "native",
	}

	emb, err := CreateEmbedder(config)
	require.NoError(t, err, "native embedder should initialize successfully")
	require.NotNil(t, emb)
	assert.Equal(t, "all-MiniLM-L6-v2", emb.Model())
	assert.Equal(t, 384, emb.Dimensions())
}

func TestCreateEmbedder_OpenAI(t *testing.T) {
	config := EmbedderConfig{
		Provider: "openai",
		Model:    "text-embedding-3-small",
		APIKey:   "test-key",
	}

	emb, err := CreateEmbedder(config)
	// OpenAI embedder is not yet implemented
	assert.Error(t, err)
	assert.Nil(t, emb)
	assert.Contains(t, err.Error(), "not yet implemented")
}

func TestCreateEmbedder_InvalidProvider(t *testing.T) {
	config := EmbedderConfig{
		Provider: "invalid-provider",
	}

	emb, err := CreateEmbedder(config)
	assert.Error(t, err)
	assert.Nil(t, emb)
	assert.Contains(t, err.Error(), "unknown embedder provider")
}

func TestValidateEmbedderConfig_Native(t *testing.T) {
	config := EmbedderConfig{
		Provider: "native",
	}

	err := ValidateEmbedderConfig(config)
	assert.NoError(t, err, "native embedder config should be valid")
}

func TestValidateEmbedderConfig_OpenAI_Valid(t *testing.T) {
	config := EmbedderConfig{
		Provider: "openai",
		Model:    "text-embedding-3-small",
		APIKey:   "sk-test-key",
	}

	err := ValidateEmbedderConfig(config)
	assert.NoError(t, err, "valid OpenAI config should pass validation")
}

func TestValidateEmbedderConfig_OpenAI_MissingAPIKey(t *testing.T) {
	config := EmbedderConfig{
		Provider: "openai",
		Model:    "text-embedding-3-small",
		APIKey:   "",
	}

	err := ValidateEmbedderConfig(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "api_key")
}

func TestValidateEmbedderConfig_OpenAI_MissingModel(t *testing.T) {
	config := EmbedderConfig{
		Provider: "openai",
		Model:    "",
		APIKey:   "sk-test-key",
	}

	err := ValidateEmbedderConfig(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "model")
}

func TestValidateEmbedderConfig_EmptyProvider(t *testing.T) {
	config := EmbedderConfig{
		Provider: "",
	}

	err := ValidateEmbedderConfig(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "provider cannot be empty")
}

func TestValidateEmbedderConfig_UnknownProvider(t *testing.T) {
	config := EmbedderConfig{
		Provider: "unknown-provider",
	}

	err := ValidateEmbedderConfig(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown embedder provider")
}

func TestDefaultEmbedderConfig(t *testing.T) {
	config := DefaultEmbedderConfig()

	assert.Equal(t, "native", config.Provider, "default provider should be native")
	assert.NoError(t, ValidateEmbedderConfig(config),
		"default config should be valid")
}

func TestEmbedderType_Constants(t *testing.T) {
	// Verify embedder type constants
	assert.Equal(t, EmbedderType("native"), EmbedderTypeNative)
	assert.Equal(t, EmbedderType("openai"), EmbedderTypeOpenAI)
}
