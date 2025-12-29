package workflow

import (
	"fmt"
	"strconv"
)

// Condition evaluation for workflow DAG nodes.
//
// This package provides a recursive descent parser and evaluator for conditional
// expressions used in workflow DAG branching logic. It supports:
//
//   - Path resolution: "nodes.recon.output.found_endpoint" resolves to context.NodeResults["recon"].Output["found_endpoint"]
//   - Comparison operators: ==, !=, <, >, <=, >=
//   - Boolean operators: &&, ||, !
//   - Literals: true, false, numbers, quoted strings
//   - Parentheses for grouping
//   - Built-in functions: len(), empty(), exists()
//   - Custom functions via RegisterFunction()
//   - Array/map indexing with []
//
// Expression examples:
//
//	nodes.recon.status == "completed" && nodes.recon.output.found_endpoint
//	len(nodes.scan.output.endpoints) > 0
//	!empty(nodes.recon.output.vulnerabilities) && nodes.recon.output.critical > threshold
//	(nodes.scan.status == "completed" && nodes.exploit.status == "completed") || skip_exploit
//
// All expressions must evaluate to a boolean value. Invalid expressions return
// WorkflowError with code ErrExpressionInvalid.

// EvaluationContext provides the context for evaluating conditional expressions
type EvaluationContext struct {
	// NodeResults contains the results of all executed nodes, indexed by node ID
	NodeResults map[string]*NodeResult
	// Variables contains custom variables that can be referenced in expressions
	Variables map[string]any
}

// ConditionFunc is a function that can be called within conditional expressions
type ConditionFunc func(args []any) (any, error)

// ConditionEvaluator parses and evaluates conditional expressions
type ConditionEvaluator struct {
	functions map[string]ConditionFunc
}

// NewConditionEvaluator creates a new ConditionEvaluator with default functions
func NewConditionEvaluator() *ConditionEvaluator {
	evaluator := &ConditionEvaluator{
		functions: make(map[string]ConditionFunc),
	}

	// Register default functions
	evaluator.RegisterFunction("len", func(args []any) (any, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("len() requires exactly 1 argument, got %d", len(args))
		}
		switch v := args[0].(type) {
		case string:
			return float64(len(v)), nil
		case []any:
			return float64(len(v)), nil
		case map[string]any:
			return float64(len(v)), nil
		default:
			return nil, fmt.Errorf("len() requires string, array, or map argument")
		}
	})

	evaluator.RegisterFunction("empty", func(args []any) (any, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("empty() requires exactly 1 argument, got %d", len(args))
		}
		switch v := args[0].(type) {
		case string:
			return len(v) == 0, nil
		case []any:
			return len(v) == 0, nil
		case map[string]any:
			return len(v) == 0, nil
		case nil:
			return true, nil
		default:
			return false, nil
		}
	})

	evaluator.RegisterFunction("exists", func(args []any) (any, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("exists() requires exactly 1 argument, got %d", len(args))
		}
		return args[0] != nil, nil
	})

	return evaluator
}

// RegisterFunction adds a custom function that can be called in expressions
func (ce *ConditionEvaluator) RegisterFunction(name string, fn ConditionFunc) {
	ce.functions[name] = fn
}

// Evaluate parses and evaluates a conditional expression in the given context
func (ce *ConditionEvaluator) Evaluate(expr string, context *EvaluationContext) (bool, error) {
	tokens, err := tokenize(expr)
	if err != nil {
		return false, &WorkflowError{
			Code:    WorkflowErrorExpressionInvalid,
			Message: fmt.Sprintf("failed to tokenize expression: %v", err),
			Cause:   err,
		}
	}

	parser := &parser{
		tokens:    tokens,
		pos:       0,
		context:   context,
		evaluator: ce,
	}

	result, err := parser.parseExpression()
	if err != nil {
		return false, &WorkflowError{
			Code:    WorkflowErrorExpressionInvalid,
			Message: fmt.Sprintf("failed to evaluate expression: %v", err),
			Cause:   err,
		}
	}

	// Convert result to boolean
	boolResult, ok := result.(bool)
	if !ok {
		return false, &WorkflowError{
			Code:    WorkflowErrorExpressionInvalid,
			Message: fmt.Sprintf("expression did not evaluate to boolean, got %T", result),
		}
	}

	return boolResult, nil
}

