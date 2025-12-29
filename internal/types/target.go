package types

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// TargetType represents the type of LLM target system being tested
type TargetType string

const (
	TargetTypeLLMChat    TargetType = "llm_chat"
	TargetTypeLLMAPI     TargetType = "llm_api"
	TargetTypeRAG        TargetType = "rag"
	TargetTypeAgent      TargetType = "agent"
	TargetTypeEmbedding  TargetType = "embedding"
	TargetTypeMultimodal TargetType = "multimodal"
)

// String returns the string representation of TargetType
func (t TargetType) String() string {
	return string(t)
}

// IsValid checks if the TargetType is a valid value
func (t TargetType) IsValid() bool {
	switch t {
	case TargetTypeLLMChat, TargetTypeLLMAPI, TargetTypeRAG,
		TargetTypeAgent, TargetTypeEmbedding, TargetTypeMultimodal:
		return true
	default:
		return false
	}
}

// MarshalJSON implements json.Marshaler
func (t TargetType) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(t))
}

// UnmarshalJSON implements json.Unmarshaler
func (t *TargetType) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}

	targetType := TargetType(str)
	if !targetType.IsValid() {
		return fmt.Errorf("invalid target type: %s", str)
	}

	*t = targetType
	return nil
}

// Provider represents the LLM service provider
type Provider string

const (
	ProviderOpenAI    Provider = "openai"
	ProviderAnthropic Provider = "anthropic"
	ProviderGoogle    Provider = "google"
	ProviderAzure     Provider = "azure"
	ProviderOllama    Provider = "ollama"
	ProviderCustom    Provider = "custom"
)

// String returns the string representation of Provider
func (p Provider) String() string {
	return string(p)
}

// IsValid checks if the Provider is a valid value
func (p Provider) IsValid() bool {
	switch p {
	case ProviderOpenAI, ProviderAnthropic, ProviderGoogle,
		ProviderAzure, ProviderOllama, ProviderCustom:
		return true
	default:
		return false
	}
}

// MarshalJSON implements json.Marshaler
func (p Provider) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(p))
}

// UnmarshalJSON implements json.Unmarshaler
func (p *Provider) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}

	provider := Provider(str)
	if !provider.IsValid() {
		return fmt.Errorf("invalid provider: %s", str)
	}

	*p = provider
	return nil
}

// AuthType represents the authentication method for a target
type AuthType string

const (
	AuthTypeNone   AuthType = "none"
	AuthTypeAPIKey AuthType = "api_key"
	AuthTypeBearer AuthType = "bearer"
	AuthTypeBasic  AuthType = "basic"
	AuthTypeOAuth  AuthType = "oauth"
)

// String returns the string representation of AuthType
func (a AuthType) String() string {
	return string(a)
}

// IsValid checks if the AuthType is a valid value
func (a AuthType) IsValid() bool {
	switch a {
	case AuthTypeNone, AuthTypeAPIKey, AuthTypeBearer, AuthTypeBasic, AuthTypeOAuth:
		return true
	default:
		return false
	}
}

// MarshalJSON implements json.Marshaler
func (a AuthType) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(a))
}

// UnmarshalJSON implements json.Unmarshaler
func (a *AuthType) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}

	authType := AuthType(str)
	if !authType.IsValid() {
		return fmt.Errorf("invalid auth type: %s", str)
	}

	*a = authType
	return nil
}

// Target represents a target LLM system to be tested
type Target struct {
	ID           ID                     `json:"id"`
	Name         string                 `json:"name"`
	Type         TargetType             `json:"type"`
	Provider     Provider               `json:"provider,omitempty"`
	URL          string                 `json:"url"`
	Model        string                 `json:"model,omitempty"`
	Headers      map[string]string      `json:"headers,omitempty"`
	Config       map[string]interface{} `json:"config,omitempty"`
	Capabilities []string               `json:"capabilities,omitempty"`
	AuthType     AuthType               `json:"auth_type,omitempty"`
	CredentialID *ID                    `json:"credential_id,omitempty"` // Pointer for nullable FK
	Status       TargetStatus           `json:"status"`
	Description  string                 `json:"description,omitempty"`
	Tags         []string               `json:"tags,omitempty"`
	Timeout      int                    `json:"timeout"` // seconds
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

// NewTarget creates a new Target with default values
// name: human-readable name for the target
// url: endpoint URL for the target system
// targetType: type of LLM system (llm_chat, llm_api, rag, etc.)
func NewTarget(name, url string, targetType TargetType) *Target {
	now := time.Now()
	return &Target{
		ID:           NewID(),
		Name:         name,
		Type:         targetType,
		URL:          url,
		Status:       TargetStatusActive,
		Headers:      make(map[string]string),
		Config:       make(map[string]interface{}),
		Capabilities: []string{},
		Tags:         []string{},
		Timeout:      30, // default 30 seconds
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// Validate checks if the Target has all required fields and valid values
func (t *Target) Validate() error {
	// Validate ID
	if err := t.ID.Validate(); err != nil {
		return fmt.Errorf("invalid target ID: %w", err)
	}

	// Validate Name
	if strings.TrimSpace(t.Name) == "" {
		return fmt.Errorf("target name cannot be empty")
	}

	// Validate URL
	if strings.TrimSpace(t.URL) == "" {
		return fmt.Errorf("target URL cannot be empty")
	}

	// Validate Type
	if !t.Type.IsValid() {
		return fmt.Errorf("invalid target type: %s", t.Type)
	}

	// Validate Status
	if !t.Status.IsValid() {
		return fmt.Errorf("invalid target status: %s", t.Status)
	}

	// Validate Provider if set
	if t.Provider != "" && !t.Provider.IsValid() {
		return fmt.Errorf("invalid provider: %s", t.Provider)
	}

	// Validate AuthType if set
	if t.AuthType != "" && !t.AuthType.IsValid() {
		return fmt.Errorf("invalid auth type: %s", t.AuthType)
	}

	// Validate CredentialID if set
	if t.CredentialID != nil {
		if err := t.CredentialID.Validate(); err != nil {
			return fmt.Errorf("invalid credential ID: %w", err)
		}
	}

	// Validate Timeout
	if t.Timeout <= 0 {
		return fmt.Errorf("timeout must be greater than 0")
	}

	return nil
}

// TargetFilter represents query filters for retrieving targets
type TargetFilter struct {
	Provider *Provider
	Type     *TargetType
	Status   *TargetStatus
	Tags     []string
	Limit    int
	Offset   int
}

// NewTargetFilter creates a new TargetFilter with default values
func NewTargetFilter() *TargetFilter {
	return &TargetFilter{
		Tags:   []string{},
		Limit:  100, // default limit
		Offset: 0,
	}
}

// WithProvider sets the Provider filter
func (f *TargetFilter) WithProvider(provider Provider) *TargetFilter {
	f.Provider = &provider
	return f
}

// WithType sets the Type filter
func (f *TargetFilter) WithType(targetType TargetType) *TargetFilter {
	f.Type = &targetType
	return f
}

// WithStatus sets the Status filter
func (f *TargetFilter) WithStatus(status TargetStatus) *TargetFilter {
	f.Status = &status
	return f
}

// WithTags sets the Tags filter
func (f *TargetFilter) WithTags(tags []string) *TargetFilter {
	f.Tags = tags
	return f
}

// WithLimit sets the result limit
func (f *TargetFilter) WithLimit(limit int) *TargetFilter {
	f.Limit = limit
	return f
}

// WithOffset sets the result offset for pagination
func (f *TargetFilter) WithOffset(offset int) *TargetFilter {
	f.Offset = offset
	return f
}
