package attack

import (
	"context"
	"testing"

	"github.com/zero-day-ai/gibson/internal/types"
)

// TestParseAndValidateURL tests URL parsing and validation.
func TestParseAndValidateURL(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid https URL",
			url:     "https://api.openai.com/v1/chat/completions",
			wantErr: false,
		},
		{
			name:    "valid http URL",
			url:     "http://localhost:8080/api",
			wantErr: false,
		},
		{
			name:    "valid URL with port",
			url:     "https://api.example.com:443/v1/chat",
			wantErr: false,
		},
		{
			name:    "valid URL with query params",
			url:     "https://api.example.com/chat?model=gpt-4",
			wantErr: false,
		},
		{
			name:        "empty URL",
			url:         "",
			wantErr:     true,
			errContains: "cannot be empty",
		},
		{
			name:        "whitespace only URL",
			url:         "   ",
			wantErr:     true,
			errContains: "cannot be empty",
		},
		{
			name:        "invalid scheme - ftp",
			url:         "ftp://example.com/file",
			wantErr:     true,
			errContains: "unsupported URL scheme",
		},
		{
			name:        "invalid scheme - file",
			url:         "file:///path/to/file",
			wantErr:     true,
			errContains: "unsupported URL scheme",
		},
		{
			name:        "no scheme",
			url:         "example.com/api",
			wantErr:     true,
			errContains: "unsupported URL scheme",
		},
		{
			name:        "missing host",
			url:         "https:///path",
			wantErr:     true,
			errContains: "must contain a host",
		},
		{
			name:        "invalid URL format",
			url:         "https://[invalid",
			wantErr:     true,
			errContains: "failed to parse URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsedURL, err := parseAndValidateURL(tt.url)

			if tt.wantErr {
				if err == nil {
					t.Errorf("parseAndValidateURL() expected error but got nil")
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("parseAndValidateURL() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("parseAndValidateURL() unexpected error = %v", err)
				return
			}

			if parsedURL == nil {
				t.Error("parseAndValidateURL() returned nil URL without error")
			}
		})
	}
}

// TestParseHeadersJSON tests JSON header parsing.
func TestParseHeadersJSON(t *testing.T) {
	tests := []struct {
		name        string
		json        string
		want        map[string]string
		wantErr     bool
		errContains string
	}{
		{
			name: "valid headers",
			json: `{"Authorization": "Bearer token123", "X-Custom-Header": "value"}`,
			want: map[string]string{
				"Authorization":   "Bearer token123",
				"X-Custom-Header": "value",
			},
			wantErr: false,
		},
		{
			name: "single header",
			json: `{"Content-Type": "application/json"}`,
			want: map[string]string{
				"Content-Type": "application/json",
			},
			wantErr: false,
		},
		{
			name:    "empty JSON object",
			json:    `{}`,
			want:    map[string]string{},
			wantErr: false,
		},
		{
			name:    "empty string",
			json:    "",
			want:    map[string]string{},
			wantErr: false,
		},
		{
			name:    "whitespace only",
			json:    "   ",
			want:    map[string]string{},
			wantErr: false,
		},
		{
			name:        "invalid JSON",
			json:        `{"Authorization": "Bearer"`,
			wantErr:     true,
			errContains: "invalid JSON",
		},
		{
			name:        "JSON array instead of object",
			json:        `["Authorization", "Bearer"]`,
			wantErr:     true,
			errContains: "invalid JSON",
		},
		{
			name:        "non-string value - number",
			json:        `{"X-Request-ID": 12345}`,
			wantErr:     true,
			errContains: "non-string value",
		},
		{
			name:        "non-string value - boolean",
			json:        `{"X-Enable": true}`,
			wantErr:     true,
			errContains: "non-string value",
		},
		{
			name:        "non-string value - null",
			json:        `{"X-Value": null}`,
			wantErr:     true,
			errContains: "non-string value",
		},
		{
			name:        "non-string value - nested object",
			json:        `{"X-Config": {"key": "value"}}`,
			wantErr:     true,
			errContains: "non-string value",
		},
		{
			name:        "invalid header name - empty",
			json:        `{"": "value"}`,
			wantErr:     true,
			errContains: "cannot be empty",
		},
		{
			name:        "invalid header name - special chars",
			json:        `{"X-Header@Invalid": "value"}`,
			wantErr:     true,
			errContains: "invalid character",
		},
		{
			name:        "invalid header name - space",
			json:        `{"X Header": "value"}`,
			wantErr:     true,
			errContains: "invalid character",
		},
		{
			name:        "invalid header value - control char",
			json:        `{"X-Header": "value\u0000"}`,
			wantErr:     true,
			errContains: "invalid control character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseHeadersJSON(tt.json)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseHeadersJSON() expected error but got nil")
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("ParseHeadersJSON() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("ParseHeadersJSON() unexpected error = %v", err)
				return
			}

			if len(got) != len(tt.want) {
				t.Errorf("ParseHeadersJSON() got %d headers, want %d", len(got), len(tt.want))
				return
			}

			for key, wantValue := range tt.want {
				gotValue, exists := got[key]
				if !exists {
					t.Errorf("ParseHeadersJSON() missing header %q", key)
					continue
				}
				if gotValue != wantValue {
					t.Errorf("ParseHeadersJSON() header %q = %q, want %q", key, gotValue, wantValue)
				}
			}
		})
	}
}

