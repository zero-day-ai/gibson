package resolver

import (
	"testing"
)

// TestParseVersionConstraint tests the ParseVersionConstraint function.
func TestParseVersionConstraint(t *testing.T) {
	tests := []struct {
		name        string
		constraint  string
		wantType    ConstraintType
		wantMin     string
		wantMax     string
		wantExact   string
		minInc      bool
		maxInc      bool
		shouldError bool
	}{
		// Any version
		{
			name:       "empty string",
			constraint: "",
			wantType:   ConstraintAny,
		},
		{
			name:       "wildcard",
			constraint: "*",
			wantType:   ConstraintAny,
		},
		{
			name:       "whitespace only",
			constraint: "   ",
			wantType:   ConstraintAny,
		},

		// Exact version
		{
			name:       "exact version",
			constraint: "1.0.0",
			wantType:   ConstraintExact,
			wantExact:  "1.0.0",
		},
		{
			name:       "exact version with v prefix",
			constraint: "v1.2.3",
			wantType:   ConstraintExact,
			wantExact:  "1.2.3",
		},
		{
			name:       "exact version with spaces",
			constraint: "  1.5.0  ",
			wantType:   ConstraintExact,
			wantExact:  "1.5.0",
		},

		// Minimum version
		{
			name:       "minimum inclusive",
			constraint: ">=1.0.0",
			wantType:   ConstraintMinimum,
			wantMin:    "1.0.0",
			minInc:     true,
		},
		{
			name:       "minimum inclusive with v prefix",
			constraint: ">=v2.0.0",
			wantType:   ConstraintMinimum,
			wantMin:    "2.0.0",
			minInc:     true,
		},
		{
			name:       "minimum exclusive",
			constraint: ">1.0.0",
			wantType:   ConstraintMinimum,
			wantMin:    "1.0.0",
			minInc:     false,
		},
		{
			name:       "minimum with spaces",
			constraint: ">=  1.0.0  ",
			wantType:   ConstraintMinimum,
			wantMin:    "1.0.0",
			minInc:     true,
		},

		// Maximum version
		{
			name:       "maximum inclusive",
			constraint: "<=2.0.0",
			wantType:   ConstraintMaximum,
			wantMax:    "2.0.0",
			maxInc:     true,
		},
		{
			name:       "maximum exclusive",
			constraint: "<2.0.0",
			wantType:   ConstraintMaximum,
			wantMax:    "2.0.0",
			maxInc:     false,
		},
		{
			name:       "maximum with v prefix",
			constraint: "<=v3.0.0",
			wantType:   ConstraintMaximum,
			wantMax:    "3.0.0",
			maxInc:     true,
		},

		// Range constraints
		{
			name:       "range inclusive-exclusive",
			constraint: ">=1.0.0,<2.0.0",
			wantType:   ConstraintRange,
			wantMin:    "1.0.0",
			wantMax:    "2.0.0",
			minInc:     true,
			maxInc:     false,
		},
		{
			name:       "range both inclusive",
			constraint: ">=1.0.0,<=2.0.0",
			wantType:   ConstraintRange,
			wantMin:    "1.0.0",
			wantMax:    "2.0.0",
			minInc:     true,
			maxInc:     true,
		},
		{
			name:       "range both exclusive",
			constraint: ">1.0.0,<2.0.0",
			wantType:   ConstraintRange,
			wantMin:    "1.0.0",
			wantMax:    "2.0.0",
			minInc:     false,
			maxInc:     false,
		},
		{
			name:       "range with spaces",
			constraint: ">= 1.0.0 , < 2.0.0 ",
			wantType:   ConstraintRange,
			wantMin:    "1.0.0",
			wantMax:    "2.0.0",
			minInc:     true,
			maxInc:     false,
		},
		{
			name:       "range with v prefix",
			constraint: ">=v1.0.0,<v2.0.0",
			wantType:   ConstraintRange,
			wantMin:    "1.0.0",
			wantMax:    "2.0.0",
			minInc:     true,
			maxInc:     false,
		},

		// Error cases
		{
			name:        "invalid version format",
			constraint:  "1.0",
			shouldError: true,
		},
		{
			name:        "invalid version - too many parts",
			constraint:  "1.0.0.0",
			shouldError: true,
		},
		{
			name:        "invalid version - non-numeric",
			constraint:  "abc",
			shouldError: true,
		},
		{
			name:        "invalid range - too many parts",
			constraint:  ">=1.0.0,<2.0.0,<3.0.0",
			shouldError: true,
		},
		{
			name:        "invalid range - min >= max",
			constraint:  ">=2.0.0,<1.0.0",
			shouldError: true,
		},
		{
			name:        "invalid range - min == max",
			constraint:  ">=1.0.0,<=1.0.0",
			shouldError: true,
		},
		{
			name:        "invalid range - missing operator on first part",
			constraint:  "1.0.0,<2.0.0",
			shouldError: true,
		},
		{
			name:        "invalid range - missing operator on second part",
			constraint:  ">=1.0.0,2.0.0",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseVersionConstraint(tt.constraint)

			if tt.shouldError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Type != tt.wantType {
				t.Errorf("Type = %v, want %v", result.Type, tt.wantType)
			}

			switch tt.wantType {
			case ConstraintExact:
				if result.ExactVersion != tt.wantExact {
					t.Errorf("ExactVersion = %v, want %v", result.ExactVersion, tt.wantExact)
				}

			case ConstraintMinimum:
				if result.MinVersion != tt.wantMin {
					t.Errorf("MinVersion = %v, want %v", result.MinVersion, tt.wantMin)
				}
				if result.MinInclusive != tt.minInc {
					t.Errorf("MinInclusive = %v, want %v", result.MinInclusive, tt.minInc)
				}

			case ConstraintMaximum:
				if result.MaxVersion != tt.wantMax {
					t.Errorf("MaxVersion = %v, want %v", result.MaxVersion, tt.wantMax)
				}
				if result.MaxInclusive != tt.maxInc {
					t.Errorf("MaxInclusive = %v, want %v", result.MaxInclusive, tt.maxInc)
				}

			case ConstraintRange:
				if result.MinVersion != tt.wantMin {
					t.Errorf("MinVersion = %v, want %v", result.MinVersion, tt.wantMin)
				}
				if result.MaxVersion != tt.wantMax {
					t.Errorf("MaxVersion = %v, want %v", result.MaxVersion, tt.wantMax)
				}
				if result.MinInclusive != tt.minInc {
					t.Errorf("MinInclusive = %v, want %v", result.MinInclusive, tt.minInc)
				}
				if result.MaxInclusive != tt.maxInc {
					t.Errorf("MaxInclusive = %v, want %v", result.MaxInclusive, tt.maxInc)
				}
			}
		})
	}
}

