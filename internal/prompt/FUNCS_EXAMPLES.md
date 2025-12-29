# Template Function Examples

This document demonstrates the usage of custom template functions available in Gibson's prompt system.

## String Functions

### toUpper / toLower / title
```go
// Template
"Hello {{ toUpper .name }}"
// With data: {"name": "gibson"}
// Result: "Hello GIBSON"

// Template
"Welcome to {{ title .city }}"
// With data: {"city": "new york"}
// Result: "Welcome to New York"
```

### trim / trimPrefix / trimSuffix
```go
// Template
"Clean: {{ trim .input }}"
// With data: {"input": "  hello  "}
// Result: "Clean: hello"

// Template
"{{ trimPrefix "http://" .url }}"
// With data: {"url": "http://example.com"}
// Result: "example.com"
```

### replace
```go
// Template
"{{ replace "," " | " .tags }}"
// With data: {"tags": "security,pentesting,recon"}
// Result: "security | pentesting | recon"
```

### contains / hasPrefix / hasSuffix
```go
// Template
"{{ if hasPrefix .url "https://" }}Secure{{ else }}Insecure{{ end }}"
// With data: {"url": "https://example.com"}
// Result: "Secure"
```

### split / join
```go
// Template
"{{ $items := split "," .csv }}{{ join " - " $items }}"
// With data: {"csv": "a,b,c"}
// Result: "a - b - c"
```

## Utility Functions

### default
```go
// Template
"Role: {{ default "user" .role }}"
// With data: {} (empty)
// Result: "Role: user"

// With data: {"role": "admin"}
// Result: "Role: admin"
```

### required
```go
// Template
"API Key: {{ required .apiKey }}"
// With data: {} (empty)
// Result: Error - required value is missing

// With data: {"apiKey": "secret123"}
// Result: "API Key: secret123"
```

### coalesce
```go
// Template
"{{ coalesce .primaryDB .secondaryDB .fallbackDB }}"
// With data: {"primaryDB": "", "secondaryDB": "db2", "fallbackDB": "db3"}
// Result: "db2"
```

### ternary
```go
// Template
"{{ ternary .isActive "ACTIVE" "INACTIVE" }}"
// With data: {"isActive": true}
// Result: "ACTIVE"
```

## Date/Time Functions

### now / formatDate
```go
// Template
"Current date: {{ formatDate "2006-01-02" (now) }}"
// Result: "Current date: 2024-12-26" (current date)

// Template
"Timestamp: {{ formatDate "15:04:05" .timestamp }}"
// With data: {"timestamp": <time.Time>}
// Result: "Timestamp: 14:30:45"
```

## JSON Functions

### toJSON / toPrettyJSON
```go
// Template
"{{ toJSON .config }}"
// With data: {"config": {"timeout": 30, "retries": 3}}
// Result: '{"timeout":30,"retries":3}'

// Template
"Config:\n{{ toPrettyJSON .config }}"
// Result:
// Config:
// {
//   "timeout": 30,
//   "retries": 3
// }
```

### fromJSON
```go
// Template
"{{ $data := fromJSON .jsonString }}{{ $data.name }}"
// With data: {"jsonString": '{"name":"Gibson","version":1}'}
// Result: "Gibson"
```

## Formatting Functions

### indent / nindent
```go
// Template
"YAML:\n{{ indent 2 .yaml }}"
// With data: {"yaml": "key: value\nfoo: bar"}
// Result:
// YAML:
//   key: value
//   foo: bar

// Template
"config:{{ nindent 2 .content }}"
// Result: newline added before indented content
```

### quote / squote
```go
// Template
"Message: {{ quote .msg }}"
// With data: {"msg": "hello world"}
// Result: 'Message: "hello world"'

// Template
"{{ squote .tag }}"
// With data: {"tag": "security"}
// Result: "'security'"
```

## Complete Example

Here's a realistic prompt using multiple functions:

```go
template := `You are {{ title .Agent.name }}.

Mission: {{ trim .Mission.description }}
Status: {{ ternary .Mission.active "Active" "Inactive" }}
Priority: {{ default "normal" .Mission.priority }}

