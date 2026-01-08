package attack

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/zero-day-ai/gibson/internal/types"
)

// TargetResolver resolves target configuration from attack options.
// It handles URL parsing, validation, header parsing, and credential resolution.
type TargetResolver interface {
	// Resolve converts AttackOptions into a validated TargetConfig.
	// It performs URL validation, header parsing, and credential lookup.
	Resolve(ctx context.Context, opts *AttackOptions) (*TargetConfig, error)
}

// TargetConfig represents the resolved and validated target configuration
// for an attack execution. It contains all necessary information to
// establish a connection to the target system.
type TargetConfig struct {
	// URL is the validated target endpoint URL
	URL string

	// Type is the detected or specified target type (llm_chat, llm_api, etc.)
	Type types.TargetType

	// Provider is the detected or specified LLM provider (openai, anthropic, etc.)
	Provider string

	// Headers contains all HTTP headers to send with requests (custom + defaults)
	Headers map[string]string

	// Credential contains the resolved authentication credential (if any)
	Credential *types.Credential
}

// DefaultTargetResolver is the standard implementation of TargetResolver.
// It performs URL parsing, validation, header parsing, and basic target
// type detection based on URL patterns.
type DefaultTargetResolver struct {
	// credStore is used to resolve credential names/IDs to Credential objects
	// This can be nil if credential resolution is not needed
	credStore CredentialStore
}

// CredentialStore defines the interface for retrieving credentials.
// This allows the resolver to be independent of the specific storage implementation.
type CredentialStore interface {
	// GetByID retrieves a credential by its ID
	GetByID(ctx context.Context, id types.ID) (*types.Credential, error)

	// GetByName retrieves a credential by its name
	GetByName(ctx context.Context, name string) (*types.Credential, error)
}

// NewDefaultTargetResolver creates a new DefaultTargetResolver.
// credStore can be nil if credential resolution is not needed.
func NewDefaultTargetResolver(credStore CredentialStore) *DefaultTargetResolver {
	return &DefaultTargetResolver{
		credStore: credStore,
	}
}

// Resolve implements TargetResolver.Resolve.
// It validates the target URL, parses headers, detects target type and provider,
// and resolves credentials if specified.
func (r *DefaultTargetResolver) Resolve(ctx context.Context, opts *AttackOptions) (*TargetConfig, error) {
	if opts == nil {
		return nil, fmt.Errorf("attack options cannot be nil")
	}

	// Initialize target config
	config := &TargetConfig{
		Type:     opts.TargetType,
		Provider: opts.TargetProvider,
		Headers:  make(map[string]string),
	}

	// URL is optional for some target types (e.g., 'network' uses subnet instead)
	if strings.TrimSpace(opts.TargetURL) != "" {
		// Parse and validate the URL
		parsedURL, err := parseAndValidateURL(opts.TargetURL)
		if err != nil {
			return nil, fmt.Errorf("invalid target URL: %w", err)
		}
		config.URL = parsedURL.String()

		// Detect target type if not specified (only when URL is available)
		if config.Type == "" {
			config.Type = detectTargetType(parsedURL)
		}

		// Detect provider if not specified (only when URL is available)
		if config.Provider == "" {
			config.Provider = detectProvider(parsedURL)
		}
	}

	// Merge headers: start with provided headers, add defaults if not present
	if opts.TargetHeaders != nil {
		for k, v := range opts.TargetHeaders {
			config.Headers[k] = v
		}
	}

	// Add default headers if not already present
	addDefaultHeaders(config.Headers)

	// Resolve credential if specified
	if opts.Credential != "" {
		if r.credStore == nil {
			return nil, fmt.Errorf("credential store not configured but credential requested: %s", opts.Credential)
		}
		cred, err := r.resolveCredential(ctx, opts.Credential)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve credential: %w", err)
		}
		config.Credential = cred
	}

	return config, nil
}

// parseAndValidateURL parses a URL string and validates it for attack use.
// It ensures the URL has a valid scheme (http or https) and host.
func parseAndValidateURL(rawURL string) (*url.URL, error) {
	// Trim whitespace
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("URL cannot be empty")
	}

	// Parse the URL
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}

	// Validate scheme
	scheme := strings.ToLower(parsedURL.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, fmt.Errorf("unsupported URL scheme '%s': only http and https are allowed", parsedURL.Scheme)
	}

	// Validate host
	if parsedURL.Host == "" {
		return nil, fmt.Errorf("URL must contain a host")
	}

	return parsedURL, nil
}

// ParseHeadersJSON parses a JSON string containing HTTP headers.
// The JSON should be an object with string keys and values.
// Returns an error if the JSON is invalid or contains non-string values.
func ParseHeadersJSON(headersJSON string) (map[string]string, error) {
	headersJSON = strings.TrimSpace(headersJSON)
	if headersJSON == "" {
		return make(map[string]string), nil
	}

	// Parse JSON into a map
	var rawHeaders map[string]interface{}
	if err := json.Unmarshal([]byte(headersJSON), &rawHeaders); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	// Validate and convert all values to strings
	headers := make(map[string]string, len(rawHeaders))
	for key, value := range rawHeaders {
		// Validate header name
		if err := validateHeaderName(key); err != nil {
			return nil, err
		}

		// Convert value to string and validate
		strValue, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("header '%s' has non-string value: must be a string", key)
		}

		if err := validateHeaderValue(strValue); err != nil {
			return nil, fmt.Errorf("invalid value for header '%s': %w", key, err)
		}

		headers[key] = strValue
	}

	return headers, nil
}