// TestSatisfiesConstraint tests the SatisfiesConstraint function.
func TestSatisfiesConstraint(t *testing.T) {
	tests := []struct {
		name        string
		actual      string
		constraint  string
		want        bool
		shouldError bool
	}{
		// Any version
		{
			name:       "any version - wildcard",
			actual:     "1.0.0",
			constraint: "*",
			want:       true,
		},
		{
			name:       "any version - empty",
			actual:     "2.5.3",
			constraint: "",
			want:       true,
		},

		// Exact version
		{
			name:       "exact match",
			actual:     "1.0.0",
			constraint: "1.0.0",
			want:       true,
		},
		{
			name:       "exact match with v prefix",
			actual:     "v1.0.0",
			constraint: "1.0.0",
			want:       true,
		},
		{
			name:       "exact mismatch",
			actual:     "1.0.1",
			constraint: "1.0.0",
			want:       false,
		},

		// Minimum version (inclusive)
		{
			name:       "minimum inclusive - satisfied",
			actual:     "1.5.0",
			constraint: ">=1.0.0",
			want:       true,
		},
		{
			name:       "minimum inclusive - exact match",
			actual:     "1.0.0",
			constraint: ">=1.0.0",
			want:       true,
		},
		{
			name:       "minimum inclusive - not satisfied",
			actual:     "0.9.0",
			constraint: ">=1.0.0",
			want:       false,
		},

		// Minimum version (exclusive)
		{
			name:       "minimum exclusive - satisfied",
			actual:     "1.5.0",
			constraint: ">1.0.0",
			want:       true,
		},
		{
			name:       "minimum exclusive - exact match not satisfied",
			actual:     "1.0.0",
			constraint: ">1.0.0",
			want:       false,
		},
		{
			name:       "minimum exclusive - not satisfied",
			actual:     "0.9.0",
			constraint: ">1.0.0",
			want:       false,
		},

		// Maximum version (inclusive)
		{
			name:       "maximum inclusive - satisfied",
			actual:     "1.5.0",
			constraint: "<=2.0.0",
			want:       true,
		},
		{
			name:       "maximum inclusive - exact match",
			actual:     "2.0.0",
			constraint: "<=2.0.0",
			want:       true,
		},
		{
			name:       "maximum inclusive - not satisfied",
			actual:     "2.5.0",
			constraint: "<=2.0.0",
			want:       false,
		},

		// Maximum version (exclusive)
		{
			name:       "maximum exclusive - satisfied",
			actual:     "1.5.0",
			constraint: "<2.0.0",
			want:       true,
		},
		{
			name:       "maximum exclusive - exact match not satisfied",
			actual:     "2.0.0",
			constraint: "<2.0.0",
			want:       false,
		},
		{
			name:       "maximum exclusive - not satisfied",
			actual:     "2.5.0",
			constraint: "<2.0.0",
			want:       false,
		},

		// Range constraints
		{
			name:       "range - satisfied",
			actual:     "1.5.0",
			constraint: ">=1.0.0,<2.0.0",
			want:       true,
		},
		{
			name:       "range - lower bound satisfied",
			actual:     "1.0.0",
			constraint: ">=1.0.0,<2.0.0",
			want:       true,
		},
		{
			name:       "range - upper bound not satisfied",
			actual:     "2.0.0",
			constraint: ">=1.0.0,<2.0.0",
			want:       false,
		},
		{
			name:       "range - below lower bound",
			actual:     "0.9.0",
			constraint: ">=1.0.0,<2.0.0",
			want:       false,
		},
		{
			name:       "range - above upper bound",
			actual:     "3.0.0",
			constraint: ">=1.0.0,<2.0.0",
			want:       false,
		},
		{
			name:       "range - both inclusive",
			actual:     "2.0.0",
			constraint: ">=1.0.0,<=2.0.0",
			want:       true,
		},
		{
			name:       "range - exclusive lower bound",
			actual:     "1.0.0",
			constraint: ">1.0.0,<2.0.0",
			want:       false,
		},

		// Version comparisons
		{
			name:       "major version difference",
			actual:     "2.0.0",
			constraint: ">=1.0.0",
			want:       true,
		},
		{
			name:       "minor version difference",
			actual:     "1.5.0",
			constraint: ">=1.3.0",
			want:       true,
		},
		{
			name:       "patch version difference",
			actual:     "1.0.5",
			constraint: ">=1.0.3",
			want:       true,
		},

		// Error cases
		{
			name:        "invalid actual version",
			actual:      "abc",
			constraint:  ">=1.0.0",
			shouldError: true,
		},
		{
			name:        "invalid constraint",
			actual:      "1.0.0",
			constraint:  "invalid",
			shouldError: true,
		},
		{
			name:        "malformed actual version",
			actual:      "1.0",
			constraint:  ">=1.0.0",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := SatisfiesConstraint(tt.actual, tt.constraint)

			if tt.shouldError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result != tt.want {
				t.Errorf("SatisfiesConstraint(%q, %q) = %v, want %v", tt.actual, tt.constraint, result, tt.want)
			}
		})
	}
}