{{ if .Target.url }}Target: {{ .Target.url }}
Security: {{ if hasPrefix .Target.url "https://" }}Enabled{{ else }}Disabled{{ end }}{{ end }}

Configuration:{{ nindent 2 (toPrettyJSON .Custom.config) }}

{{ $tags := split "," .Mission.tags }}{{ if $tags }}Tags: {{ join ", " $tags }}{{ end }}

{{ if .Instructions }}Instructions:{{ range $idx, $inst := .Instructions }}
{{ add $idx 1 }}. {{ $inst }}{{ end }}{{ end }}`

data := map[string]any{
	"Agent": map[string]any{
		"name": "gibson recon agent",
	},
	"Mission": map[string]any{
		"description": "  Perform security reconnaissance  ",
		"active":      true,
		"tags":        "recon,osint,passive",
	},
	"Target": map[string]any{
		"url": "https://example.com",
	},
	"Custom": map[string]any{
		"config": map[string]any{
			"timeout": 30,
			"retries": 3,
			"stealth": true,
		},
	},
	"Instructions": []string{
		"Identify subdomains",
		"Enumerate technologies",
		"Find public documents",
	},
}

// Result:
// You are Gibson Recon Agent.
//
// Mission: Perform security reconnaissance
// Status: Active
// Priority: normal
//
// Target: https://example.com
// Security: Enabled
//
// Configuration:
//   {
//     "retries": 3,
//     "stealth": true,
//     "timeout": 30
//   }
//
// Tags: recon, osint, passive
//
// Instructions:
// 1. Identify subdomains
// 2. Enumerate technologies
// 3. Find public documents
```

## Using with TemplateRenderer

```go
// Create a renderer with default functions
renderer := prompt.NewTemplateRenderer()

// Create a prompt
p := &prompt.Prompt{
    ID:       "my-prompt",
    Position: prompt.PositionSystem,
    Content:  "Hello {{ toUpper .Variables.name }}!",
}

// Create context
ctx := prompt.NewRenderContext()
ctx.Variables["name"] = "gibson"

// Render
result, err := renderer.Render(p, ctx)
// Result: "Hello GIBSON!"
```

## Adding Custom Functions

```go
// Create renderer
renderer := prompt.NewTemplateRenderer()

// Add custom function
renderer.RegisterFunc("reverse", func(s string) string {
    runes := []rune(s)
    for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
        runes[i], runes[j] = runes[j], runes[i]
    }
    return string(runes)
})

// Use in template
prompt := &prompt.Prompt{
    ID:       "test",
    Position: prompt.PositionSystem,
    Content:  "{{ reverse .Variables.word }}",
}

// Render will now have access to 'reverse' function
```

## Function Reference

### String Functions
- `toUpper(s string) string` - Convert to uppercase
- `toLower(s string) string` - Convert to lowercase
- `title(s string) string` - Convert to title case
- `trim(s string) string` - Trim whitespace
- `trimPrefix(prefix, s string) string` - Remove prefix
- `trimSuffix(suffix, s string) string` - Remove suffix
- `replace(old, new, s string) string` - Replace all occurrences
- `contains(s, substr string) bool` - Check if contains substring
- `hasPrefix(s, prefix string) bool` - Check if has prefix
- `hasSuffix(s, suffix string) bool` - Check if has suffix
- `split(sep, s string) []string` - Split string
- `join(sep string, items []string) string` - Join strings

### Utility Functions
- `default(def, val any) any` - Return val if not empty, else def
- `required(val any) (any, error)` - Return val or error if empty
- `coalesce(vals ...any) any` - Return first non-empty value
- `ternary(cond bool, trueVal, falseVal any) any` - Conditional value

### Date/Time Functions
- `now() time.Time` - Current time
- `formatDate(format string, t time.Time) string` - Format time

### JSON Functions
- `toJSON(v any) string` - Marshal to JSON
- `toPrettyJSON(v any) string` - Marshal with indentation
- `fromJSON(s string) any` - Unmarshal JSON

### Formatting Functions
- `indent(spaces int, s string) string` - Indent each line
- `nindent(spaces int, s string) string` - Newline + indent
- `quote(s string) string` - Wrap in double quotes
- `squote(s string) string` - Wrap in single quotes
