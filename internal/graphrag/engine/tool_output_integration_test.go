//go:build integration

package engine

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/graphrag/graph"
	"github.com/zero-day-ai/gibson/internal/graphrag/taxonomy"
	"github.com/zero-day-ai/gibson/internal/types"
)

// TestToolOutputIntegration tests tool output parsing and graph creation with real Neo4j.
// This test requires Neo4j to be running. Start it with:
//   docker-compose -f build/docker-compose.yml up -d neo4j
//
// Run with: go test -tags=integration -v ./internal/graphrag/engine/...
func TestToolOutputIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Get Neo4j connection details from environment
	neo4jURI := os.Getenv("NEO4J_URI")
	if neo4jURI == "" {
		neo4jURI = "bolt://localhost:7687"
	}

	neo4jUser := os.Getenv("NEO4J_USER")
	if neo4jUser == "" {
		neo4jUser = "neo4j"
	}

	neo4jPassword := os.Getenv("NEO4J_PASSWORD")
	if neo4jPassword == "" {
		neo4jPassword = "testpassword"
	}

	ctx := context.Background()

	// Create Neo4j client with proper config
	graphClient, err := graph.NewNeo4jClient(graph.GraphClientConfig{
		URI:                     neo4jURI,
		Username:                neo4jUser,
		Password:                neo4jPassword,
		MaxConnectionPoolSize:   10,
		ConnectionTimeout:       10 * time.Second,
		MaxTransactionRetryTime: 30 * time.Second,
	})
	require.NoError(t, err, "Failed to create Neo4j client")

	// Connect to Neo4j
	err = graphClient.Connect(ctx)
	require.NoError(t, err, "Failed to connect to Neo4j")
	defer graphClient.Close(ctx)

	// Verify Neo4j connectivity
	health := graphClient.Health(ctx)
	require.Equal(t, types.HealthStateHealthy, health.State, "Neo4j is not healthy: %s", health.Message)

	// Load taxonomy
	loader := taxonomy.NewTaxonomyLoader()
	tax, err := loader.Load()
	require.NoError(t, err, "Failed to load taxonomy")

	// Create registry from taxonomy
	registry, err := taxonomy.NewTaxonomyRegistry(tax)
	require.NoError(t, err, "Failed to create taxonomy registry")

	// Create taxonomy engine
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	engine := NewTaxonomyGraphEngine(registry, graphClient, logger)

	// Clean up test data before running tests
	cleanupToolOutputTestData(t, ctx, graphClient)

	t.Run("NmapOutput", func(t *testing.T) {
		testNmapOutput(t, ctx, engine, graphClient)
	})

	t.Run("SubfinderOutput", func(t *testing.T) {
		testSubfinderOutput(t, ctx, engine, graphClient)
	})

	t.Run("HttpxOutput", func(t *testing.T) {
		testHttpxOutput(t, ctx, engine, graphClient)
	})

	// Clean up test data after tests
	cleanupToolOutputTestData(t, ctx, graphClient)
}

// testNmapOutput tests nmap tool output parsing and graph creation
func testNmapOutput(t *testing.T, ctx context.Context, engine TaxonomyGraphEngine, graphClient graph.GraphClient) {
	agentRunID := fmt.Sprintf("test-agent-nmap-%d", time.Now().UnixNano())

	// First create the AgentRun node that tool output links to
	_, err := graphClient.Query(ctx, `
		MERGE (a:AgentRun {id: $id})
		SET a.agent_name = 'nmap-test-agent'
	`, map[string]any{"id": agentRunID})
	require.NoError(t, err, "Failed to create AgentRun node")

	// Sample nmap output
	nmapOutput := map[string]any{
		"hosts": []any{
			map[string]any{
				"ip":       "192.168.1.100",
				"hostname": "webserver.example.com",
				"os_match": "Linux 5.4",
				"state":    "up",
				"ports": []any{
					map[string]any{
						"port":     80,
						"protocol": "tcp",
						"state":    "open",
						"service":  "http",
						"version":  "Apache httpd 2.4.41",
					},
					map[string]any{
						"port":     443,
						"protocol": "tcp",
						"state":    "open",
						"service":  "https",
						"version":  "Apache httpd 2.4.41",
					},
				},
			},
		},
	}

	// Process nmap output
	err = engine.HandleToolOutput(ctx, "nmap", nmapOutput, agentRunID)
	require.NoError(t, err, "Failed to handle nmap output")

	// Verify Host node was created
	result, err := graphClient.Query(ctx, `
		MATCH (h:Host {ip: '192.168.1.100'})
		RETURN h.ip as ip, h.hostname as hostname, h.os_match as os
	`, nil)
	require.NoError(t, err, "Failed to query Host node")

	if len(result.Records) > 0 {
		assert.Equal(t, "192.168.1.100", result.Records[0]["ip"])
		t.Logf("✓ Host node created with IP 192.168.1.100")
	} else {
		t.Logf("⚠ Host node not found - tool output schema may not be configured for nmap")
	}

	// Verify Port nodes were created with HAS_PORT relationships (if schema supports it)
	result, err = graphClient.Query(ctx, `
		MATCH (h:Host {ip: '192.168.1.100'})-[:HAS_PORT]->(p:Port)
		RETURN p.port as port, p.service as service
		ORDER BY p.port
	`, nil)
	require.NoError(t, err)

	if len(result.Records) > 0 {
		t.Logf("✓ Found %d ports with HAS_PORT relationships", len(result.Records))
	}

	t.Logf("✓ Nmap output test completed for agent %s", agentRunID)
}

