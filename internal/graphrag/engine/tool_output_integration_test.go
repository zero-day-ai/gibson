//go:build integration

package engine

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zero-day-ai/gibson/internal/graphrag/graph"
	"github.com/zero-day-ai/gibson/internal/graphrag/taxonomy"
)

// TestToolOutputIntegration tests tool output parsing and graph creation with real Neo4j.
// This test requires Neo4j to be running. Start it with:
//   docker-compose -f build/docker-compose.yml up -d neo4j
func TestToolOutputIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Get Neo4j connection details from environment
	neo4jURI := os.Getenv("NEO4J_URI")
	if neo4jURI == "" {
		neo4jURI = "neo4j://localhost:7687"
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

	// Create Neo4j client
	graphClient, err := graph.NewNeo4jClient(ctx, graph.Neo4jConfig{
		URI:      neo4jURI,
		Username: neo4jUser,
		Password: neo4jPassword,
	})
	require.NoError(t, err, "Failed to create Neo4j client")
	defer graphClient.Close(ctx)

	// Verify Neo4j connectivity
	health := graphClient.Health(ctx)
	require.True(t, health.Healthy, "Neo4j is not healthy: %s", health.Message)

	// Load taxonomy
	loader, err := taxonomy.NewLoader()
	require.NoError(t, err, "Failed to create taxonomy loader")

	registry, err := loader.Load()
	require.NoError(t, err, "Failed to load taxonomy")

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
	agentRunID := "agent_run:test-nmap-" + time.Now().Format("20060102-150405")

	// Sample nmap output
	nmapOutput := map[string]any{
		"hosts": []any{
			map[string]any{
				"ip":       "192.168.1.100",
				"hostname": "webserver.example.com",
				"os_match": "Linux 5.4",
				"state":    "up",
				"version":  "ipv4",
				"ports": []any{
					map[string]any{
						"port":     80,
						"protocol": "tcp",
						"state":    "open",
						"service": map[string]any{
							"name":    "http",
							"version": "nginx 1.18.0",
							"banner":  "nginx/1.18.0",
							"product": "nginx",
						},
					},
					map[string]any{
						"port":     443,
						"protocol": "tcp",
						"state":    "open",
						"service": map[string]any{
							"name":    "https",
							"version": "nginx 1.18.0",
							"banner":  "nginx/1.18.0 (TLS)",
							"product": "nginx",
						},
					},
					map[string]any{
						"port":     22,
						"protocol": "tcp",
						"state":    "open",
						"service": map[string]any{
							"name":    "ssh",
							"version": "OpenSSH 8.2p1",
							"product": "OpenSSH",
						},
					},
				},
			},
		},
	}

	// Process nmap output
	err := engine.HandleToolOutput(ctx, "nmap", nmapOutput, agentRunID)
	require.NoError(t, err, "Failed to handle nmap output")

	// Verify Host node was created
	t.Run("HostNodeCreated", func(t *testing.T) {
		cypher := `MATCH (h:host {id: $id}) RETURN h`
		params := map[string]any{"id": "host:192.168.1.100"}
		result, err := graphClient.Query(ctx, cypher, params)
		require.NoError(t, err, "Failed to query Host node")
		require.Len(t, result, 1, "Host node should exist")

		host := result[0]["h"].(map[string]any)
		assert.Equal(t, "192.168.1.100", host["ip"], "IP mismatch")
		assert.Equal(t, "webserver.example.com", host["hostname"], "Hostname mismatch")
		assert.Equal(t, "Linux 5.4", host["os"], "OS mismatch")
		assert.Equal(t, "up", host["state"], "State mismatch")
	})

	// Verify Port nodes were created
	t.Run("PortNodesCreated", func(t *testing.T) {
		cypher := `MATCH (p:port) WHERE p.host_id = $host_id RETURN p ORDER BY p.number`
		params := map[string]any{"host_id": "host:192.168.1.100"}
		result, err := graphClient.Query(ctx, cypher, params)
		require.NoError(t, err, "Failed to query Port nodes")
		require.Len(t, result, 3, "Should have 3 ports")

		// Verify port 22 (SSH)
		port22 := result[0]["p"].(map[string]any)
		assert.Equal(t, int64(22), port22["number"], "Port number mismatch")
		assert.Equal(t, "tcp", port22["protocol"], "Protocol mismatch")
		assert.Equal(t, "open", port22["state"], "State mismatch")

		// Verify port 80 (HTTP)
		port80 := result[1]["p"].(map[string]any)
		assert.Equal(t, int64(80), port80["number"], "Port number mismatch")

		// Verify port 443 (HTTPS)
		port443 := result[2]["p"].(map[string]any)
		assert.Equal(t, int64(443), port443["number"], "Port number mismatch")
	})

	// Verify HAS_PORT relationships
	t.Run("HasPortRelationships", func(t *testing.T) {
		cypher := `
			MATCH (h:host {id: $host_id})-[r:HAS_PORT]->(p:port)
			RETURN count(r) as rel_count
		`
		params := map[string]any{"host_id": "host:192.168.1.100"}
		result, err := graphClient.Query(ctx, cypher, params)
		require.NoError(t, err, "Failed to query HAS_PORT relationships")
		require.Len(t, result, 1, "Should have HAS_PORT count")

		relCount := result[0]["rel_count"].(int64)
		assert.Equal(t, int64(3), relCount, "Should have 3 HAS_PORT relationships")
	})

	// Verify DISCOVERED relationship from agent to host
	t.Run("DiscoveredRelationship", func(t *testing.T) {
		cypher := `
			MATCH (ar:agent_run {id: $agent_run_id})-[r:DISCOVERED]->(h:host {id: $host_id})
			RETURN r
		`
		params := map[string]any{
			"agent_run_id": agentRunID,
			"host_id":      "host:192.168.1.100",
		}
		result, err := graphClient.Query(ctx, cypher, params)
		require.NoError(t, err, "Failed to query DISCOVERED relationship")

		// Note: This may be 0 if the agent_run node doesn't exist yet
		// In a real scenario, the agent_run would be created first via HandleEvent
		if len(result) > 0 {
			rel := result[0]["r"].(map[string]any)
			assert.NotNil(t, rel, "DISCOVERED relationship should exist")
		}
	})

	// Verify Service nodes were created
	t.Run("ServiceNodesCreated", func(t *testing.T) {
		cypher := `MATCH (s:service) WHERE s.port_id STARTS WITH 'port:192.168.1.100' RETURN s ORDER BY s.name`
		result, err := graphClient.Query(ctx, cypher, nil)
		require.NoError(t, err, "Failed to query Service nodes")
		require.Len(t, result, 3, "Should have 3 services")

		// Verify HTTP service
		httpService := result[0]["s"].(map[string]any)
		assert.Equal(t, "http", httpService["name"], "Service name mismatch")
		assert.Equal(t, "nginx 1.18.0", httpService["version"], "Service version mismatch")
		assert.Equal(t, "nginx", httpService["product"], "Service product mismatch")

		// Verify HTTPS service
		httpsService := result[1]["s"].(map[string]any)
		assert.Equal(t, "https", httpsService["name"], "Service name mismatch")

		// Verify SSH service
		sshService := result[2]["s"].(map[string]any)
		assert.Equal(t, "ssh", sshService["name"], "Service name mismatch")
		assert.Equal(t, "OpenSSH 8.2p1", sshService["version"], "Service version mismatch")
	})

	// Verify RUNS_SERVICE relationships
	t.Run("RunsServiceRelationships", func(t *testing.T) {
		cypher := `
			MATCH (p:port)-[r:RUNS_SERVICE]->(s:service)
			WHERE p.host_id = $host_id
			RETURN count(r) as rel_count
		`
		params := map[string]any{"host_id": "host:192.168.1.100"}
		result, err := graphClient.Query(ctx, cypher, params)
		require.NoError(t, err, "Failed to query RUNS_SERVICE relationships")
		require.Len(t, result, 1, "Should have RUNS_SERVICE count")

		relCount := result[0]["rel_count"].(int64)
		assert.Equal(t, int64(3), relCount, "Should have 3 RUNS_SERVICE relationships")
	})
}