// tokenType represents the type of a token
type tokenType int

const (
	tokenEOF tokenType = iota
	tokenIdentifier
	tokenNumber
	tokenString
	tokenBool
	tokenDot
	tokenComma
	tokenLParen
	tokenRParen
	tokenLBracket
	tokenRBracket
	tokenEQ
	tokenNE
	tokenLT
	tokenLE
	tokenGT
	tokenGE
	tokenAnd
	tokenOr
	tokenNot
)

// token represents a lexical token
type token struct {
	typ   tokenType
	value string
}

// tokenize converts an expression string into a slice of tokens
func tokenize(expr string) ([]token, error) {
	var tokens []token
	i := 0

	for i < len(expr) {
		// Skip whitespace
		if expr[i] == ' ' || expr[i] == '\t' || expr[i] == '\n' || expr[i] == '\r' {
			i++
			continue
		}

		// Single character tokens
		switch expr[i] {
		case '.':
			tokens = append(tokens, token{typ: tokenDot, value: "."})
			i++
			continue
		case ',':
			tokens = append(tokens, token{typ: tokenComma, value: ","})
			i++
			continue
		case '(':
			tokens = append(tokens, token{typ: tokenLParen, value: "("})
			i++
			continue
		case ')':
			tokens = append(tokens, token{typ: tokenRParen, value: ")"})
			i++
			continue
		case '[':
			tokens = append(tokens, token{typ: tokenLBracket, value: "["})
			i++
			continue
		case ']':
			tokens = append(tokens, token{typ: tokenRBracket, value: "]"})
			i++
			continue
		}

		// Multi-character operators
		if i+1 < len(expr) {
			twoChar := expr[i : i+2]
			switch twoChar {
			case "==":
				tokens = append(tokens, token{typ: tokenEQ, value: "=="})
				i += 2
				continue
			case "!=":
				tokens = append(tokens, token{typ: tokenNE, value: "!="})
				i += 2
				continue
			case "<=":
				tokens = append(tokens, token{typ: tokenLE, value: "<="})
				i += 2
				continue
			case ">=":
				tokens = append(tokens, token{typ: tokenGE, value: ">="})
				i += 2
				continue
			case "&&":
				tokens = append(tokens, token{typ: tokenAnd, value: "&&"})
				i += 2
				continue
			case "||":
				tokens = append(tokens, token{typ: tokenOr, value: "||"})
				i += 2
				continue
			}
		}

		// Single character comparison operators
		switch expr[i] {
		case '<':
			tokens = append(tokens, token{typ: tokenLT, value: "<"})
			i++
			continue
		case '>':
			tokens = append(tokens, token{typ: tokenGT, value: ">"})
			i++
			continue
		case '!':
			tokens = append(tokens, token{typ: tokenNot, value: "!"})
			i++
			continue
		}

		// String literals
		if expr[i] == '"' || expr[i] == '\'' {
			quote := expr[i]
			i++
			start := i
			for i < len(expr) && expr[i] != quote {
				if expr[i] == '\\' && i+1 < len(expr) {
					i += 2 // Skip escaped character
				} else {
					i++
				}
			}
			if i >= len(expr) {
				return nil, fmt.Errorf("unterminated string literal")
			}
			tokens = append(tokens, token{typ: tokenString, value: expr[start:i]})
			i++ // Skip closing quote
			continue
		}

		// Numbers
		if expr[i] >= '0' && expr[i] <= '9' {
			start := i
			for i < len(expr) && ((expr[i] >= '0' && expr[i] <= '9') || expr[i] == '.') {
				i++
			}
			tokens = append(tokens, token{typ: tokenNumber, value: expr[start:i]})
			continue
		}

		// Identifiers and keywords
		if (expr[i] >= 'a' && expr[i] <= 'z') || (expr[i] >= 'A' && expr[i] <= 'Z') || expr[i] == '_' {
			start := i
			for i < len(expr) && ((expr[i] >= 'a' && expr[i] <= 'z') ||
				(expr[i] >= 'A' && expr[i] <= 'Z') ||
				(expr[i] >= '0' && expr[i] <= '9') ||
				expr[i] == '_') {
				i++
			}
			value := expr[start:i]

			// Check for boolean keywords
			if value == "true" || value == "false" {
				tokens = append(tokens, token{typ: tokenBool, value: value})
			} else {
				tokens = append(tokens, token{typ: tokenIdentifier, value: value})
			}
			continue
		}

		return nil, fmt.Errorf("unexpected character at position %d: %c", i, expr[i])
	}

	tokens = append(tokens, token{typ: tokenEOF})
	return tokens, nil
}

