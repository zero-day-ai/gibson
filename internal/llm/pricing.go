package llm

import (
	"fmt"
	"sync"

	"github.com/zero-day-ai/gibson/internal/types"
)

// TokenUsage represents the number of tokens used in a request.
type TokenUsage struct {
	InputTokens  int
	OutputTokens int
}

// ModelPricing contains pricing information for a specific model.
// Prices are specified per 1 million tokens.
type ModelPricing struct {
	InputPer1M  float64 `mapstructure:"input_per_1m" yaml:"input_per_1m" validate:"min=0"`
	OutputPer1M float64 `mapstructure:"output_per_1m" yaml:"output_per_1m" validate:"min=0"`
}

// PricingConfig manages pricing information for all providers and models.
// It maintains a hierarchical map structure: provider -> model -> pricing.
type PricingConfig struct {
	mu       sync.RWMutex
	Pricing  map[string]map[string]ModelPricing `mapstructure:"pricing" yaml:"pricing"`
}

// NewPricingConfig creates a new PricingConfig with default pricing data.
func NewPricingConfig() *PricingConfig {
	return &PricingConfig{
		Pricing: make(map[string]map[string]ModelPricing),
	}
}

// DefaultPricing returns a PricingConfig populated with known model prices
// for major LLM providers as of January 2025.
func DefaultPricing() *PricingConfig {
	config := NewPricingConfig()

	// Anthropic Claude pricing
	config.Pricing["anthropic"] = map[string]ModelPricing{
		// Claude 3 Opus - Most powerful model
		"claude-3-opus-20240229": {
			InputPer1M:  15.00,
			OutputPer1M: 75.00,
		},
		"claude-3-opus": {
			InputPer1M:  15.00,
			OutputPer1M: 75.00,
		},
		// Claude 3 Sonnet - Balanced performance/cost
		"claude-3-sonnet-20240229": {
			InputPer1M:  3.00,
			OutputPer1M: 15.00,
		},
		"claude-3-sonnet": {
			InputPer1M:  3.00,
			OutputPer1M: 15.00,
		},
		// Claude 3.5 Sonnet - Enhanced Sonnet
		"claude-3-5-sonnet-20240620": {
			InputPer1M:  3.00,
			OutputPer1M: 15.00,
		},
		"claude-3-5-sonnet": {
			InputPer1M:  3.00,
			OutputPer1M: 15.00,
		},
		// Claude 3 Haiku - Fastest and most cost-effective
		"claude-3-haiku-20240307": {
			InputPer1M:  0.25,
			OutputPer1M: 1.25,
		},
		"claude-3-haiku": {
			InputPer1M:  0.25,
			OutputPer1M: 1.25,
		},
	}

	// OpenAI pricing
	config.Pricing["openai"] = map[string]ModelPricing{
		// GPT-4 Turbo - Latest GPT-4 model
		"gpt-4-turbo": {
			InputPer1M:  10.00,
			OutputPer1M: 30.00,
		},
		"gpt-4-turbo-preview": {
			InputPer1M:  10.00,
			OutputPer1M: 30.00,
		},
		"gpt-4-0125-preview": {
			InputPer1M:  10.00,
			OutputPer1M: 30.00,
		},
		"gpt-4-1106-preview": {
			InputPer1M:  10.00,
			OutputPer1M: 30.00,
		},
		// GPT-4 - Original GPT-4 models
		"gpt-4": {
			InputPer1M:  30.00,
			OutputPer1M: 60.00,
		},
		"gpt-4-0613": {
			InputPer1M:  30.00,
			OutputPer1M: 60.00,
		},
		"gpt-4-32k": {
			InputPer1M:  60.00,
			OutputPer1M: 120.00,
		},
		// GPT-3.5 Turbo - Most cost-effective
		"gpt-3.5-turbo": {
			InputPer1M:  0.50,
			OutputPer1M: 1.50,
		},
		"gpt-3.5-turbo-0125": {
			InputPer1M:  0.50,
			OutputPer1M: 1.50,
		},
		"gpt-3.5-turbo-1106": {
			InputPer1M:  1.00,
			OutputPer1M: 2.00,
		},
		"gpt-3.5-turbo-16k": {
			InputPer1M:  3.00,
			OutputPer1M: 4.00,
		},
	}

	// Google Gemini pricing
	config.Pricing["google"] = map[string]ModelPricing{
		// Gemini 1.5 Pro
		"gemini-1.5-pro": {
			InputPer1M:  7.00,
			OutputPer1M: 21.00,
		},
		"gemini-1.5-pro-latest": {
			InputPer1M:  7.00,
			OutputPer1M: 21.00,
		},
		// Gemini 1.5 Flash - Faster and cheaper
		"gemini-1.5-flash": {
			InputPer1M:  0.35,
			OutputPer1M: 1.05,
		},
		"gemini-1.5-flash-latest": {
			InputPer1M:  0.35,
			OutputPer1M: 1.05,
		},
		// Gemini Pro (legacy)
		"gemini-pro": {
			InputPer1M:  0.50,
			OutputPer1M: 1.50,
		},
		// Gemini Pro Vision (legacy)
		"gemini-pro-vision": {
			InputPer1M:  0.50,
			OutputPer1M: 1.50,
		},
	}

	return config
}

