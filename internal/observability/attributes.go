package observability

import (
	"fmt"

	"github.com/zero-day-ai/gibson/internal/agent"
	"github.com/zero-day-ai/gibson/internal/harness"
	"github.com/zero-day-ai/gibson/internal/types"
	"go.opentelemetry.io/otel/attribute"
)

// Gibson-specific attribute keys for observability
const (
	// GibsonAgentName is the name of the Gibson agent
	GibsonAgentName = "gibson.agent.name"

	// GibsonAgentVersion is the version of the agent
	GibsonAgentVersion = "gibson.agent.version"

	// GibsonMissionID is the unique identifier for the mission
	GibsonMissionID = "gibson.mission.id"

	// GibsonMissionName is the name of the mission
	GibsonMissionName = "gibson.mission.name"

	// GibsonTurnNumber is the turn number in the agent's execution
	GibsonTurnNumber = "gibson.turn.number"

	// GibsonToolName is the name of the tool being used
	GibsonToolName = "gibson.tool.name"

	// GibsonPluginName is the name of the plugin being called
	GibsonPluginName = "gibson.plugin.name"

	// GibsonPluginMethod is the method being called on the plugin
	GibsonPluginMethod = "gibson.plugin.method"

	// GibsonDelegationTarget is the target agent for delegation
	GibsonDelegationTarget = "gibson.delegation.target_agent"

	// GibsonDelegationTaskID is the task ID for delegation
	GibsonDelegationTaskID = "gibson.delegation.task_id"

	// GibsonFindingID is the unique identifier for a finding
	GibsonFindingID = "gibson.finding.id"

	// GibsonFindingSeverity is the severity level of a finding
	GibsonFindingSeverity = "gibson.finding.severity"

	// GibsonFindingCategory is the category of the finding
	GibsonFindingCategory = "gibson.finding.category"

	// GibsonLLMCost is the cost of LLM operations in USD
	GibsonLLMCost = "gibson.llm.cost"
)

// Gibson span name constants for various operations
const (
	// SpanAgentDelegate represents an agent delegation operation
	SpanAgentDelegate = "gibson.agent.delegate"

	// SpanFindingSubmit represents a finding submission operation
	SpanFindingSubmit = "gibson.finding.submit"

	// SpanPluginQuery represents a plugin query operation
	SpanPluginQuery = "gibson.plugin.query"

	// SpanMemoryGet represents a memory retrieval operation
	SpanMemoryGet = "gibson.memory.get"

	// SpanMemorySet represents a memory storage operation
	SpanMemorySet = "gibson.memory.set"

	// SpanMemorySearch represents a memory search operation
	SpanMemorySearch = "gibson.memory.search"
)

// MissionAttributes creates OpenTelemetry attributes from a MissionContext.
func MissionAttributes(mission harness.MissionContext) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String(GibsonMissionID, mission.ID.String()),
		attribute.String(GibsonMissionName, mission.Name),
	}

	if mission.CurrentAgent != "" {
		attrs = append(attrs, attribute.String(GibsonAgentName, mission.CurrentAgent))
	}

	if mission.Phase != "" {
		attrs = append(attrs, attribute.String("gibson.mission.phase", mission.Phase))
	}

	return attrs
}

// AgentAttributes creates OpenTelemetry attributes for an agent operation.
// It accepts the agent name and optional version.
func AgentAttributes(name string, version string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String(GibsonAgentName, name),
	}

	if version != "" {
		attrs = append(attrs, attribute.String(GibsonAgentVersion, version))
	}

	return attrs
}

// FindingAttributes creates OpenTelemetry attributes from a Finding.
func FindingAttributes(finding agent.Finding) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String(GibsonFindingID, finding.ID.String()),
		attribute.String(GibsonFindingSeverity, string(finding.Severity)),
	}

	if finding.Category != "" {
		attrs = append(attrs, attribute.String(GibsonFindingCategory, finding.Category))
	}

	if finding.TargetID != nil {
		attrs = append(attrs, attribute.String("gibson.finding.target_id", finding.TargetID.String()))
	}

	// Add confidence score
	attrs = append(attrs, attribute.Float64("gibson.finding.confidence", finding.Confidence))

	return attrs
}

// ToolAttributes creates OpenTelemetry attributes for a tool operation.
func ToolAttributes(toolName string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String(GibsonToolName, toolName),
	}
}

// PluginAttributes creates OpenTelemetry attributes for a plugin operation.
func PluginAttributes(pluginName, method string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String(GibsonPluginName, pluginName),
	}

	if method != "" {
		attrs = append(attrs, attribute.String(GibsonPluginMethod, method))
	}

	return attrs
}

// DelegationAttributes creates OpenTelemetry attributes for an agent delegation.
func DelegationAttributes(targetAgent string, taskID types.ID) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String(GibsonDelegationTarget, targetAgent),
		attribute.String(GibsonDelegationTaskID, taskID.String()),
	}
}

// TurnAttributes creates OpenTelemetry attributes for an agent turn.
func TurnAttributes(turnNumber int) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.Int(GibsonTurnNumber, turnNumber),
	}
}

// CostAttributes creates OpenTelemetry attributes for LLM cost tracking.
func CostAttributes(cost float64) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.Float64(GibsonLLMCost, cost),
	}
}