// parser implements a recursive descent parser for conditional expressions
type parser struct {
	tokens    []token
	pos       int
	context   *EvaluationContext
	evaluator *ConditionEvaluator
}

// current returns the current token
func (p *parser) current() token {
	if p.pos >= len(p.tokens) {
		return token{typ: tokenEOF}
	}
	return p.tokens[p.pos]
}

// advance moves to the next token
func (p *parser) advance() {
	if p.pos < len(p.tokens) {
		p.pos++
	}
}

// expect checks if the current token is of the expected type and advances
func (p *parser) expect(typ tokenType) error {
	if p.current().typ != typ {
		return fmt.Errorf("expected %v, got %v", typ, p.current().typ)
	}
	p.advance()
	return nil
}

// parseExpression parses the top-level expression (OR has lowest precedence)
func (p *parser) parseExpression() (any, error) {
	return p.parseOr()
}

// parseOr parses logical OR expressions
func (p *parser) parseOr() (any, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}

	for p.current().typ == tokenOr {
		p.advance()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}

		leftBool, ok := left.(bool)
		if !ok {
			return nil, fmt.Errorf("|| operator requires boolean operands")
		}
		rightBool, ok := right.(bool)
		if !ok {
			return nil, fmt.Errorf("|| operator requires boolean operands")
		}

		left = leftBool || rightBool
	}

	return left, nil
}

// parseAnd parses logical AND expressions
func (p *parser) parseAnd() (any, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}

	for p.current().typ == tokenAnd {
		p.advance()
		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}

		leftBool, ok := left.(bool)
		if !ok {
			return nil, fmt.Errorf("&& operator requires boolean operands")
		}
		rightBool, ok := right.(bool)
		if !ok {
			return nil, fmt.Errorf("&& operator requires boolean operands")
		}

		left = leftBool && rightBool
	}

	return left, nil
}

// parseNot parses logical NOT expressions
func (p *parser) parseNot() (any, error) {
	if p.current().typ == tokenNot {
		p.advance()
		expr, err := p.parseNot()
		if err != nil {
			return nil, err
		}

		boolExpr, ok := expr.(bool)
		if !ok {
			return nil, fmt.Errorf("! operator requires boolean operand")
		}

		return !boolExpr, nil
	}

	return p.parseComparison()
}

// parseComparison parses comparison expressions
func (p *parser) parseComparison() (any, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}

	tok := p.current()
	switch tok.typ {
	case tokenEQ, tokenNE, tokenLT, tokenLE, tokenGT, tokenGE:
		p.advance()
		right, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		return p.compare(left, right, tok.typ)
	}

	return left, nil
}