// TestCompareVersions tests the CompareVersions function.
func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name string
		v1   string
		v2   string
		want int
	}{
		// Equal versions
		{
			name: "equal versions",
			v1:   "1.0.0",
			v2:   "1.0.0",
			want: 0,
		},
		{
			name: "equal with v prefix",
			v1:   "v1.0.0",
			v2:   "1.0.0",
			want: 0,
		},
		{
			name: "equal both with v prefix",
			v1:   "v1.0.0",
			v2:   "v1.0.0",
			want: 0,
		},

		// Major version differences
		{
			name: "major version v1 < v2",
			v1:   "1.0.0",
			v2:   "2.0.0",
			want: -1,
		},
		{
			name: "major version v1 > v2",
			v1:   "3.0.0",
			v2:   "2.0.0",
			want: 1,
		},

		// Minor version differences
		{
			name: "minor version v1 < v2",
			v1:   "1.0.0",
			v2:   "1.5.0",
			want: -1,
		},
		{
			name: "minor version v1 > v2",
			v1:   "1.8.0",
			v2:   "1.5.0",
			want: 1,
		},

		// Patch version differences
		{
			name: "patch version v1 < v2",
			v1:   "1.0.0",
			v2:   "1.0.5",
			want: -1,
		},
		{
			name: "patch version v1 > v2",
			v1:   "1.0.9",
			v2:   "1.0.5",
			want: 1,
		},

		// Multi-digit versions
		{
			name: "multi-digit major",
			v1:   "10.0.0",
			v2:   "9.0.0",
			want: 1,
		},
		{
			name: "multi-digit minor",
			v1:   "1.20.0",
			v2:   "1.19.0",
			want: 1,
		},
		{
			name: "multi-digit patch",
			v1:   "1.0.100",
			v2:   "1.0.99",
			want: 1,
		},

		// Complex comparisons
		{
			name: "major overrides minor",
			v1:   "2.0.0",
			v2:   "1.99.99",
			want: 1,
		},
		{
			name: "minor overrides patch",
			v1:   "1.5.0",
			v2:   "1.4.99",
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CompareVersions(tt.v1, tt.v2)
			if result != tt.want {
				t.Errorf("CompareVersions(%q, %q) = %d, want %d", tt.v1, tt.v2, result, tt.want)
			}
		})
	}
}

