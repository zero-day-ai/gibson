package prompt

import (
	"context"
	"sort"
	"strings"
)

// AssembleOptions configures the assembly process.
// It controls which prompts are included and how they are rendered.
type AssembleOptions struct {
	// Registry is the source for registered prompts
	Registry PromptRegistry

	// RenderContext provides data for template rendering
	RenderContext *RenderContext

	// IncludeTools determines whether to include tool prompts
	IncludeTools bool

	// IncludePlugins determines whether to include plugin prompts
	IncludePlugins bool

	// IncludeAgents determines whether to include sub-agent prompts
	IncludeAgents bool

	// SystemOverride replaces all system prompts with this content
	// When set, all system positions (system_prefix, system, system_suffix, etc.) are ignored
	SystemOverride string

	// Persona is the persona ID to include
	// Prompts with IDs matching "persona:{Persona}" will be included
	Persona string

	// ExtraPrompts are additional prompts to include in the assembly
	ExtraPrompts []Prompt
}

// Assembler composes prompts into LLM input.
// It collects prompts from the registry, filters by conditions, renders templates,
// and assembles them into the final message format for LLM consumption.
type Assembler interface {
	// Assemble collects and orders prompts for LLM consumption.
	// It applies the following algorithm:
	//   1. Collect all prompts from registry into a map (deduplicated by ID)
	//   2. Add ExtraPrompts (overrides prompts with same ID)
	//   3. If Persona is set, ensure prompt with ID "persona:{Persona}" is included
	//   4. For each prompt:
	//      - Evaluate conditions (skip if any condition returns false)
	//      - Render template with RenderContext
	//   5. Group prompts by position, sort each group by Priority (higher = first)
	//   6. If SystemOverride is set, use it instead of system positions
	//   7. Concatenate positions with "\n\n" separator, trimming whitespace:
	//      - System = system_prefix + system + system_suffix + context + tools* + plugins* + agents* + constraints + examples
	//        (* = only if corresponding Include flag is true)
	//      - User = user_prefix + user + user_suffix
	//   8. Build Messages array with {role: "system", content: System} and {role: "user", content: User}
	//      (empty messages are excluded)
	Assemble(ctx context.Context, opts AssembleOptions) (*AssembleResult, error)
}

// DefaultAssembler implements the Assembler interface.
// It uses a TemplateRenderer for rendering prompt templates.
type DefaultAssembler struct {
	renderer TemplateRenderer
}

// NewAssembler creates a new DefaultAssembler with the given template renderer.
func NewAssembler(renderer TemplateRenderer) Assembler {
	return &DefaultAssembler{
		renderer: renderer,
	}
}