// parsePrimary parses primary expressions (literals, identifiers, function calls, parentheses)
func (p *parser) parsePrimary() (any, error) {
	tok := p.current()

	switch tok.typ {
	case tokenBool:
		p.advance()
		return tok.value == "true", nil

	case tokenNumber:
		p.advance()
		return strconv.ParseFloat(tok.value, 64)

	case tokenString:
		p.advance()
		return tok.value, nil

	case tokenLParen:
		p.advance()
		expr, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if err := p.expect(tokenRParen); err != nil {
			return nil, err
		}
		return expr, nil

	case tokenIdentifier:
		return p.parseIdentifierOrFunction()

	default:
		return nil, fmt.Errorf("unexpected token: %v", tok.typ)
	}
}

// parseIdentifierOrFunction parses an identifier path or function call
func (p *parser) parseIdentifierOrFunction() (any, error) {
	name := p.current().value
	p.advance()

	// Check if it's a function call
	if p.current().typ == tokenLParen {
		return p.parseFunctionCall(name)
	}

	// Otherwise, it's a path expression
	return p.resolvePath(name)
}

// parseFunctionCall parses a function call with arguments
func (p *parser) parseFunctionCall(name string) (any, error) {
	fn, ok := p.evaluator.functions[name]
	if !ok {
		return nil, fmt.Errorf("unknown function: %s", name)
	}

	p.advance() // consume '('

	var args []any
	if p.current().typ != tokenRParen {
		for {
			arg, err := p.parseExpression()
			if err != nil {
				return nil, err
			}
			args = append(args, arg)

			if p.current().typ != tokenComma {
				break
			}
			p.advance() // consume ','
		}
	}

	if err := p.expect(tokenRParen); err != nil {
		return nil, err
	}

	return fn(args)
}

// resolvePath resolves a path expression like "nodes.recon.output.found_endpoint"
func (p *parser) resolvePath(name string) (any, error) {
	path := []string{name}

	// Build the complete path
	for p.current().typ == tokenDot {
		p.advance()
		if p.current().typ != tokenIdentifier {
			return nil, fmt.Errorf("expected identifier after '.'")
		}
		path = append(path, p.current().value)
		p.advance()
	}

	// Handle array/map indexing
	for p.current().typ == tokenLBracket {
		p.advance()
		index, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		if err := p.expect(tokenRBracket); err != nil {
			return nil, err
		}

		// Resolve current path first
		current, err := p.resolvePathValue(path)
		if err != nil {
			return nil, err
		}

		// Apply index
		switch v := current.(type) {
		case map[string]any:
			indexStr, ok := index.(string)
			if !ok {
				return nil, fmt.Errorf("map index must be string")
			}
			current = v[indexStr]
		case []any:
			indexNum, ok := index.(float64)
			if !ok {
				return nil, fmt.Errorf("array index must be number")
			}
			idx := int(indexNum)
			if idx < 0 || idx >= len(v) {
				return nil, fmt.Errorf("array index out of bounds: %d", idx)
			}
			current = v[idx]
		default:
			return nil, fmt.Errorf("cannot index %T", v)
		}

		// For subsequent dots, start fresh with current value
		if p.current().typ == tokenDot {
			return p.continuePathResolution(current)
		}

		return current, nil
	}

	return p.resolvePathValue(path)
}

// continuePathResolution continues resolving a path from a given value
func (p *parser) continuePathResolution(current any) (any, error) {
	for p.current().typ == tokenDot {
		p.advance()
		if p.current().typ != tokenIdentifier {
			return nil, fmt.Errorf("expected identifier after '.'")
		}
		fieldName := p.current().value
		p.advance()

		switch v := current.(type) {
		case map[string]any:
			current = v[fieldName]
		default:
			return nil, fmt.Errorf("cannot access field %s on %T", fieldName, v)
		}
	}
	return current, nil
}

