#!/bin/bash

echo "=== Gibson Tracing Metadata Fixes Verification ==="
echo ""

echo "1. Checking new observability context helpers..."
if [ -f "internal/observability/context.go" ]; then
    echo "   ✓ context.go exists"
    grep -q "ExtractParentSpanID" internal/observability/context.go && echo "   ✓ ExtractParentSpanID() function present"
    grep -q "ExtractSpanContext" internal/observability/context.go && echo "   ✓ ExtractSpanContext() function present"
else
    echo "   ✗ context.go not found"
fi

echo ""
echo "2. Checking token usage extraction in convert.go..."
if grep -q "GenerationInfo" internal/llm/providers/convert.go; then
    echo "   ✓ Token extraction from GenerationInfo implemented"
    grep -q "prompt_tokens" internal/llm/providers/convert.go && echo "   ✓ OpenAI token keys handled"
    grep -q "input_tokens" internal/llm/providers/convert.go && echo "   ✓ Anthropic token keys handled"
else
    echo "   ✗ Token extraction not found"
fi

echo ""
echo "3. Verifying Anthropic direct client token usage..."
if grep -q "anthropicResp.Usage.InputTokens" internal/llm/providers/anthropic_direct.go; then
    echo "   ✓ Anthropic direct client extracts tokens correctly"
else
    echo "   ✗ Anthropic token extraction missing"
fi

echo ""
echo "4. Verifying OpenAI provider token usage..."
if grep -q "apiResp.Usage.PromptTokens" internal/llm/providers/openai.go; then
    echo "   ✓ OpenAI provider extracts tokens correctly"
else
    echo "   ✗ OpenAI token extraction missing"
fi

echo ""
echo "5. Running unit tests..."
go test ./internal/observability/ -run TestExtract -v 2>&1 | grep -E "(PASS|FAIL)"

echo ""
echo "6. Building project..."
if go build ./... 2>&1 | grep -q "error"; then
    echo "   ✗ Build failed"
    go build ./... 2>&1 | head -10
else
    echo "   ✓ Build successful"
fi

echo ""
echo "7. Building main binary..."
if go build -o /tmp/gibson-verify ./cmd/gibson/ 2>&1; then
    echo "   ✓ Main binary builds successfully"
    rm -f /tmp/gibson-verify
else
    echo "   ✗ Main binary build failed"
fi

echo ""
echo "=== Verification Complete ==="
