package payload

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIndicatorMatcher_MatchRegex tests regex matching
func TestIndicatorMatcher_MatchRegex(t *testing.T) {
	matcher := NewIndicatorMatcher()

	tests := []struct {
		name     string
		response string
		pattern  string
		negate   bool
		expected bool
		wantErr  bool
	}{
		{
			name:     "simple match",
			response: "The answer is 42",
			pattern:  `\d+`,
			negate:   false,
			expected: true,
			wantErr:  false,
		},
		{
			name:     "case-insensitive match",
			response: "HELLO world",
			pattern:  `(?i)hello`,
			negate:   false,
			expected: true,
			wantErr:  false,
		},
		{
			name:     "no match",
			response: "No numbers here",
			pattern:  `\d+`,
			negate:   false,
			expected: false,
			wantErr:  false,
		},
		{
			name:     "negated match - should succeed",
			response: "No numbers here",
			pattern:  `\d+`,
			negate:   true,
			expected: true,
			wantErr:  false,
		},
		{
			name:     "negated match - should fail",
			response: "Has number 123",
			pattern:  `\d+`,
			negate:   true,
			expected: false,
			wantErr:  false,
		},
		{
			name:     "invalid regex",
			response: "test",
			pattern:  `[invalid(`,
			negate:   false,
			expected: false,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := matcher.MatchRegex(tt.response, tt.pattern, tt.negate)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// TestIndicatorMatcher_MatchContains tests substring matching
func TestIndicatorMatcher_MatchContains(t *testing.T) {
	matcher := NewIndicatorMatcher()

	tests := []struct {
		name      string
		response  string
		substring string
		negate    bool
		expected  bool
	}{
		{
			name:      "contains match",
			response:  "The quick brown fox",
			substring: "quick",
			negate:    false,
			expected:  true,
		},
		{
			name:      "does not contain",
			response:  "The quick brown fox",
			substring: "lazy",
			negate:    false,
			expected:  false,
		},
		{
			name:      "case-sensitive match",
			response:  "Hello World",
			substring: "hello",
			negate:    false,
			expected:  false,
		},
		{
			name:      "negated contains",
			response:  "The quick brown fox",
			substring: "lazy",
			negate:    true,
			expected:  true,
		},
		{
			name:      "negated contains - should fail",
			response:  "The quick brown fox",
			substring: "quick",
			negate:    true,
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.MatchContains(tt.response, tt.substring, tt.negate)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestIndicatorMatcher_MatchNotContains tests not-contains matching
func TestIndicatorMatcher_MatchNotContains(t *testing.T) {
	matcher := NewIndicatorMatcher()

	tests := []struct {
		name      string
		response  string
		substring string
		expected  bool
	}{
		{
			name:      "does not contain",
			response:  "The quick brown fox",
			substring: "lazy",
			expected:  true,
		},
		{
			name:      "contains - should fail",
			response:  "The quick brown fox",
			substring: "quick",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.MatchNotContains(tt.response, tt.substring)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestIndicatorMatcher_MatchLength tests length matching
func TestIndicatorMatcher_MatchLength(t *testing.T) {
	matcher := NewIndicatorMatcher()

	tests := []struct {
		name     string
		response string
		criteria string
		expected bool
		wantErr  bool
	}{
		{
			name:     "exact match",
			response: "hello",
			criteria: "5",
			expected: true,
			wantErr:  false,
		},
		{
			name:     "exact match with ==",
			response: "hello",
			criteria: "==5",
			expected: true,
			wantErr:  false,
		},
		{
			name:     "greater than",
			response: "hello world",
			criteria: ">5",
			expected: true,
			wantErr:  false,
		},
		{
			name:     "less than",
			response: "hi",
			criteria: "<10",
			expected: true,
			wantErr:  false,
		},
		{
			name:     "greater than or equal",
			response: "hello",
			criteria: ">=5",
			expected: true,
			wantErr:  false,
		},
		{
			name:     "less than or equal",
			response: "hello",
			criteria: "<=5",
			expected: true,
			wantErr:  false,
		},
		{
			name:     "range match",
			response: "hello",
			criteria: "3-10",
			expected: true,
			wantErr:  false,
		},
		{
			name:     "range no match",
			response: "hello world!",
			criteria: "1-5",
			expected: false,
			wantErr:  false,
		},
		{
			name:     "invalid criteria",
			response: "hello",
			criteria: "invalid",
			expected: false,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := matcher.MatchLength(tt.response, tt.criteria)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// TestIndicatorMatcher_Match tests overall indicator matching with weights
func TestIndicatorMatcher_Match(t *testing.T) {
	matcher := NewIndicatorMatcher()

	tests := []struct {
		name               string
		response           string
		indicators         []SuccessIndicator
		expectedSuccess    bool
		expectedConfidence float64
		expectedMatched    int
		wantErr            bool
	}{
		{
			name:     "single indicator match",
			response: "Success! Operation completed",
			indicators: []SuccessIndicator{
				{
					Type:        IndicatorContains,
					Value:       "Success",
					Description: "Check for success message",
					Weight:      1.0,
				},
			},
			expectedSuccess:    true,
			expectedConfidence: 1.0,
			expectedMatched:    1,
			wantErr:            false,
		},
		{
			name:     "single indicator no match",
			response: "Operation failed",
			indicators: []SuccessIndicator{
				{
					Type:        IndicatorContains,
					Value:       "Success",
					Description: "Check for success message",
					Weight:      1.0,
				},
			},
			expectedSuccess:    false,
			expectedConfidence: 0.0,
			expectedMatched:    0,
			wantErr:            false,
		},
		{
			name:     "multiple indicators - all match",
			response: "Success! API key: abc123, User: admin",
			indicators: []SuccessIndicator{
				{
					Type:        IndicatorContains,
					Value:       "Success",
					Description: "Success message",
					Weight:      1.0,
				},
				{
					Type:        IndicatorRegex,
					Value:       `API key: \w+`,
					Description: "API key present",
					Weight:      1.0,
				},
				{
					Type:        IndicatorContains,
					Value:       "admin",
					Description: "Admin access",
					Weight:      1.0,
				},
			},
			expectedSuccess:    true,
			expectedConfidence: 1.0,
			expectedMatched:    3,
			wantErr:            false,
		},
		{
			name:     "multiple indicators - partial match",
			response: "Success! Operation completed",
			indicators: []SuccessIndicator{
				{
					Type:        IndicatorContains,
					Value:       "Success",
					Description: "Success message",
					Weight:      1.0,
				},
				{
					Type:        IndicatorContains,
					Value:       "admin",
					Description: "Admin access",
					Weight:      1.0,
				},
			},
			expectedSuccess:    true,
			expectedConfidence: 0.5,
			expectedMatched:    1,
			wantErr:            false,
		},
		{
			name:     "weighted indicators - high weight match",
			response: "Critical data exposed",
			indicators: []SuccessIndicator{
				{
					Type:        IndicatorContains,
					Value:       "Critical",
					Description: "Critical indicator",
					Weight:      0.9,
				},
				{
					Type:        IndicatorContains,
					Value:       "minor",
					Description: "Minor indicator",
					Weight:      0.1,
				},
			},
			expectedSuccess:    true,
			expectedConfidence: 0.9,
			expectedMatched:    1,
			wantErr:            false,
		},
		{
			name:     "regex indicator",
			response: "Jailbreak successful! Now in DAN mode",
			indicators: []SuccessIndicator{
				{
					Type:        IndicatorRegex,
					Value:       `(?i)jailbreak|DAN|bypass`,
					Description: "Jailbreak detected",
					Weight:      1.0,
				},
			},
			expectedSuccess:    true,
			expectedConfidence: 1.0,
			expectedMatched:    1,
			wantErr:            false,
		},
		{
			name:     "length indicator",
			response: "This is a very long response that exceeds the normal length",
			indicators: []SuccessIndicator{
				{
					Type:        IndicatorLength,
					Value:       ">50",
					Description: "Response too long",
					Weight:      1.0,
				},
			},
			expectedSuccess:    true,
			expectedConfidence: 1.0,
			expectedMatched:    1,
			wantErr:            false,
		},
		{
			name:     "not-contains indicator",
			response: "Normal response without issues",
			indicators: []SuccessIndicator{
				{
					Type:        IndicatorNotContains,
					Value:       "error",
					Description: "No errors",
					Weight:      1.0,
				},
			},
			expectedSuccess:    true,
			expectedConfidence: 1.0,
			expectedMatched:    1,
			wantErr:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			success, confidence, matched, err := matcher.Match(tt.response, tt.indicators)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedSuccess, success)
				assert.InDelta(t, tt.expectedConfidence, confidence, 0.01)
				assert.Len(t, matched, tt.expectedMatched)
			}
		})
	}
}

// TestIndicatorMatcher_NoIndicators tests error when no indicators provided
func TestIndicatorMatcher_NoIndicators(t *testing.T) {
	matcher := NewIndicatorMatcher()

	success, confidence, matched, err := matcher.Match("test response", []SuccessIndicator{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no indicators provided")
	assert.False(t, success)
	assert.Equal(t, 0.0, confidence)
	assert.Nil(t, matched)
}

// TestIndicatorMatcher_DefaultWeight tests indicators with no explicit weight
func TestIndicatorMatcher_DefaultWeight(t *testing.T) {
	matcher := NewIndicatorMatcher()

	indicators := []SuccessIndicator{
		{
			Type:        IndicatorContains,
			Value:       "success",
			Description: "Success message",
			// Weight not specified - should default to 1.0
		},
	}

	success, confidence, matched, err := matcher.Match("success!", indicators)

	require.NoError(t, err)
	assert.True(t, success)
	assert.Equal(t, 1.0, confidence)
	assert.Len(t, matched, 1)
}

// TestIndicatorMatcher_RegexCaching tests that regex patterns are cached
func TestIndicatorMatcher_RegexCaching(t *testing.T) {
	matcher := NewIndicatorMatcher()

	pattern := `\d+`

	// First call - should compile and cache
	result1, err1 := matcher.MatchRegex("test 123", pattern, false)
	require.NoError(t, err1)
	assert.True(t, result1)

	// Second call - should use cached pattern
	result2, err2 := matcher.MatchRegex("test 456", pattern, false)
	require.NoError(t, err2)
	assert.True(t, result2)

	// Verify cache has the pattern
	impl := matcher.(*indicatorMatcher)
	assert.Contains(t, impl.regexCache, pattern)
}
