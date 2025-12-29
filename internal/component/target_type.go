package component

import (
	"encoding/json"
	"fmt"
)

// TargetType represents the type of target being tested
type TargetType string

const (
	TargetTypeLLMChat    TargetType = "llm_chat"
	TargetTypeLLMAPI     TargetType = "llm_api"
	TargetTypeRAG        TargetType = "rag"
	TargetTypeAgent      TargetType = "agent"
	TargetTypeEmbedding  TargetType = "embedding"
	TargetTypeMultimodal TargetType = "multimodal"
	TargetTypeCustom     TargetType = "custom"
)

// String returns the string representation of the TargetType
func (t TargetType) String() string {
	return string(t)
}

// IsValid checks if the TargetType is a valid enum value
func (t TargetType) IsValid() bool {
	switch t {
	case TargetTypeLLMChat, TargetTypeLLMAPI, TargetTypeRAG, TargetTypeAgent,
		TargetTypeEmbedding, TargetTypeMultimodal, TargetTypeCustom:
		return true
	default:
		return false
	}
}

// MarshalJSON implements the json.Marshaler interface
func (t TargetType) MarshalJSON() ([]byte, error) {
	if !t.IsValid() {
		return nil, fmt.Errorf("invalid target type: %s", t)
	}
	return json.Marshal(string(t))
}

// UnmarshalJSON implements the json.Unmarshaler interface
func (t *TargetType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	parsed, err := ParseTargetType(s)
	if err != nil {
		return err
	}

	*t = parsed
	return nil
}

// AllTargetTypes returns a slice containing all valid TargetType values
func AllTargetTypes() []TargetType {
	return []TargetType{
		TargetTypeLLMChat,
		TargetTypeLLMAPI,
		TargetTypeRAG,
		TargetTypeAgent,
		TargetTypeEmbedding,
		TargetTypeMultimodal,
		TargetTypeCustom,
	}
}

// ParseTargetType parses a string into a TargetType, returning an error if invalid
func ParseTargetType(s string) (TargetType, error) {
	t := TargetType(s)
	if !t.IsValid() {
		return "", fmt.Errorf("invalid target type: %s", s)
	}
	return t, nil
}