// resolvePathValue resolves a path to its value in the context
func (p *parser) resolvePathValue(path []string) (any, error) {
	if len(path) == 0 {
		return nil, fmt.Errorf("empty path")
	}

	var current any

	// Check for special paths
	if path[0] == "nodes" {
		if len(path) < 2 {
			return nil, fmt.Errorf("nodes path requires node ID")
		}
		if p.context.NodeResults == nil {
			return nil, fmt.Errorf("no node results available")
		}

		nodeID := path[1]
		result, ok := p.context.NodeResults[nodeID]
		if !ok {
			return nil, fmt.Errorf("node result not found: %s", nodeID)
		}

		if len(path) == 2 {
			return result, nil
		}

		// Navigate into node result
		current = map[string]any{
			"status":   string(result.Status),
			"output":   result.Output,
			"error":    result.Error,
			"duration": result.Duration.Seconds(),
		}
		path = path[2:] // Remove "nodes" and node ID
	} else {
		// Check variables
		if p.context.Variables == nil {
			return nil, fmt.Errorf("variable not found: %s", path[0])
		}
		var ok bool
		current, ok = p.context.Variables[path[0]]
		if !ok {
			return nil, fmt.Errorf("variable not found: %s", path[0])
		}
		path = path[1:]
	}

	// Navigate the rest of the path
	for _, segment := range path {
		switch v := current.(type) {
		case map[string]any:
			current = v[segment]
		default:
			return nil, fmt.Errorf("cannot access field %s on %T", segment, v)
		}

		if current == nil {
			return nil, nil
		}
	}

	return current, nil
}

// compare performs comparison operations
func (p *parser) compare(left, right any, op tokenType) (bool, error) {
	switch op {
	case tokenEQ:
		return p.equals(left, right), nil
	case tokenNE:
		return !p.equals(left, right), nil
	case tokenLT, tokenLE, tokenGT, tokenGE:
		return p.compareOrdered(left, right, op)
	default:
		return false, fmt.Errorf("unknown comparison operator: %v", op)
	}
}

// equals checks equality between two values
func (p *parser) equals(left, right any) bool {
	// Handle nil cases
	if left == nil && right == nil {
		return true
	}
	if left == nil || right == nil {
		return false
	}

	// Type-specific comparison
	switch l := left.(type) {
	case bool:
		r, ok := right.(bool)
		return ok && l == r
	case float64:
		r, ok := right.(float64)
		return ok && l == r
	case string:
		r, ok := right.(string)
		return ok && l == r
	default:
		return false
	}
}

// compareOrdered performs ordered comparisons (<, <=, >, >=)
func (p *parser) compareOrdered(left, right any, op tokenType) (bool, error) {
	// Convert to float64 for numeric comparison
	leftNum, leftOk := toNumber(left)
	rightNum, rightOk := toNumber(right)

	if !leftOk || !rightOk {
		// Try string comparison
		leftStr, leftStrOk := left.(string)
		rightStr, rightStrOk := right.(string)
		if !leftStrOk || !rightStrOk {
			return false, fmt.Errorf("cannot compare %T and %T", left, right)
		}

		switch op {
		case tokenLT:
			return leftStr < rightStr, nil
		case tokenLE:
			return leftStr <= rightStr, nil
		case tokenGT:
			return leftStr > rightStr, nil
		case tokenGE:
			return leftStr >= rightStr, nil
		}
	}

	switch op {
	case tokenLT:
		return leftNum < rightNum, nil
	case tokenLE:
		return leftNum <= rightNum, nil
	case tokenGT:
		return leftNum > rightNum, nil
	case tokenGE:
		return leftNum >= rightNum, nil
	default:
		return false, fmt.Errorf("unknown comparison operator: %v", op)
	}
}

// toNumber attempts to convert a value to float64
func toNumber(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case string:
		if num, err := strconv.ParseFloat(val, 64); err == nil {
			return num, true
		}
	}
	return 0, false
}
