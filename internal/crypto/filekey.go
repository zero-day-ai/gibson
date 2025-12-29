package crypto

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	// KeyFilePermission defines the file permissions for the key file
	// 0600 means only the owner can read and write
	KeyFilePermission = 0600
)

// KeyManagerInterface defines the interface for managing encryption keys.
// This is separate from the existing KeyManager to provide path-based key management.
type KeyManagerInterface interface {
	// GenerateKey generates a new cryptographically secure random key
	GenerateKey() ([]byte, error)

	// LoadKey loads a key from the specified file path
	LoadKey(path string) ([]byte, error)

	// SaveKey saves a key to the specified file path with secure permissions
	SaveKey(key []byte, path string) error

	// KeyExists checks if a key file exists at the specified path
	KeyExists(path string) bool
}

// FileKeyManager implements KeyManagerInterface using the filesystem.
// Unlike the existing KeyManager, this works with full file paths instead
// of a key directory + name pattern.
type FileKeyManager struct {
	keySize int
}

// NewFileKeyManager creates a new FileKeyManager with the default key size (32 bytes for AES-256).
func NewFileKeyManager() *FileKeyManager {
	return &FileKeyManager{
		keySize: KeySize,
	}
}

// GenerateKey generates a new cryptographically secure random key
// using crypto/rand. This key is suitable for AES-256 encryption.
func (m *FileKeyManager) GenerateKey() ([]byte, error) {
	key := make([]byte, m.keySize)

	// Read random bytes from the system's CSPRNG
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("failed to generate random key: %w", err)
	}

	return key, nil
}

// SaveKey saves the key to a file with secure permissions (0600).
// The file is created with owner read/write only permissions.
// The directory path will be created if it doesn't exist.
func (m *FileKeyManager) SaveKey(key []byte, path string) error {
	if len(key) != m.keySize {
		return fmt.Errorf("invalid key size: expected %d bytes, got %d bytes", m.keySize, len(key))
	}

	// Ensure the directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create key directory: %w", err)
	}

	// Write the key with secure permissions
	// Using WriteFile with 0600 ensures the file is created with restricted permissions
	if err := os.WriteFile(path, key, KeyFilePermission); err != nil {
		return fmt.Errorf("failed to write key file: %w", err)
	}

	// Verify permissions were set correctly (defense in depth)
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to verify key file permissions: %w", err)
	}

	if info.Mode().Perm() != KeyFilePermission {
		return fmt.Errorf("key file has incorrect permissions: got %o, expected %o", info.Mode().Perm(), KeyFilePermission)
	}

	return nil
}

// LoadKey loads a key from a file, verifying that the file has secure permissions.
// Returns an error if:
//   - The file doesn't exist
//   - The file has insecure permissions (not 0600)
//   - The key size is incorrect
func (m *FileKeyManager) LoadKey(path string) ([]byte, error) {
	// Check file info and permissions before reading
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("key file does not exist: %s", path)
		}
		return nil, fmt.Errorf("failed to stat key file: %w", err)
	}

	// Verify the file has secure permissions
	perm := info.Mode().Perm()
	if perm != KeyFilePermission {
		return nil, fmt.Errorf(
			"key file has insecure permissions: %o (expected %o). "+
				"The key file must not be readable by group or others. "+
				"Fix with: chmod %o %s",
			perm, KeyFilePermission, KeyFilePermission, path,
		)
	}

	// Read the key file
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}

	// Verify the key size
	if len(key) != m.keySize {
		return nil, fmt.Errorf("invalid key size in file: expected %d bytes, got %d bytes", m.keySize, len(key))
	}

	return key, nil
}

// KeyExists checks if a key file exists at the specified path.
func (m *FileKeyManager) KeyExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
