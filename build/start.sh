#!/bin/bash
# start.sh - Start the Gibson development stack
#
# Usage:
#   ./start.sh              # Start core services
#   ./start.sh --ollama     # Include local Ollama LLM
#   ./start.sh --pull       # Pull latest images first
#   ./start.sh --clean      # Clean start (remove volumes)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

# Parse arguments
PULL_IMAGES=false
INCLUDE_OLLAMA=false
CLEAN_START=false

for arg in "$@"; do
    case $arg in
        --pull)
            PULL_IMAGES=true
            ;;
        --ollama)
            INCLUDE_OLLAMA=true
            ;;
        --clean)
            CLEAN_START=true
            ;;
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --pull     Pull latest images before starting"
            echo "  --ollama   Include Ollama local LLM service"
            echo "  --clean    Remove volumes and start fresh"
            echo "  --help     Show this help message"
            exit 0
            ;;
    esac
done

# Check for .env file
if [ ! -f ".env" ]; then
    echo -e "${YELLOW}No .env file found. Creating from .env.example...${NC}"
    if [ -f ".env.example" ]; then
        cp .env.example .env
        echo -e "${YELLOW}Please edit .env and add your API keys${NC}"
    else
        echo -e "${RED}Error: .env.example not found${NC}"
        exit 1
    fi
fi

# Check for required API keys
source .env 2>/dev/null || true
if [ -z "$ANTHROPIC_API_KEY" ] && [ -z "$OPENAI_API_KEY" ]; then
    echo -e "${YELLOW}Warning: No LLM API keys configured in .env${NC}"
    echo -e "${YELLOW}Gibson requires at least one of: ANTHROPIC_API_KEY or OPENAI_API_KEY${NC}"
fi

# Clean start
if [ "$CLEAN_START" = true ]; then
    echo -e "${YELLOW}Removing existing volumes...${NC}"
    docker compose -f docker-compose.yml down -v 2>/dev/null || true
fi

# Pull images
if [ "$PULL_IMAGES" = true ]; then
    echo -e "${BLUE}Pulling latest images...${NC}"
    docker compose -f docker-compose.yml pull
fi

# Build compose command
COMPOSE_CMD="docker compose -f docker-compose.yml"
if [ "$INCLUDE_OLLAMA" = true ]; then
    COMPOSE_CMD="$COMPOSE_CMD --profile ollama"
fi

echo -e "${BLUE}Starting Gibson development stack...${NC}"
$COMPOSE_CMD up -d

# Wait for services
echo -e "${BLUE}Waiting for services to be healthy...${NC}"
sleep 5

# Print status
echo ""
echo -e "${GREEN}Gibson development stack is starting!${NC}"
echo ""
echo -e "${BLUE}Services:${NC}"
echo "  Gibson:      docker exec -it gibson bash"
echo "  Neo4j:       http://localhost:7474 (neo4j/gibson-dev-2024)"
echo "  Qdrant:      http://localhost:6333/dashboard"
echo "  Langfuse:    http://localhost:3000 (admin@gibson.local/gibson-admin)"
echo "  Jaeger:      http://localhost:16686"
echo "  Prometheus:  http://localhost:9090"
echo "  Grafana:     http://localhost:3001 (admin/gibson-admin)"
if [ "$INCLUDE_OLLAMA" = true ]; then
echo "  Ollama:      http://localhost:11434"
fi
echo ""
echo -e "${BLUE}Langfuse API Keys (for Gibson config):${NC}"
echo "  Public Key:  pk-gibson-dev"
echo "  Secret Key:  sk-gibson-dev"
echo ""
echo -e "${BLUE}Quick start:${NC}"
echo "  docker exec -it gibson bash"
echo "  gibson init"
echo "  gibson mission create --name 'test-mission'"
echo ""
echo -e "${BLUE}View logs:${NC}"
echo "  docker compose -f $SCRIPT_DIR/docker-compose.yml logs -f gibson"
echo ""
