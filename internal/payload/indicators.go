package payload

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// IndicatorMatcher evaluates success indicators against responses
type IndicatorMatcher interface {
	// Match evaluates all indicators against a response and returns overall success
	Match(response string, indicators []SuccessIndicator) (bool, float64, []string, error)

	// MatchRegex matches a regex pattern against response
	MatchRegex(response string, pattern string, negate bool) (bool, error)

	// MatchContains checks if response contains a substring
	MatchContains(response string, substring string, negate bool) bool

	// MatchNotContains checks if response does not contain a substring (convenience method)
	MatchNotContains(response string, substring string) bool

	// MatchLength checks if response length meets criteria
	MatchLength(response string, criteria string) (bool, error)
}

// MatchResult represents the result of matching indicators
type MatchResult struct {
	Success           bool                   // Overall success
	ConfidenceScore   float64                // 0.0 - 1.0
	MatchedIndicators []string               // Names/descriptions of matched indicators
	Details           map[string]interface{} // Additional match details
}

// indicatorMatcher implements IndicatorMatcher
type indicatorMatcher struct {
	regexCache map[string]*regexp.Regexp // Cache compiled regex patterns
}

// NewIndicatorMatcher creates a new indicator matcher
func NewIndicatorMatcher() IndicatorMatcher {
	return &indicatorMatcher{
		regexCache: make(map[string]*regexp.Regexp),
	}
}

// Match evaluates all indicators and returns overall success with confidence score
func (m *indicatorMatcher) Match(response string, indicators []SuccessIndicator) (bool, float64, []string, error) {
	if len(indicators) == 0 {
		return false, 0.0, nil, fmt.Errorf("no indicators provided")
	}

	var matchedIndicators []string
	var totalWeight float64
	var matchedWeight float64

	// Evaluate each indicator
	for i, indicator := range indicators {
		// Default weight if not specified
		weight := indicator.Weight
		if weight == 0 {
			weight = 1.0
		}
		totalWeight += weight

		matched, err := m.matchSingleIndicator(response, indicator)
		if err != nil {
			return false, 0.0, nil, fmt.Errorf("indicator %d failed: %w", i, err)
		}

		if matched {
			matchedWeight += weight
			// Add to matched indicators list
			desc := indicator.Description
			if desc == "" {
				desc = fmt.Sprintf("%s: %s", indicator.Type, indicator.Value)
			}
			matchedIndicators = append(matchedIndicators, desc)
		}
	}

	// Calculate confidence score (weighted)
	confidenceScore := 0.0
	if totalWeight > 0 {
		confidenceScore = matchedWeight / totalWeight
	}

	// Determine overall success
	// Success if at least one indicator matched (confidence > 0)
	// Or if all indicators are negative (negate=true) and none matched
	success := confidenceScore > 0

	return success, confidenceScore, matchedIndicators, nil
}

// matchSingleIndicator evaluates a single indicator
func (m *indicatorMatcher) matchSingleIndicator(response string, indicator SuccessIndicator) (bool, error) {
	var matched bool
	var err error

	switch indicator.Type {
	case IndicatorRegex:
		matched, err = m.MatchRegex(response, indicator.Value, indicator.Negate)

	case IndicatorContains:
		matched = m.MatchContains(response, indicator.Value, indicator.Negate)

	case IndicatorNotContains:
		matched = m.MatchNotContains(response, indicator.Value)

	case IndicatorLength:
		matched, err = m.MatchLength(response, indicator.Value)

	case IndicatorStatus:
		// Status indicator would require additional context (HTTP status code)
		// For now, we'll return an error as this needs extra data
		return false, fmt.Errorf("status indicator requires HTTP status code (not yet supported)")

	case IndicatorJSON:
		// JSON path evaluation would require a JSON path library
		// For now, we'll attempt basic JSON validation
		matched, err = m.matchJSON(response, indicator.Value)

	case IndicatorSemantic:
		// Semantic matching would require an LLM or embedding model
		return false, fmt.Errorf("semantic indicator not yet supported")

	default:
		return false, fmt.Errorf("unknown indicator type: %s", indicator.Type)
	}

	if err != nil {
		return false, err
	}

	return matched, nil
}

