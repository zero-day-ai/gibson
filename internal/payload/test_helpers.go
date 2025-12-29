package payload

// Test helper functions shared across test files

// boolPtr returns a pointer to a bool value
func boolPtr(b bool) *bool {
	return &b
}

// float64Ptr returns a pointer to a float64 value
func float64Ptr(f float64) *float64 {
	return &f
}

// intPtr returns a pointer to an int value
func intPtr(i int) *int {
	return &i
}

// stringPtr returns a pointer to a string value
func stringPtr(s string) *string {
	return &s
}