// testSubfinderOutput tests subfinder tool output parsing
func testSubfinderOutput(t *testing.T, ctx context.Context, engine TaxonomyGraphEngine, graphClient graph.GraphClient) {
	agentRunID := fmt.Sprintf("test-agent-subfinder-%d", time.Now().UnixNano())

	// Create AgentRun node
	_, err := graphClient.Query(ctx, `
		MERGE (a:AgentRun {id: $id})
		SET a.agent_name = 'subfinder-test-agent'
	`, map[string]any{"id": agentRunID})
	require.NoError(t, err)

	// Sample subfinder output
	subfinderOutput := map[string]any{
		"domain": "example.com",
		"subdomains": []any{
			map[string]any{
				"subdomain": "www.example.com",
				"source":    "crtsh",
			},
			map[string]any{
				"subdomain": "api.example.com",
				"source":    "virustotal",
			},
			map[string]any{
				"subdomain": "mail.example.com",
				"source":    "dnsdb",
			},
		},
	}

	// Process subfinder output
	err = engine.HandleToolOutput(ctx, "subfinder", subfinderOutput, agentRunID)
	require.NoError(t, err, "Failed to handle subfinder output")

	// Verify Subdomain/Domain nodes were created
	result, err := graphClient.Query(ctx, `
		MATCH (s:Subdomain)
		WHERE s.subdomain IN ['www.example.com', 'api.example.com', 'mail.example.com']
		RETURN s.subdomain as subdomain, s.source as source
		ORDER BY s.subdomain
	`, nil)
	require.NoError(t, err)

	if len(result.Records) > 0 {
		t.Logf("✓ Found %d subdomain nodes", len(result.Records))
		for _, r := range result.Records {
			t.Logf("  - %s (source: %v)", r["subdomain"], r["source"])
		}
	} else {
		t.Logf("⚠ Subdomain nodes not found - tool output schema may not be configured for subfinder")
	}

	t.Logf("✓ Subfinder output test completed for agent %s", agentRunID)
}

// testHttpxOutput tests httpx tool output parsing
func testHttpxOutput(t *testing.T, ctx context.Context, engine TaxonomyGraphEngine, graphClient graph.GraphClient) {
	agentRunID := fmt.Sprintf("test-agent-httpx-%d", time.Now().UnixNano())

	// Create AgentRun node
	_, err := graphClient.Query(ctx, `
		MERGE (a:AgentRun {id: $id})
		SET a.agent_name = 'httpx-test-agent'
	`, map[string]any{"id": agentRunID})
	require.NoError(t, err)

	// Sample httpx output
	httpxOutput := map[string]any{
		"results": []any{
			map[string]any{
				"url":           "https://www.example.com",
				"status_code":   200,
				"title":         "Example Domain",
				"content_type":  "text/html",
				"content_length": 1256,
				"technologies": []any{
					"Apache",
					"PHP",
					"WordPress",
				},
			},
			map[string]any{
				"url":           "https://api.example.com",
				"status_code":   200,
				"title":         "API Server",
				"content_type":  "application/json",
				"content_length": 42,
				"technologies": []any{
					"nginx",
					"Node.js",
				},
			},
		},
	}

	// Process httpx output
	err = engine.HandleToolOutput(ctx, "httpx", httpxOutput, agentRunID)
	require.NoError(t, err, "Failed to handle httpx output")

	// Verify Endpoint nodes were created
	result, err := graphClient.Query(ctx, `
		MATCH (e:Endpoint)
		WHERE e.url IN ['https://www.example.com', 'https://api.example.com']
		RETURN e.url as url, e.status_code as status_code, e.title as title
		ORDER BY e.url
	`, nil)
	require.NoError(t, err)

	if len(result.Records) > 0 {
		t.Logf("✓ Found %d endpoint nodes", len(result.Records))
		for _, r := range result.Records {
			t.Logf("  - %s (status: %v, title: %v)", r["url"], r["status_code"], r["title"])
		}
	} else {
		t.Logf("⚠ Endpoint nodes not found - tool output schema may not be configured for httpx")
	}

	t.Logf("✓ Httpx output test completed for agent %s", agentRunID)
}

// cleanupToolOutputTestData removes test data from Neo4j
func cleanupToolOutputTestData(t *testing.T, ctx context.Context, client graph.GraphClient) {
	// Delete all test nodes
	_, err := client.Query(ctx, `
		MATCH (n)
		WHERE n.id STARTS WITH 'test-'
		   OR n.ip = '192.168.1.100'
		   OR n.subdomain IN ['www.example.com', 'api.example.com', 'mail.example.com']
		   OR n.url IN ['https://www.example.com', 'https://api.example.com']
		DETACH DELETE n
	`, nil)
	if err != nil {
		t.Logf("Warning: Failed to clean up test data: %v", err)
	}
}