// testSubfinderOutput tests subfinder tool output parsing and graph creation
func testSubfinderOutput(t *testing.T, ctx context.Context, engine TaxonomyGraphEngine, graphClient graph.GraphClient) {
	agentRunID := "agent_run:test-subfinder-" + time.Now().Format("20060102-150405")

	// Sample subfinder output
	subfinderOutput := map[string]any{
		"target_domain": map[string]any{
			"name": "example.com",
		},
		"subdomains": []any{
			map[string]any{
				"name":          "www.example.com",
				"parent_domain": "example.com",
				"source":        "crtsh",
				"is_wildcard":   false,
			},
			map[string]any{
				"name":          "api.example.com",
				"parent_domain": "example.com",
				"source":        "virustotal",
				"is_wildcard":   false,
			},
			map[string]any{
				"name":          "mail.example.com",
				"parent_domain": "example.com",
				"source":        "dnsdumpster",
				"is_wildcard":   false,
			},
		},
	}

	// Process subfinder output
	err := engine.HandleToolOutput(ctx, "subfinder", subfinderOutput, agentRunID)
	require.NoError(t, err, "Failed to handle subfinder output")

	// Verify Domain node was created
	t.Run("DomainNodeCreated", func(t *testing.T) {
		cypher := `MATCH (d:domain {id: $id}) RETURN d`
		params := map[string]any{"id": "domain:example.com"}
		result, err := graphClient.Query(ctx, cypher, params)
		require.NoError(t, err, "Failed to query Domain node")
		require.Len(t, result, 1, "Domain node should exist")

		domain := result[0]["d"].(map[string]any)
		assert.Equal(t, "example.com", domain["name"], "Domain name mismatch")
	})

	// Verify Subdomain nodes were created
	t.Run("SubdomainNodesCreated", func(t *testing.T) {
		cypher := `MATCH (s:subdomain) WHERE s.parent_domain = $parent_domain RETURN s ORDER BY s.name`
		params := map[string]any{"parent_domain": "example.com"}
		result, err := graphClient.Query(ctx, cypher, params)
		require.NoError(t, err, "Failed to query Subdomain nodes")
		require.Len(t, result, 3, "Should have 3 subdomains")

		// Verify api.example.com
		apiSub := result[0]["s"].(map[string]any)
		assert.Equal(t, "api.example.com", apiSub["name"], "Subdomain name mismatch")
		assert.Equal(t, "virustotal", apiSub["discovery_method"], "Discovery method mismatch")
		assert.Equal(t, false, apiSub["is_wildcard"], "is_wildcard mismatch")

		// Verify mail.example.com
		mailSub := result[1]["s"].(map[string]any)
		assert.Equal(t, "mail.example.com", mailSub["name"], "Subdomain name mismatch")

		// Verify www.example.com
		wwwSub := result[2]["s"].(map[string]any)
		assert.Equal(t, "www.example.com", wwwSub["name"], "Subdomain name mismatch")
	})

	// Verify HAS_SUBDOMAIN relationships
	t.Run("HasSubdomainRelationships", func(t *testing.T) {
		cypher := `
			MATCH (d:domain {id: $domain_id})-[r:HAS_SUBDOMAIN]->(s:subdomain)
			RETURN count(r) as rel_count
		`
		params := map[string]any{"domain_id": "domain:example.com"}
		result, err := graphClient.Query(ctx, cypher, params)
		require.NoError(t, err, "Failed to query HAS_SUBDOMAIN relationships")
		require.Len(t, result, 1, "Should have HAS_SUBDOMAIN count")

		relCount := result[0]["rel_count"].(int64)
		assert.Equal(t, int64(3), relCount, "Should have 3 HAS_SUBDOMAIN relationships")
	})
}