// validateHeaderName validates an HTTP header name.
// Header names must be non-empty and contain only valid characters.
func validateHeaderName(name string) error {
	if name == "" {
		return fmt.Errorf("header name cannot be empty")
	}

	// HTTP header names are case-insensitive and should contain only
	// alphanumeric characters, hyphens, and underscores
	for i, r := range name {
		if !isValidHeaderNameChar(r) {
			return fmt.Errorf("header name '%s' contains invalid character at position %d", name, i)
		}
	}

	return nil
}

// validateHeaderValue validates an HTTP header value.
// Header values should not contain control characters.
func validateHeaderValue(value string) error {
	for i, r := range value {
		// Disallow control characters except tab
		if r < 32 && r != 9 {
			return fmt.Errorf("header value contains invalid control character at position %d", i)
		}
		// Disallow DEL character
		if r == 127 {
			return fmt.Errorf("header value contains invalid control character at position %d", i)
		}
	}
	return nil
}

// isValidHeaderNameChar checks if a rune is valid in an HTTP header name.
func isValidHeaderNameChar(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '-' || r == '_'
}

// addDefaultHeaders adds default HTTP headers to the provided map
// if they are not already present. This ensures proper User-Agent
// and Accept headers for Gibson attacks.
func addDefaultHeaders(headers map[string]string) {
	// Add User-Agent if not present
	if _, exists := headers["User-Agent"]; !exists {
		headers["User-Agent"] = "Gibson/1.0"
	}

	// Add Accept if not present
	if _, exists := headers["Accept"]; !exists {
		headers["Accept"] = "application/json, text/plain, */*"
	}

	// Add Content-Type if not present (for POST/PUT requests)
	if _, exists := headers["Content-Type"]; !exists {
		headers["Content-Type"] = "application/json"
	}
}

// detectTargetType attempts to detect the target type from the URL.
// This is a best-effort heuristic and may not be accurate for all cases.
func detectTargetType(parsedURL *url.URL) types.TargetType {
	path := strings.ToLower(parsedURL.Path)
	host := strings.ToLower(parsedURL.Host)

	// Check for common API patterns
	if strings.Contains(path, "/chat") || strings.Contains(path, "/completions") {
		return types.TargetTypeLLMChat
	}

	if strings.Contains(path, "/embeddings") || strings.Contains(path, "/embed") {
		return types.TargetTypeEmbedding
	}

	if strings.Contains(path, "/rag") || strings.Contains(path, "/search") {
		return types.TargetTypeRAG
	}

	if strings.Contains(path, "/agent") || strings.Contains(host, "agent") {
		return types.TargetTypeAgent
	}

	// Default to LLM API if we can't determine
	return types.TargetTypeLLMAPI
}

// detectProvider attempts to detect the LLM provider from the URL.
// This is a best-effort heuristic based on common hostnames.
func detectProvider(parsedURL *url.URL) string {
	host := strings.ToLower(parsedURL.Host)

	if strings.Contains(host, "openai.com") || strings.Contains(host, "api.openai") {
		return string(types.ProviderOpenAI)
	}

	if strings.Contains(host, "anthropic.com") || strings.Contains(host, "api.anthropic") {
		return string(types.ProviderAnthropic)
	}

	if strings.Contains(host, "google.com") || strings.Contains(host, "googleapis.com") {
		return string(types.ProviderGoogle)
	}

	if strings.Contains(host, "azure.com") || strings.Contains(host, "openai.azure") {
		return string(types.ProviderAzure)
	}

	if strings.Contains(host, "ollama") {
		return string(types.ProviderOllama)
	}

	// Return custom for unknown providers
	return string(types.ProviderCustom)
}

// resolveCredential resolves a credential identifier to a Credential object.
// It attempts to parse the identifier as an ID first, then falls back to name lookup.
func (r *DefaultTargetResolver) resolveCredential(ctx context.Context, identifier string) (*types.Credential, error) {
	if r.credStore == nil {
		return nil, fmt.Errorf("credential store not configured")
	}

	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return nil, fmt.Errorf("credential identifier cannot be empty")
	}

	// Try to parse as ID
	id := types.ID(identifier)
	if err := id.Validate(); err == nil {
		// Valid ID, try to retrieve
		cred, err := r.credStore.GetByID(ctx, id)
		if err == nil {
			return cred, nil
		}
		// If not found as ID, fall through to try as name
	}

	// Try to retrieve by name
	cred, err := r.credStore.GetByName(ctx, identifier)
	if err != nil {
		return nil, fmt.Errorf("credential not found: %s", identifier)
	}

	return cred, nil
}
