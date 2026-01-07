package util

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandPath(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home dir: %v", err)
	}

	// Set a test environment variable
	os.Setenv("TEST_VAR", "/test/path")
	defer os.Unsetenv("TEST_VAR")

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "empty path",
			input: "",
			want:  "",
		},
		{
			name:  "tilde only",
			input: "~",
			want:  homeDir,
		},
		{
			name:  "tilde with path",
			input: "~/data",
			want:  filepath.Join(homeDir, "data"),
		},
		{
			name:  "tilde with nested path",
			input: "~/.gibson/config.yaml",
			want:  filepath.Join(homeDir, ".gibson", "config.yaml"),
		},
		{
			name:  "absolute path unchanged",
			input: "/absolute/path",
			want:  "/absolute/path",
		},
		{
			name:  "relative path cleaned",
			input: "relative/./path",
			want:  "relative/path",
		},
		{
			name:  "env var $VAR",
			input: "$TEST_VAR/data",
			want:  "/test/path/data",
		},
		{
			name:  "env var ${VAR}",
			input: "${TEST_VAR}/data",
			want:  "/test/path/data",
		},
		{
			name:  "$HOME expansion",
			input: "$HOME/data",
			want:  filepath.Join(homeDir, "data"),
		},
		{
			name:  "${HOME} expansion",
			input: "${HOME}/data",
			want:  filepath.Join(homeDir, "data"),
		},
		{
			name:  "mixed tilde and env var",
			input: "~/data/$TEST_VAR",
			want:  filepath.Join(homeDir, "data", "/test/path"),
		},
		{
			name:  "path with dot-dot",
			input: "/a/b/../c",
			want:  "/a/c",
		},
		{
			name:  "undefined env var",
			input: "$UNDEFINED_VAR/path",
			want:  "/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExpandPath(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExpandPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ExpandPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExpandPath_TildeNotAtStart(t *testing.T) {
	// Tilde that's not at the start should not be expanded
	result, err := ExpandPath("/path/to/~")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "~") {
		t.Errorf("expected tilde to remain in path when not at start, got: %s", result)
	}
}

func TestMustExpandPath(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home dir: %v", err)
	}

	// Test successful expansion
	result := MustExpandPath("~/data")
	expected := filepath.Join(homeDir, "data")
	if result != expected {
		t.Errorf("MustExpandPath() = %v, want %v", result, expected)
	}

	// Test with absolute path
	result = MustExpandPath("/absolute/path")
	if result != "/absolute/path" {
		t.Errorf("MustExpandPath() = %v, want /absolute/path", result)
	}
}

func TestExpandPath_CleansDuplicateSlashes(t *testing.T) {
	result, err := ExpandPath("/path//to///file")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "/path/to/file"
	if result != expected {
		t.Errorf("ExpandPath() = %v, want %v", result, expected)
	}
}

func TestExpandPath_CleansTrailingSlash(t *testing.T) {
	result, err := ExpandPath("/path/to/dir/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "/path/to/dir"
	if result != expected {
		t.Errorf("ExpandPath() = %v, want %v", result, expected)
	}
}

func TestExpandPath_HandlesComplexEnvVars(t *testing.T) {
	// Set up test environment variables
	os.Setenv("PREFIX", "/usr")
	os.Setenv("SUFFIX", "local/bin")
	defer func() {
		os.Unsetenv("PREFIX")
		os.Unsetenv("SUFFIX")
	}()

	result, err := ExpandPath("$PREFIX/$SUFFIX/tool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "/usr/local/bin/tool"
	if result != expected {
		t.Errorf("ExpandPath() = %v, want %v", result, expected)
	}
}
