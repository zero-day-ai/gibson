package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/payload"
)

func main() {
	// Load built-in payloads from embedded files
	fmt.Println("Loading built-in payloads...")
	payloads, err := payload.LoadBuiltInPayloads()
	if err != nil {
		log.Fatalf("Failed to load built-in payloads: %v", err)
	}

	fmt.Printf("Loaded %d built-in payloads\n", len(payloads))
	for i, p := range payloads {
		fmt.Printf("  %d. %s (ID: %s, Categories: %v)\n", i+1, p.Name, p.ID, p.Categories)
	}

	// Test database storage
	dbPath := os.Getenv("GIBSON_HOME")
	if dbPath == "" {
		dbPath = os.Getenv("HOME") + "/.gibson"
	}
	dbPath = dbPath + "/gibson.db"

	fmt.Printf("\nOpening database: %s\n", dbPath)
	db, err := database.Open(dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create payload store
	store := payload.NewPayloadStore(db)

	// Save all built-in payloads
	ctx := context.Background()
	fmt.Println("\nSaving built-in payloads to database...")
	saved := 0
	for _, p := range payloads {
		// Check if payload already exists
		exists, err := store.Exists(ctx, p.ID)
		if err != nil {
			log.Printf("Warning: Failed to check if payload %s exists: %v", p.ID, err)
			continue
		}

		if exists {
			fmt.Printf("  Payload %s already exists, skipping\n", p.Name)
			continue
		}

		// Save the payload
		pCopy := p
		if err := store.Save(ctx, &pCopy); err != nil {
			log.Printf("Warning: Failed to save payload %s: %v", p.Name, err)
			continue
		}
		saved++
		fmt.Printf("  Saved: %s\n", p.Name)
	}

	fmt.Printf("\nSuccessfully saved %d/%d payloads to database\n", saved, len(payloads))

	// Verify by listing
	fmt.Println("\nVerifying payloads in database...")
	storedPayloads, err := store.List(ctx, &payload.PayloadFilter{})
	if err != nil {
		log.Fatalf("Failed to list payloads: %v", err)
	}

	fmt.Printf("Found %d payloads in database\n", len(storedPayloads))
	for i, p := range storedPayloads {
		fmt.Printf("  %d. %s (Built-in: %v, Enabled: %v)\n", i+1, p.Name, p.BuiltIn, p.Enabled)
	}
}