// TaskAttributes creates OpenTelemetry attributes for a task operation.
func TaskAttributes(task agent.Task) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("gibson.task.id", task.ID.String()),
		attribute.String("gibson.task.name", task.Name),
		attribute.Int("gibson.task.priority", task.Priority),
	}

	if task.MissionID != nil {
		attrs = append(attrs, attribute.String(GibsonMissionID, task.MissionID.String()))
	}

	if task.ParentTaskID != nil {
		attrs = append(attrs, attribute.String("gibson.task.parent_id", task.ParentTaskID.String()))
	}

	if task.TargetID != nil {
		attrs = append(attrs, attribute.String("gibson.task.target_id", task.TargetID.String()))
	}

	if len(task.Tags) > 0 {
		attrs = append(attrs, attribute.StringSlice("gibson.task.tags", task.Tags))
	}

	return attrs
}

// MetricsAttributes creates OpenTelemetry attributes from task metrics.
func MetricsAttributes(metrics agent.TaskMetrics) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.Int("gibson.metrics.llm_calls", metrics.LLMCalls),
		attribute.Int("gibson.metrics.tool_calls", metrics.ToolCalls),
		attribute.Int("gibson.metrics.plugin_calls", metrics.PluginCalls),
		attribute.Int("gibson.metrics.tokens_used", metrics.TokensUsed),
		attribute.Float64(GibsonLLMCost, metrics.Cost),
		attribute.Int("gibson.metrics.findings_count", metrics.FindingsCount),
		attribute.Int("gibson.metrics.errors", metrics.Errors),
		attribute.Int("gibson.metrics.retries", metrics.Retries),
		attribute.Int("gibson.metrics.sub_tasks", metrics.SubTasks),
	}

	if metrics.Duration > 0 {
		attrs = append(attrs, attribute.String("gibson.metrics.duration", metrics.Duration.String()))
	}

	return attrs
}

// ErrorAttributes creates OpenTelemetry attributes for error tracking.
func ErrorAttributes(err error, code string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.Bool("error", true),
		attribute.String("error.message", err.Error()),
	}

	if code != "" {
		attrs = append(attrs, attribute.String("error.code", code))
	}

	return attrs
}

// TargetAttributes creates OpenTelemetry attributes from TargetInfo.
func TargetAttributes(target harness.TargetInfo) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("gibson.target.id", target.ID.String()),
		attribute.String("gibson.target.name", target.Name),
		attribute.String("gibson.target.type", target.Type),
	}

	if target.Provider != "" {
		attrs = append(attrs, attribute.String("gibson.target.provider", target.Provider))
	}

	if target.URL != "" {
		// Sanitize URL to avoid logging credentials
		attrs = append(attrs, attribute.String("gibson.target.url", sanitizeURL(target.URL)))
	}

	return attrs
}

// sanitizeURL removes credentials from URLs for safe logging.
func sanitizeURL(urlStr string) string {
	// Basic sanitization - in production, use url.Parse and redact credentials
	// This is a placeholder implementation
	return urlStr
}

// CombineAttributes merges multiple attribute slices into one.
func CombineAttributes(attrSets ...[]attribute.KeyValue) []attribute.KeyValue {
	var totalLen int
	for _, attrs := range attrSets {
		totalLen += len(attrs)
	}

	combined := make([]attribute.KeyValue, 0, totalLen)
	for _, attrs := range attrSets {
		combined = append(combined, attrs...)
	}

	return combined
}

// AttributeSet is a builder for creating attribute sets.
type AttributeSet struct {
	attrs []attribute.KeyValue
}

// NewAttributeSet creates a new attribute set builder.
func NewAttributeSet() *AttributeSet {
	return &AttributeSet{
		attrs: make([]attribute.KeyValue, 0, 10),
	}
}

// Add adds attributes to the set.
func (a *AttributeSet) Add(attrs ...attribute.KeyValue) *AttributeSet {
	a.attrs = append(a.attrs, attrs...)
	return a
}

// AddString adds a string attribute.
func (a *AttributeSet) AddString(key, value string) *AttributeSet {
	if value != "" {
		a.attrs = append(a.attrs, attribute.String(key, value))
	}
	return a
}

// AddInt adds an int attribute.
func (a *AttributeSet) AddInt(key string, value int) *AttributeSet {
	a.attrs = append(a.attrs, attribute.Int(key, value))
	return a
}

// AddFloat64 adds a float64 attribute.
func (a *AttributeSet) AddFloat64(key string, value float64) *AttributeSet {
	a.attrs = append(a.attrs, attribute.Float64(key, value))
	return a
}

// AddBool adds a bool attribute.
func (a *AttributeSet) AddBool(key string, value bool) *AttributeSet {
	a.attrs = append(a.attrs, attribute.Bool(key, value))
	return a
}

// AddID adds a types.ID attribute.
func (a *AttributeSet) AddID(key string, id types.ID) *AttributeSet {
	a.attrs = append(a.attrs, attribute.String(key, id.String()))
	return a
}

// Build returns the final attribute slice.
func (a *AttributeSet) Build() []attribute.KeyValue {
	return a.attrs
}

// String returns a string representation of the attribute set.
func (a *AttributeSet) String() string {
	return fmt.Sprintf("AttributeSet{len=%d}", len(a.attrs))
}