// TestValidateHeaderName tests header name validation.
func TestValidateHeaderName(t *testing.T) {
	tests := []struct {
		name        string
		headerName  string
		wantErr     bool
		errContains string
	}{
		{
			name:       "valid header - alphanumeric",
			headerName: "Content-Type",
			wantErr:    false,
		},
		{
			name:       "valid header - with hyphen",
			headerName: "X-Custom-Header",
			wantErr:    false,
		},
		{
			name:       "valid header - with underscore",
			headerName: "X_Custom_Header",
			wantErr:    false,
		},
		{
			name:       "valid header - uppercase",
			headerName: "AUTHORIZATION",
			wantErr:    false,
		},
		{
			name:       "valid header - lowercase",
			headerName: "authorization",
			wantErr:    false,
		},
		{
			name:       "valid header - mixed case",
			headerName: "User-Agent",
			wantErr:    false,
		},
		{
			name:        "empty header name",
			headerName:  "",
			wantErr:     true,
			errContains: "cannot be empty",
		},
		{
			name:        "header with space",
			headerName:  "User Agent",
			wantErr:     true,
			errContains: "invalid character",
		},
		{
			name:        "header with special char",
			headerName:  "X-Header@",
			wantErr:     true,
			errContains: "invalid character",
		},
		{
			name:        "header with parenthesis",
			headerName:  "X-Header(1)",
			wantErr:     true,
			errContains: "invalid character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateHeaderName(tt.headerName)

			if tt.wantErr {
				if err == nil {
					t.Errorf("validateHeaderName() expected error but got nil")
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("validateHeaderName() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("validateHeaderName() unexpected error = %v", err)
			}
		})
	}
}

// TestValidateHeaderValue tests header value validation.
func TestValidateHeaderValue(t *testing.T) {
	tests := []struct {
		name        string
		headerValue string
		wantErr     bool
		errContains string
	}{
		{
			name:        "valid value - plain text",
			headerValue: "application/json",
			wantErr:     false,
		},
		{
			name:        "valid value - bearer token",
			headerValue: "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			wantErr:     false,
		},
		{
			name:        "valid value - with spaces",
			headerValue: "text/html; charset=utf-8",
			wantErr:     false,
		},
		{
			name:        "valid value - with tab",
			headerValue: "value\twith\ttabs",
			wantErr:     false,
		},
		{
			name:        "invalid value - null char",
			headerValue: "value\x00",
			wantErr:     true,
			errContains: "invalid control character",
		},
		{
			name:        "invalid value - newline",
			headerValue: "value\n",
			wantErr:     true,
			errContains: "invalid control character",
		},
		{
			name:        "invalid value - carriage return",
			headerValue: "value\r",
			wantErr:     true,
			errContains: "invalid control character",
		},
		{
			name:        "invalid value - DEL character",
			headerValue: "value\x7F",
			wantErr:     true,
			errContains: "invalid control character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateHeaderValue(tt.headerValue)

			if tt.wantErr {
				if err == nil {
					t.Errorf("validateHeaderValue() expected error but got nil")
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("validateHeaderValue() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("validateHeaderValue() unexpected error = %v", err)
			}
		})
	}
}

// TestAddDefaultHeaders tests default header addition.
func TestAddDefaultHeaders(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		expected map[string]string
	}{
		{
			name:  "empty headers",
			input: map[string]string{},
			expected: map[string]string{
				"User-Agent":   "Gibson/1.0",
				"Accept":       "application/json, text/plain, */*",
				"Content-Type": "application/json",
			},
		},
		{
			name: "custom User-Agent preserved",
			input: map[string]string{
				"User-Agent": "CustomAgent/2.0",
			},
			expected: map[string]string{
				"User-Agent":   "CustomAgent/2.0",
				"Accept":       "application/json, text/plain, */*",
				"Content-Type": "application/json",
			},
		},
		{
			name: "custom Accept preserved",
			input: map[string]string{
				"Accept": "text/html",
			},
			expected: map[string]string{
				"User-Agent":   "Gibson/1.0",
				"Accept":       "text/html",
				"Content-Type": "application/json",
			},
		},
		{
			name: "all custom headers preserved",
			input: map[string]string{
				"User-Agent":   "CustomAgent/2.0",
				"Accept":       "text/html",
				"Content-Type": "application/xml",
			},
			expected: map[string]string{
				"User-Agent":   "CustomAgent/2.0",
				"Accept":       "text/html",
				"Content-Type": "application/xml",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := make(map[string]string)
			for k, v := range tt.input {
				headers[k] = v
			}

			addDefaultHeaders(headers)

			if len(headers) != len(tt.expected) {
				t.Errorf("addDefaultHeaders() got %d headers, want %d", len(headers), len(tt.expected))
			}

			for key, wantValue := range tt.expected {
				gotValue, exists := headers[key]
				if !exists {
					t.Errorf("addDefaultHeaders() missing header %q", key)
					continue
				}
				if gotValue != wantValue {
					t.Errorf("addDefaultHeaders() header %q = %q, want %q", key, gotValue, wantValue)
				}
			}
		})
	}
}

// TestDetectTargetType tests target type detection from URLs.
func TestDetectTargetType(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want types.TargetType
	}{
		{
			name: "chat endpoint",
			url:  "https://api.openai.com/v1/chat/completions",
			want: types.TargetTypeLLMChat,
		},
		{
			name: "completions endpoint",
			url:  "https://api.openai.com/v1/completions",
			want: types.TargetTypeLLMChat,
		},
		{
			name: "embeddings endpoint",
			url:  "https://api.openai.com/v1/embeddings",
			want: types.TargetTypeEmbedding,
		},
		{
			name: "embed endpoint",
			url:  "https://api.anthropic.com/v1/embed",
			want: types.TargetTypeEmbedding,
		},
		{
			name: "rag endpoint",
			url:  "https://api.example.com/rag/query",
			want: types.TargetTypeRAG,
		},
		{
			name: "search endpoint",
			url:  "https://api.example.com/search",
			want: types.TargetTypeRAG,
		},
		{
			name: "agent endpoint - path",
			url:  "https://api.example.com/agent/execute",
			want: types.TargetTypeAgent,
		},
		{
			name: "agent endpoint - host",
			url:  "https://agent.example.com/api",
			want: types.TargetTypeAgent,
		},
		{
			name: "unknown endpoint",
			url:  "https://api.example.com/custom",
			want: types.TargetTypeLLMAPI,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsedURL, err := parseAndValidateURL(tt.url)
			if err != nil {
				t.Fatalf("failed to parse test URL: %v", err)
			}

			got := detectTargetType(parsedURL)
			if got != tt.want {
				t.Errorf("detectTargetType() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestDetectProvider tests provider detection from URLs.
func TestDetectProvider(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "openai.com",
			url:  "https://api.openai.com/v1/chat",
			want: string(types.ProviderOpenAI),
		},
		{
			name: "anthropic.com",
			url:  "https://api.anthropic.com/v1/messages",
			want: string(types.ProviderAnthropic),
		},
		{
			name: "google.com",
			url:  "https://generativelanguage.googleapis.com/v1/models",
			want: string(types.ProviderGoogle),
		},
		{
			name: "azure openai",
			url:  "https://myresource.openai.azure.com/openai/deployments",
			want: string(types.ProviderAzure),
		},
		{
			name: "ollama",
			url:  "http://ollama.local:11434/api/generate",
			want: string(types.ProviderOllama),
		},
		{
			name: "custom provider",
			url:  "https://custom-llm.example.com/api",
			want: string(types.ProviderCustom),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsedURL, err := parseAndValidateURL(tt.url)
			if err != nil {
				t.Fatalf("failed to parse test URL: %v", err)
			}

			got := detectProvider(parsedURL)
			if got != tt.want {
				t.Errorf("detectProvider() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestDefaultTargetResolver_Resolve tests the complete resolution flow.
func TestDefaultTargetResolver_Resolve(t *testing.T) {
	tests := []struct {
		name        string
		opts        *AttackOptions
		credStore   CredentialStore
		wantErr     bool
		errContains string
		validate    func(t *testing.T, config *TargetConfig)
	}{
		{
			name:        "nil options",
			opts:        nil,
			wantErr:     true,
			errContains: "cannot be nil",
		},
		{
			name: "empty target URL",
			opts: &AttackOptions{
				TargetURL: "",
			},
			wantErr:     true,
			errContains: "required",
		},
		{
			name: "invalid URL scheme",
			opts: &AttackOptions{
				TargetURL: "ftp://example.com",
			},
			wantErr:     true,
			errContains: "unsupported URL scheme",
		},
		{
			name: "valid URL with all defaults",
			opts: &AttackOptions{
				TargetURL: "https://api.openai.com/v1/chat/completions",
			},
			wantErr: false,
			validate: func(t *testing.T, config *TargetConfig) {
				if config.URL != "https://api.openai.com/v1/chat/completions" {
					t.Errorf("URL = %v, want https://api.openai.com/v1/chat/completions", config.URL)
				}
				if config.Type != types.TargetTypeLLMChat {
					t.Errorf("Type = %v, want %v", config.Type, types.TargetTypeLLMChat)
				}
				if config.Provider != string(types.ProviderOpenAI) {
					t.Errorf("Provider = %v, want %v", config.Provider, types.ProviderOpenAI)
				}
				if config.Headers["User-Agent"] != "Gibson/1.0" {
					t.Error("Default User-Agent not set")
				}
			},
		},
		{
			name: "explicit type and provider override detection",
			opts: &AttackOptions{
				TargetURL:      "https://custom.example.com/api",
				TargetType:     types.TargetTypeRAG,
				TargetProvider: string(types.ProviderCustom),
			},
			wantErr: false,
			validate: func(t *testing.T, config *TargetConfig) {
				if config.Type != types.TargetTypeRAG {
					t.Errorf("Type = %v, want %v", config.Type, types.TargetTypeRAG)
				}
				if config.Provider != string(types.ProviderCustom) {
					t.Errorf("Provider = %v, want %v", config.Provider, types.ProviderCustom)
				}
			},
		},
		{
			name: "custom headers merged with defaults",
			opts: &AttackOptions{
				TargetURL: "https://api.example.com/chat",
				TargetHeaders: map[string]string{
					"Authorization": "Bearer token123",
					"X-Custom":      "value",
				},
			},
			wantErr: false,
			validate: func(t *testing.T, config *TargetConfig) {
				if config.Headers["Authorization"] != "Bearer token123" {
					t.Error("Custom Authorization header not preserved")
				}
				if config.Headers["X-Custom"] != "value" {
					t.Error("Custom X-Custom header not preserved")
				}
				if config.Headers["User-Agent"] != "Gibson/1.0" {
					t.Error("Default User-Agent not added")
				}
			},
		},
		{
			name: "custom User-Agent overrides default",
			opts: &AttackOptions{
				TargetURL: "https://api.example.com/chat",
				TargetHeaders: map[string]string{
					"User-Agent": "CustomAgent/2.0",
				},
			},
			wantErr: false,
			validate: func(t *testing.T, config *TargetConfig) {
				if config.Headers["User-Agent"] != "CustomAgent/2.0" {
					t.Errorf("User-Agent = %v, want CustomAgent/2.0", config.Headers["User-Agent"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := NewDefaultTargetResolver(tt.credStore)
			config, err := resolver.Resolve(context.Background(), tt.opts)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Resolve() expected error but got nil")
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("Resolve() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("Resolve() unexpected error = %v", err)
				return
			}

			if config == nil {
				t.Error("Resolve() returned nil config without error")
				return
			}

			if tt.validate != nil {
				tt.validate(t, config)
			}
		})
	}
}

// mockCredentialStore is a mock implementation of CredentialStore for testing.
type mockCredentialStore struct {
	credentials map[types.ID]*types.Credential
	byName      map[string]*types.Credential
}

func newMockCredentialStore() *mockCredentialStore {
	return &mockCredentialStore{
		credentials: make(map[types.ID]*types.Credential),
		byName:      make(map[string]*types.Credential),
	}
}

func (m *mockCredentialStore) GetByID(ctx context.Context, id types.ID) (*types.Credential, error) {
	cred, exists := m.credentials[id]
	if !exists {
		return nil, types.NewError(types.CREDENTIAL_NOT_FOUND, "credential not found")
	}
	return cred, nil
}

func (m *mockCredentialStore) GetByName(ctx context.Context, name string) (*types.Credential, error) {
	cred, exists := m.byName[name]
	if !exists {
		return nil, types.NewError(types.CREDENTIAL_NOT_FOUND, "credential not found")
	}
	return cred, nil
}

func (m *mockCredentialStore) addCredential(cred *types.Credential) {
	m.credentials[cred.ID] = cred
	m.byName[cred.Name] = cred
}

// TestDefaultTargetResolver_ResolveWithCredentials tests credential resolution.
func TestDefaultTargetResolver_ResolveWithCredentials(t *testing.T) {
	// Create mock credential
	cred := types.NewCredential("test-cred", types.CredentialTypeBearer)

	// Create mock store with the credential
	store := newMockCredentialStore()
	store.addCredential(cred)

	tests := []struct {
		name        string
		opts        *AttackOptions
		wantErr     bool
		errContains string
		validate    func(t *testing.T, config *TargetConfig)
	}{
		{
			name: "resolve credential by name",
			opts: &AttackOptions{
				TargetURL:  "https://api.example.com/chat",
				Credential: "test-cred",
			},
			wantErr: false,
			validate: func(t *testing.T, config *TargetConfig) {
				if config.Credential == nil {
					t.Error("Credential not resolved")
					return
				}
				if config.Credential.Name != "test-cred" {
					t.Errorf("Credential name = %v, want test-cred", config.Credential.Name)
				}
			},
		},
		{
			name: "resolve credential by ID",
			opts: &AttackOptions{
				TargetURL:  "https://api.example.com/chat",
				Credential: string(cred.ID),
			},
			wantErr: false,
			validate: func(t *testing.T, config *TargetConfig) {
				if config.Credential == nil {
					t.Error("Credential not resolved")
					return
				}
				if config.Credential.ID != cred.ID {
					t.Errorf("Credential ID = %v, want %v", config.Credential.ID, cred.ID)
				}
			},
		},
		{
			name: "credential not found",
			opts: &AttackOptions{
				TargetURL:  "https://api.example.com/chat",
				Credential: "nonexistent",
			},
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := NewDefaultTargetResolver(store)
			config, err := resolver.Resolve(context.Background(), tt.opts)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Resolve() expected error but got nil")
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("Resolve() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("Resolve() unexpected error = %v", err)
				return
			}

			if tt.validate != nil {
				tt.validate(t, config)
			}
		})
	}
}

// TestDefaultTargetResolver_ResolveWithoutCredentialStore tests behavior when no store is provided.
func TestDefaultTargetResolver_ResolveWithoutCredentialStore(t *testing.T) {
	resolver := NewDefaultTargetResolver(nil)

	opts := &AttackOptions{
		TargetURL:  "https://api.example.com/chat",
		Credential: "test-cred",
	}

	_, err := resolver.Resolve(context.Background(), opts)
	if err == nil {
		t.Error("Resolve() expected error when credential requested but no store available")
		return
	}

	if !contains(err.Error(), "not configured") {
		t.Errorf("Resolve() error = %v, want error containing 'not configured'", err)
	}
}

// contains is a helper function to check if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && stringContains(s, substr)))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