// Assemble collects and orders prompts for LLM consumption.
func (a *DefaultAssembler) Assemble(ctx context.Context, opts AssembleOptions) (*AssembleResult, error) {
	// Step 1 & 2: Collect all prompts from registry and add extra prompts
	// Use a map to track unique prompts by ID to avoid duplicates
	promptMap := make(map[string]Prompt)

	if opts.Registry != nil {
		for _, p := range opts.Registry.List() {
			promptMap[p.ID] = p
		}
	}

	// Step 2: Add ExtraPrompts (will override registry prompts with same ID)
	for _, p := range opts.ExtraPrompts {
		promptMap[p.ID] = p
	}

	// Step 3: If Persona is set, ensure persona prompt is included
	if opts.Persona != "" && opts.Registry != nil {
		personaID := "persona:" + opts.Persona
		if personaPrompt, err := opts.Registry.Get(personaID); err == nil {
			promptMap[personaID] = *personaPrompt
		}
	}

	// Convert map back to slice
	allPrompts := make([]Prompt, 0, len(promptMap))
	for _, p := range promptMap {
		allPrompts = append(allPrompts, p)
	}

	// Step 4: Filter and render prompts
	var processedPrompts []Prompt
	renderCtx := opts.RenderContext
	if renderCtx == nil {
		renderCtx = NewRenderContext()
	}

	for _, prompt := range allPrompts {
		// Evaluate conditions
		if !a.evaluateConditions(prompt, renderCtx) {
			continue
		}

		// Render template
		rendered, err := a.renderer.Render(&prompt, renderCtx)
		if err != nil {
			return nil, err
		}

		// Create a copy with rendered content
		renderedPrompt := prompt
		renderedPrompt.Content = rendered
		processedPrompts = append(processedPrompts, renderedPrompt)
	}

	// Step 5: Group by position and sort by priority
	grouped := a.groupByPosition(processedPrompts)

	// Step 6 & 7: Build system and user messages
	var systemContent string
	var userContent string

	if opts.SystemOverride != "" {
		// Use override instead of system positions
		systemContent = opts.SystemOverride
	} else {
		// Build system message from all system positions
		systemParts := []string{}

		// System positions in order
		systemPositions := []Position{
			PositionSystemPrefix,
			PositionSystem,
			PositionSystemSuffix,
			PositionContext,
			PositionTools,
			PositionPlugins,
			PositionAgents,
			PositionConstraints,
			PositionExamples,
		}

		for _, pos := range systemPositions {
			// Skip certain positions based on options
			if pos == PositionTools && !opts.IncludeTools {
				continue
			}
			if pos == PositionPlugins && !opts.IncludePlugins {
				continue
			}
			if pos == PositionAgents && !opts.IncludeAgents {
				continue
			}

			if prompts, exists := grouped[pos]; exists {
				for _, p := range prompts {
					if trimmed := strings.TrimSpace(p.Content); trimmed != "" {
						systemParts = append(systemParts, trimmed)
					}
				}
			}
		}

		systemContent = strings.Join(systemParts, "\n\n")
	}

	// Build user message from user positions
	userParts := []string{}
	userPositions := []Position{
		PositionUserPrefix,
		PositionUser,
		PositionUserSuffix,
	}

	for _, pos := range userPositions {
		if prompts, exists := grouped[pos]; exists {
			for _, p := range prompts {
				if trimmed := strings.TrimSpace(p.Content); trimmed != "" {
					userParts = append(userParts, trimmed)
				}
			}
		}
	}

	userContent = strings.Join(userParts, "\n\n")

	// Step 8: Build Messages array
	messages := []Message{}
	if systemContent != "" {
		messages = append(messages, Message{
			Role:    "system",
			Content: systemContent,
		})
	}
	if userContent != "" {
		messages = append(messages, Message{
			Role:    "user",
			Content: userContent,
		})
	}

	return &AssembleResult{
		System:   systemContent,
		User:     userContent,
		Messages: messages,
		Prompts:  processedPrompts,
	}, nil
}

// evaluateConditions checks if all conditions for a prompt are satisfied.
// Returns true if all conditions pass or if there are no conditions.
func (a *DefaultAssembler) evaluateConditions(prompt Prompt, ctx *RenderContext) bool {
	if len(prompt.Conditions) == 0 {
		return true
	}

	// Convert RenderContext to flat map for condition evaluation
	flatCtx := map[string]any{
		"mission":   ctx.Mission,
		"target":    ctx.Target,
		"agent":     ctx.Agent,
		"memory":    ctx.Memory,
		"variables": ctx.Variables,
		"custom":    ctx.Custom,
	}

	for _, condition := range prompt.Conditions {
		result, err := condition.Evaluate(flatCtx)
		if err != nil || !result {
			return false
		}
	}

	return true
}

// groupByPosition groups prompts by their position and sorts each group by priority.
// Within each position, prompts with higher priority appear first.
func (a *DefaultAssembler) groupByPosition(prompts []Prompt) map[Position][]Prompt {
	grouped := make(map[Position][]Prompt)

	for _, p := range prompts {
		grouped[p.Position] = append(grouped[p.Position], p)
	}

	// Sort each group by priority (higher priority first)
	for pos := range grouped {
		sort.Slice(grouped[pos], func(i, j int) bool {
			return grouped[pos][i].Priority > grouped[pos][j].Priority
		})
	}

	return grouped
}