// MatchRegex matches a regex pattern against response
func (m *indicatorMatcher) MatchRegex(response string, pattern string, negate bool) (bool, error) {
	// Check cache first
	regex, exists := m.regexCache[pattern]
	if !exists {
		// Compile and cache regex
		var err error
		regex, err = regexp.Compile(pattern)
		if err != nil {
			return false, fmt.Errorf("invalid regex pattern: %w", err)
		}
		m.regexCache[pattern] = regex
	}

	matched := regex.MatchString(response)

	// Apply negation if specified
	if negate {
		matched = !matched
	}

	return matched, nil
}

// MatchContains checks if response contains a substring
func (m *indicatorMatcher) MatchContains(response string, substring string, negate bool) bool {
	matched := strings.Contains(response, substring)

	// Apply negation if specified
	if negate {
		matched = !matched
	}

	return matched
}

// MatchNotContains checks if response does not contain a substring
func (m *indicatorMatcher) MatchNotContains(response string, substring string) bool {
	return !strings.Contains(response, substring)
}

// MatchLength checks if response length meets criteria
// Criteria format: ">100", "<50", ">=100", "<=50", "==100", "100-200"
func (m *indicatorMatcher) MatchLength(response string, criteria string) (bool, error) {
	length := len(response)

	// Check for range format (e.g., "100-200")
	if strings.Contains(criteria, "-") {
		parts := strings.Split(criteria, "-")
		if len(parts) != 2 {
			return false, fmt.Errorf("invalid range format: %s", criteria)
		}

		min, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return false, fmt.Errorf("invalid minimum value: %w", err)
		}

		max, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return false, fmt.Errorf("invalid maximum value: %w", err)
		}

		return length >= min && length <= max, nil
	}

	// Check for comparison operators
	if strings.HasPrefix(criteria, ">=") {
		value, err := strconv.Atoi(strings.TrimSpace(criteria[2:]))
		if err != nil {
			return false, fmt.Errorf("invalid value: %w", err)
		}
		return length >= value, nil
	}

	if strings.HasPrefix(criteria, "<=") {
		value, err := strconv.Atoi(strings.TrimSpace(criteria[2:]))
		if err != nil {
			return false, fmt.Errorf("invalid value: %w", err)
		}
		return length <= value, nil
	}

	if strings.HasPrefix(criteria, ">") {
		value, err := strconv.Atoi(strings.TrimSpace(criteria[1:]))
		if err != nil {
			return false, fmt.Errorf("invalid value: %w", err)
		}
		return length > value, nil
	}

	if strings.HasPrefix(criteria, "<") {
		value, err := strconv.Atoi(strings.TrimSpace(criteria[1:]))
		if err != nil {
			return false, fmt.Errorf("invalid value: %w", err)
		}
		return length < value, nil
	}

	if strings.HasPrefix(criteria, "==") {
		value, err := strconv.Atoi(strings.TrimSpace(criteria[2:]))
		if err != nil {
			return false, fmt.Errorf("invalid value: %w", err)
		}
		return length == value, nil
	}

	// Try direct numeric comparison (assumed ==)
	value, err := strconv.Atoi(strings.TrimSpace(criteria))
	if err != nil {
		return false, fmt.Errorf("invalid criteria format: %s", criteria)
	}
	return length == value, nil
}

// matchJSON attempts to match JSON content
// For basic implementation, we'll check if the response is valid JSON
// and optionally contains a specific key/value
func (m *indicatorMatcher) matchJSON(response string, criteria string) (bool, error) {
	// First, check if response is valid JSON
	var data interface{}
	if err := json.Unmarshal([]byte(response), &data); err != nil {
		return false, nil // Not valid JSON, no match
	}

	// If criteria is empty, just check if it's valid JSON
	if criteria == "" {
		return true, nil
	}

	// For more complex JSON path matching, we'd need a library like gjson
	// For now, do a simple contains check on the criteria
	return strings.Contains(response, criteria), nil
}
