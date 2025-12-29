package crypto_test

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/zero-day-ai/gibson/internal/crypto"
)

// ExampleFileKeyManager demonstrates basic usage of FileKeyManager
func ExampleFileKeyManager() {
	// Create a temporary directory for the example
	tempDir, err := os.MkdirTemp("", "gibson-keys-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	keyPath := filepath.Join(tempDir, "master.key")

	// Create a new key manager
	km := crypto.NewFileKeyManager()

	// Generate a new master key
	masterKey, err := km.GenerateKey()
	if err != nil {
		log.Fatalf("failed to generate key: %v", err)
	}

	// Save the key to disk
	if err := km.SaveKey(masterKey, keyPath); err != nil {
		log.Fatalf("failed to save key: %v", err)
	}

	fmt.Printf("Key generated and saved with secure permissions\n")

	// Check if the key exists
	if km.KeyExists(keyPath) {
		fmt.Printf("Key file exists\n")
	}

	// Load the key from disk
	loadedKey, err := km.LoadKey(keyPath)
	if err != nil {
		log.Fatalf("failed to load key: %v", err)
	}

	// Verify the loaded key matches
	fmt.Printf("Key loaded successfully, size: %d bytes\n", len(loadedKey))

	// Output:
	// Key generated and saved with secure permissions
	// Key file exists
	// Key loaded successfully, size: 32 bytes
}

// ExampleFileKeyManager_withEncryption demonstrates using FileKeyManager with encryption
func ExampleFileKeyManager_withEncryption() {
	// Create a temporary directory for the example
	tempDir, err := os.MkdirTemp("", "gibson-encryption-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	keyPath := filepath.Join(tempDir, "master.key")

	// Create key manager and encryptor
	km := crypto.NewFileKeyManager()
	encryptor := crypto.NewAESGCMEncryptor()

	// Generate and save master key
	masterKey, err := km.GenerateKey()
	if err != nil {
		log.Fatalf("failed to generate key: %v", err)
	}

	if err := km.SaveKey(masterKey, keyPath); err != nil {
		log.Fatalf("failed to save key: %v", err)
	}

	// Encrypt some data
	plaintext := []byte("sensitive data that needs encryption")
	ciphertext, iv, salt, err := encryptor.Encrypt(plaintext, masterKey)
	if err != nil {
		log.Fatalf("encryption failed: %v", err)
	}

	fmt.Printf("Data encrypted successfully\n")

	// Later, load the key and decrypt
	loadedKey, err := km.LoadKey(keyPath)
	if err != nil {
		log.Fatalf("failed to load key: %v", err)
	}

	decrypted, err := encryptor.Decrypt(ciphertext, iv, salt, loadedKey)
	if err != nil {
		log.Fatalf("decryption failed: %v", err)
	}

	fmt.Printf("Data decrypted successfully: %s\n", string(decrypted))

	// Output:
	// Data encrypted successfully
	// Data decrypted successfully: sensitive data that needs encryption
}
