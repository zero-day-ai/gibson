package crypto

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestNewFileKeyManager(t *testing.T) {
	km := NewFileKeyManager()
	if km == nil {
		t.Fatal("NewFileKeyManager returned nil")
	}
	if km.keySize != KeySize {
		t.Errorf("expected keySize %d, got %d", KeySize, km.keySize)
	}
}

func TestFileKeyManager_GenerateKey(t *testing.T) {
	km := NewFileKeyManager()

	tests := []struct {
		name string
	}{
		{"first key"},
		{"second key"},
		{"third key"},
	}

	var keys [][]byte
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := km.GenerateKey()
			if err != nil {
				t.Fatalf("GenerateKey() error = %v", err)
			}

			// Check key size
			if len(key) != KeySize {
				t.Errorf("expected key size %d, got %d", KeySize, len(key))
			}

			// Check key is not all zeros
			allZeros := true
			for _, b := range key {
				if b != 0 {
					allZeros = false
					break
				}
			}
			if allZeros {
				t.Error("generated key is all zeros")
			}

			keys = append(keys, key)
		})
	}

	// Verify keys are unique (highly unlikely to be the same)
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if bytes.Equal(keys[i], keys[j]) {
				t.Error("generated keys are not unique")
			}
		}
	}
}

func TestFileKeyManager_SaveKey(t *testing.T) {
	km := NewFileKeyManager()
	tempDir := t.TempDir()

	tests := []struct {
		name    string
		keySize int
		path    string
		wantErr bool
	}{
		{
			name:    "valid key",
			keySize: KeySize,
			path:    filepath.Join(tempDir, "valid.key"),
			wantErr: false,
		},
		{
			name:    "valid key in subdirectory",
			keySize: KeySize,
			path:    filepath.Join(tempDir, "subdir", "nested.key"),
			wantErr: false,
		},
		{
			name:    "invalid key size - too small",
			keySize: 16,
			path:    filepath.Join(tempDir, "invalid.key"),
			wantErr: true,
		},
		{
			name:    "invalid key size - too large",
			keySize: 64,
			path:    filepath.Join(tempDir, "invalid2.key"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := make([]byte, tt.keySize)
			for i := range key {
				key[i] = byte(i)
			}

			err := km.SaveKey(key, tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("SaveKey() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify file exists
				if !km.KeyExists(tt.path) {
					t.Error("key file does not exist after save")
				}

				// Verify file permissions
				info, err := os.Stat(tt.path)
				if err != nil {
					t.Fatalf("failed to stat key file: %v", err)
				}

				if info.Mode().Perm() != KeyFilePermission {
					t.Errorf("expected permissions %o, got %o", KeyFilePermission, info.Mode().Perm())
				}

				// Verify file contents
				content, err := os.ReadFile(tt.path)
				if err != nil {
					t.Fatalf("failed to read key file: %v", err)
				}

				if !bytes.Equal(content, key) {
					t.Error("saved key does not match original key")
				}
			}
		})
	}
}

func TestFileKeyManager_LoadKey(t *testing.T) {
	km := NewFileKeyManager()
	tempDir := t.TempDir()

	// Create a valid key file
	validKey := make([]byte, KeySize)
	for i := range validKey {
		validKey[i] = byte(i)
	}
	validPath := filepath.Join(tempDir, "valid.key")
	if err := km.SaveKey(validKey, validPath); err != nil {
		t.Fatalf("failed to create valid key file: %v", err)
	}

	// Create a key file with wrong permissions
	insecurePath := filepath.Join(tempDir, "insecure.key")
	if err := os.WriteFile(insecurePath, validKey, 0644); err != nil {
		t.Fatalf("failed to create insecure key file: %v", err)
	}

	// Create a key file with wrong size
	wrongSizePath := filepath.Join(tempDir, "wrongsize.key")
	if err := os.WriteFile(wrongSizePath, []byte{1, 2, 3}, KeyFilePermission); err != nil {
		t.Fatalf("failed to create wrong size key file: %v", err)
	}

	tests := []struct {
		name    string
		path    string
		wantKey []byte
		wantErr bool
	}{
		{
			name:    "valid key file",
			path:    validPath,
			wantKey: validKey,
			wantErr: false,
		},
		{
			name:    "non-existent file",
			path:    filepath.Join(tempDir, "nonexistent.key"),
			wantKey: nil,
			wantErr: true,
		},
		{
			name:    "insecure permissions",
			path:    insecurePath,
			wantKey: nil,
			wantErr: true,
		},
		{
			name:    "wrong key size",
			path:    wrongSizePath,
			wantKey: nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := km.LoadKey(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadKey() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if !bytes.Equal(key, tt.wantKey) {
					t.Error("loaded key does not match expected key")
				}
			}
		})
	}
}