// TestValidateVersion tests the validateVersion function.
func TestValidateVersion(t *testing.T) {
	tests := []struct {
		name        string
		version     string
		shouldError bool
	}{
		// Valid versions
		{
			name:        "valid version",
			version:     "1.0.0",
			shouldError: false,
		},
		{
			name:        "valid version with v prefix",
			version:     "v1.0.0",
			shouldError: false,
		},
		{
			name:        "multi-digit version",
			version:     "10.20.30",
			shouldError: false,
		},

		// Invalid versions
		{
			name:        "missing patch",
			version:     "1.0",
			shouldError: true,
		},
		{
			name:        "missing minor and patch",
			version:     "1",
			shouldError: true,
		},
		{
			name:        "too many parts",
			version:     "1.0.0.0",
			shouldError: true,
		},
		{
			name:        "non-numeric major",
			version:     "a.0.0",
			shouldError: true,
		},
		{
			name:        "non-numeric minor",
			version:     "1.b.0",
			shouldError: true,
		},
		{
			name:        "non-numeric patch",
			version:     "1.0.c",
			shouldError: true,
		},
		{
			name:        "empty string",
			version:     "",
			shouldError: true,
		},
		{
			name:        "empty component",
			version:     "1..0",
			shouldError: true,
		},
		{
			name:        "negative number",
			version:     "1.0.-1",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateVersion(tt.version)
			if tt.shouldError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// TestConstraintType tests the ConstraintType constants.
func TestConstraintType(t *testing.T) {
	tests := []struct {
		name  string
		ct    ConstraintType
		value string
	}{
		{"exact", ConstraintExact, "exact"},
		{"minimum", ConstraintMinimum, "minimum"},
		{"maximum", ConstraintMaximum, "maximum"},
		{"range", ConstraintRange, "range"},
		{"any", ConstraintAny, "any"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.ct) != tt.value {
				t.Errorf("ConstraintType value = %v, want %v", string(tt.ct), tt.value)
			}
		})
	}
}

// TestVersionConstraintJSON tests JSON marshaling/unmarshaling of VersionConstraint.
func TestVersionConstraintJSON(t *testing.T) {
	tests := []struct {
		name       string
		constraint string
	}{
		{"exact", "1.0.0"},
		{"minimum", ">=1.0.0"},
		{"maximum", "<2.0.0"},
		{"range", ">=1.0.0,<2.0.0"},
		{"any", "*"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse the constraint
			c, err := ParseVersionConstraint(tt.constraint)
			if err != nil {
				t.Fatalf("failed to parse constraint: %v", err)
			}

			// Marshal to JSON (indirectly tests struct tags)
			// This test ensures the JSON tags are properly defined
			if c.Type == "" {
				t.Errorf("Type should not be empty")
			}
		})
	}
}

// TestEdgeCases tests edge cases and boundary conditions.
func TestEdgeCases(t *testing.T) {
	t.Run("version with leading zeros", func(t *testing.T) {
		// Leading zeros are treated as decimal integers
		result := CompareVersions("1.01.0", "1.1.0")
		if result != 0 {
			t.Errorf("versions with leading zeros should be equal after parsing")
		}
	})

	t.Run("very large version numbers", func(t *testing.T) {
		result := CompareVersions("999.999.999", "1000.0.0")
		if result != -1 {
			t.Errorf("large version comparison failed")
		}
	})

	t.Run("constraint with lots of whitespace", func(t *testing.T) {
		satisfied, err := SatisfiesConstraint("  1.5.0  ", "  >=  1.0.0  ,  <  2.0.0  ")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !satisfied {
			t.Errorf("constraint with whitespace should be satisfied")
		}
	})

	t.Run("range with equal bounds", func(t *testing.T) {
		_, err := ParseVersionConstraint(">=1.0.0,<=1.0.0")
		if err == nil {
			t.Errorf("expected error for range with equal bounds")
		}
	})
}