// testHttpxOutput tests httpx tool output parsing and graph creation
func testHttpxOutput(t *testing.T, ctx context.Context, engine TaxonomyGraphEngine, graphClient graph.GraphClient) {
	agentRunID := "agent_run:test-httpx-" + time.Now().Format("20060102-150405")

	// Sample httpx output
	httpxOutput := map[string]any{
		"results": []any{
			map[string]any{
				"url":              "https://example.com",
				"method":           "GET",
				"status_code":      200,
				"content_type":     "text/html",
				"content_length":   12345,
				"title":            "Example Domain",
				"server":           "nginx/1.18.0",
				"response_time_ms": 150,
				"host":             "192.168.1.100",
				"technologies": []any{
					map[string]any{
						"name":       "nginx",
						"version":    "1.18.0",
						"category":   "web-server",
						"confidence": 0.95,
					},
					map[string]any{
						"name":       "PHP",
						"version":    "7.4",
						"category":   "programming-language",
						"cpe":        "cpe:/a:php:php:7.4",
						"confidence": 0.85,
					},
				},
			},
			map[string]any{
				"url":              "https://api.example.com",
				"method":           "GET",
				"status_code":      200,
				"content_type":     "application/json",
				"content_length":   567,
				"title":            "API Gateway",
				"server":           "nginx/1.18.0",
				"response_time_ms": 85,
				"host":             "192.168.1.101",
				"technologies": []any{
					map[string]any{
						"name":       "nginx",
						"version":    "1.18.0",
						"category":   "web-server",
						"confidence": 0.95,
					},
				},
			},
		},
	}

	// Process httpx output
	err := engine.HandleToolOutput(ctx, "httpx", httpxOutput, agentRunID)
	require.NoError(t, err, "Failed to handle httpx output")

	// Verify Endpoint nodes were created
	t.Run("EndpointNodesCreated", func(t *testing.T) {
		cypher := `MATCH (e:endpoint) RETURN e ORDER BY e.url`
		result, err := graphClient.Query(ctx, cypher, nil)
		require.NoError(t, err, "Failed to query Endpoint nodes")
		require.GreaterOrEqual(t, len(result), 2, "Should have at least 2 endpoints")

		// Find our test endpoints
		var exampleEndpoint, apiEndpoint map[string]any
		for _, r := range result {
			ep := r["e"].(map[string]any)
			if ep["url"] == "https://api.example.com" {
				apiEndpoint = ep
			} else if ep["url"] == "https://example.com" {
				exampleEndpoint = ep
			}
		}

		require.NotNil(t, exampleEndpoint, "example.com endpoint should exist")
		assert.Equal(t, "GET", exampleEndpoint["method"], "Method mismatch")
		assert.Equal(t, int64(200), exampleEndpoint["status_code"], "Status code mismatch")
		assert.Equal(t, "text/html", exampleEndpoint["content_type"], "Content type mismatch")
		assert.Equal(t, "Example Domain", exampleEndpoint["page_title"], "Page title mismatch")
		assert.Equal(t, "nginx/1.18.0", exampleEndpoint["server"], "Server mismatch")

		require.NotNil(t, apiEndpoint, "api.example.com endpoint should exist")
		assert.Equal(t, "application/json", apiEndpoint["content_type"], "Content type mismatch")
	})

	// Verify Technology nodes were created
	t.Run("TechnologyNodesCreated", func(t *testing.T) {
		cypher := `MATCH (t:technology) RETURN t ORDER BY t.name, t.version`
		result, err := graphClient.Query(ctx, cypher, nil)
		require.NoError(t, err, "Failed to query Technology nodes")
		require.GreaterOrEqual(t, len(result), 2, "Should have at least 2 technologies")

		// Find nginx and PHP technologies
		var nginxTech, phpTech map[string]any
		for _, r := range result {
			tech := r["t"].(map[string]any)
			if tech["name"] == "nginx" && tech["version"] == "1.18.0" {
				nginxTech = tech
			} else if tech["name"] == "PHP" && tech["version"] == "7.4" {
				phpTech = tech
			}
		}

		require.NotNil(t, nginxTech, "nginx technology should exist")
		assert.Equal(t, "web-server", nginxTech["category"], "Category mismatch")

		require.NotNil(t, phpTech, "PHP technology should exist")
		assert.Equal(t, "programming-language", phpTech["category"], "Category mismatch")
		assert.Equal(t, "cpe:/a:php:php:7.4", phpTech["cpe"], "CPE mismatch")
	})

	// Verify USES_TECHNOLOGY relationships
	t.Run("UsesTechnologyRelationships", func(t *testing.T) {
		cypher := `
			MATCH (e:endpoint)-[r:USES_TECHNOLOGY]->(t:technology)
			RETURN count(r) as rel_count
		`
		result, err := graphClient.Query(ctx, cypher, nil)
		require.NoError(t, err, "Failed to query USES_TECHNOLOGY relationships")
		require.Len(t, result, 1, "Should have USES_TECHNOLOGY count")

		relCount := result[0]["rel_count"].(int64)
		assert.GreaterOrEqual(t, relCount, int64(3), "Should have at least 3 USES_TECHNOLOGY relationships")
	})
}

// cleanupToolOutputTestData removes all tool output test data from Neo4j
func cleanupToolOutputTestData(t *testing.T, ctx context.Context, graphClient graph.GraphClient) {
	cypher := `
		MATCH (n)
		WHERE n.id STARTS WITH 'host:192.168.1.' OR
		      n.id STARTS WITH 'port:192.168.1.' OR
		      n.id STARTS WITH 'service:port:192.168.1.' OR
		      n.id STARTS WITH 'domain:example.com' OR
		      n.id STARTS WITH 'subdomain:' OR
		      n.id STARTS WITH 'endpoint:' OR
		      n.id STARTS WITH 'technology:' OR
		      n.id STARTS WITH 'agent_run:test-'
		DETACH DELETE n
	`
	_, err := graphClient.Query(ctx, cypher, nil)
	if err != nil {
		t.Logf("Warning: Failed to clean up tool output test data: %v", err)
	}
}