// SetProviderPricing sets pricing for all models of a specific provider.
// This replaces any existing pricing data for the provider.
func (p *PricingConfig) SetProviderPricing(provider string, pricing map[string]ModelPricing) {
	p.mu.Lock()
	defer p.mu.Unlock()

	provider = NormalizeProviderName(provider)
	if p.Pricing == nil {
		p.Pricing = make(map[string]map[string]ModelPricing)
	}
	p.Pricing[provider] = pricing
}

// SetModelPricing sets pricing for a specific provider and model.
func (p *PricingConfig) SetModelPricing(provider, model string, pricing ModelPricing) {
	p.mu.Lock()
	defer p.mu.Unlock()

	provider = NormalizeProviderName(provider)
	model = NormalizeModelName(model)

	if p.Pricing == nil {
		p.Pricing = make(map[string]map[string]ModelPricing)
	}
	if p.Pricing[provider] == nil {
		p.Pricing[provider] = make(map[string]ModelPricing)
	}
	p.Pricing[provider][model] = pricing
}

// GetModelPricing retrieves pricing for a specific provider and model.
// Returns nil if pricing is not found.
func (p *PricingConfig) GetModelPricing(provider, model string) *ModelPricing {
	p.mu.RLock()
	defer p.mu.RUnlock()

	provider = NormalizeProviderName(provider)
	model = NormalizeModelName(model)

	if p.Pricing == nil {
		return nil
	}

	providerPricing, exists := p.Pricing[provider]
	if !exists {
		return nil
	}

	modelPricing, exists := providerPricing[model]
	if !exists {
		return nil
	}

	return &modelPricing
}

// CalculateCost calculates the cost for a given token usage with a specific provider and model.
// Returns the total cost in USD and any error encountered.
// Returns an error if pricing information is not available for the specified provider/model.
func (p *PricingConfig) CalculateCost(provider, model string, usage TokenUsage) (float64, error) {
	pricing := p.GetModelPricing(provider, model)
	if pricing == nil {
		return 0, types.NewError(
			types.CONFIG_VALIDATION_FAILED,
			fmt.Sprintf("pricing not found for provider '%s' model '%s'", provider, model),
		)
	}

	return pricing.CalculateCost(usage), nil
}

// CalculateCost calculates the total cost based on token usage.
// Cost = (InputTokens / 1,000,000 * InputPer1M) + (OutputTokens / 1,000,000 * OutputPer1M)
func (m *ModelPricing) CalculateCost(usage TokenUsage) float64 {
	inputCost := (float64(usage.InputTokens) / 1_000_000.0) * m.InputPer1M
	outputCost := (float64(usage.OutputTokens) / 1_000_000.0) * m.OutputPer1M
	return inputCost + outputCost
}

// EstimateCost estimates the cost for a given number of input and output tokens.
// This is a convenience method that creates a TokenUsage and calculates cost.
func (p *PricingConfig) EstimateCost(provider, model string, inputTokens, outputTokens int) (float64, error) {
	usage := TokenUsage{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	}
	return p.CalculateCost(provider, model, usage)
}

// GetAllProviders returns a list of all providers with pricing data.
func (p *PricingConfig) GetAllProviders() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	providers := make([]string, 0, len(p.Pricing))
	for provider := range p.Pricing {
		providers = append(providers, provider)
	}
	return providers
}

// GetProviderModels returns a list of all models for a specific provider.
func (p *PricingConfig) GetProviderModels(provider string) []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	provider = NormalizeProviderName(provider)
	providerPricing, exists := p.Pricing[provider]
	if !exists {
		return []string{}
	}

	models := make([]string, 0, len(providerPricing))
	for model := range providerPricing {
		models = append(models, model)
	}
	return models
}

// HasPricing checks if pricing data exists for a specific provider and model.
func (p *PricingConfig) HasPricing(provider, model string) bool {
	return p.GetModelPricing(provider, model) != nil
}

// MergePricing merges another PricingConfig into this one.
// Existing entries are overwritten by the new config.
func (p *PricingConfig) MergePricing(other *PricingConfig) {
	if other == nil || other.Pricing == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.Pricing == nil {
		p.Pricing = make(map[string]map[string]ModelPricing)
	}

	for provider, models := range other.Pricing {
		if p.Pricing[provider] == nil {
			p.Pricing[provider] = make(map[string]ModelPricing)
		}
		for model, pricing := range models {
			p.Pricing[provider][model] = pricing
		}
	}
}