func TestFileKeyManager_KeyExists(t *testing.T) {
	km := NewFileKeyManager()
	tempDir := t.TempDir()

	existingPath := filepath.Join(tempDir, "existing.key")
	key, _ := km.GenerateKey()
	if err := km.SaveKey(key, existingPath); err != nil {
		t.Fatalf("failed to create test key file: %v", err)
	}

	tests := []struct {
		name   string
		path   string
		exists bool
	}{
		{
			name:   "existing file",
			path:   existingPath,
			exists: true,
		},
		{
			name:   "non-existent file",
			path:   filepath.Join(tempDir, "nonexistent.key"),
			exists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := km.KeyExists(tt.path); got != tt.exists {
				t.Errorf("KeyExists() = %v, want %v", got, tt.exists)
			}
		})
	}
}

func TestFileKeyManager_RoundTrip(t *testing.T) {
	km := NewFileKeyManager()
	tempDir := t.TempDir()
	keyPath := filepath.Join(tempDir, "roundtrip.key")

	// Generate a key
	originalKey, err := km.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}

	// Save the key
	if err := km.SaveKey(originalKey, keyPath); err != nil {
		t.Fatalf("SaveKey() error = %v", err)
	}

	// Load the key
	loadedKey, err := km.LoadKey(keyPath)
	if err != nil {
		t.Fatalf("LoadKey() error = %v", err)
	}

	// Verify keys match
	if !bytes.Equal(originalKey, loadedKey) {
		t.Error("loaded key does not match original key")
	}
}

func TestFileKeyManager_SaveKeyOverwrite(t *testing.T) {
	km := NewFileKeyManager()
	tempDir := t.TempDir()
	keyPath := filepath.Join(tempDir, "overwrite.key")

	// Save first key
	key1, _ := km.GenerateKey()
	if err := km.SaveKey(key1, keyPath); err != nil {
		t.Fatalf("SaveKey() first save error = %v", err)
	}

	// Save second key (overwrite)
	key2, _ := km.GenerateKey()
	if err := km.SaveKey(key2, keyPath); err != nil {
		t.Fatalf("SaveKey() second save error = %v", err)
	}

	// Load and verify it's the second key
	loadedKey, err := km.LoadKey(keyPath)
	if err != nil {
		t.Fatalf("LoadKey() error = %v", err)
	}

	if !bytes.Equal(loadedKey, key2) {
		t.Error("loaded key does not match second key")
	}

	if bytes.Equal(loadedKey, key1) {
		t.Error("loaded key incorrectly matches first key")
	}
}

func TestFileKeyManager_Interface(t *testing.T) {
	// Verify FileKeyManager implements KeyManagerInterface
	var _ KeyManagerInterface = (*FileKeyManager)(nil)
}

func TestFileKeyManager_PermissionValidation(t *testing.T) {
	km := NewFileKeyManager()
	tempDir := t.TempDir()

	// Create keys with various insecure permissions
	tests := []struct {
		name        string
		permissions os.FileMode
		shouldFail  bool
	}{
		{"secure 0600", 0600, false},
		{"insecure 0644 (world readable)", 0644, true},
		{"insecure 0666 (world writable)", 0666, true},
		{"insecure 0660 (group readable)", 0660, true},
		{"insecure 0640 (group readable)", 0640, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyPath := filepath.Join(tempDir, "test_"+tt.name+".key")

			// Generate and save a key
			key, _ := km.GenerateKey()

			// Manually write file with specific permissions to test
			if err := os.WriteFile(keyPath, key, tt.permissions); err != nil {
				t.Fatalf("failed to write test file: %v", err)
			}

			// Try to load the key
			_, err := km.LoadKey(keyPath)

			if tt.shouldFail && err == nil {
				t.Error("LoadKey() should have failed for insecure permissions")
			}

			if !tt.shouldFail && err != nil {
				t.Errorf("LoadKey() should have succeeded: %v", err)
			}
		})
	}
}

func TestFileKeyManager_ConcurrentGenerate(t *testing.T) {
	km := NewFileKeyManager()
	const numGoroutines = 100

	keys := make(chan []byte, numGoroutines)
	errors := make(chan error, numGoroutines)

	// Generate keys concurrently
	for i := 0; i < numGoroutines; i++ {
		go func() {
			key, err := km.GenerateKey()
			if err != nil {
				errors <- err
				return
			}
			keys <- key
		}()
	}

	// Collect results
	generatedKeys := make([][]byte, 0, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		select {
		case err := <-errors:
			t.Fatalf("concurrent GenerateKey() error: %v", err)
		case key := <-keys:
			generatedKeys = append(generatedKeys, key)
		}
	}

	// Verify all keys are unique
	for i := 0; i < len(generatedKeys); i++ {
		for j := i + 1; j < len(generatedKeys); j++ {
			if bytes.Equal(generatedKeys[i], generatedKeys[j]) {
				t.Error("concurrent key generation produced duplicate keys")
			}
		}
	}
}
