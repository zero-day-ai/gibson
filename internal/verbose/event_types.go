package verbose

import (
	"time"

	"github.com/zero-day-ai/gibson/internal/types"
)

// VerboseEventType identifies the type of verbose event.
type VerboseEventType string

// LLM event types
const (
	EventLLMRequestStarted   VerboseEventType = "llm.request.started"
	EventLLMRequestCompleted VerboseEventType = "llm.request.completed"
	EventLLMRequestFailed    VerboseEventType = "llm.request.failed"
	EventLLMStreamStarted    VerboseEventType = "llm.stream.started"
	EventLLMStreamChunk      VerboseEventType = "llm.stream.chunk"
	EventLLMStreamCompleted  VerboseEventType = "llm.stream.completed"
)

// Tool event types
const (
	EventToolCallStarted   VerboseEventType = "tool.call.started"
	EventToolCallCompleted VerboseEventType = "tool.call.completed"
	EventToolCallFailed    VerboseEventType = "tool.call.failed"
	EventToolNotFound      VerboseEventType = "tool.not_found"
)

// Plugin event types
const (
	EventPluginQueryStarted   VerboseEventType = "plugin.query.started"
	EventPluginQueryCompleted VerboseEventType = "plugin.query.completed"
	EventPluginQueryFailed    VerboseEventType = "plugin.query.failed"
)

// Agent event types
const (
	EventAgentStarted     VerboseEventType = "agent.started"
	EventAgentCompleted   VerboseEventType = "agent.completed"
	EventAgentFailed      VerboseEventType = "agent.failed"
	EventAgentDelegated   VerboseEventType = "agent.delegated"
	EventFindingSubmitted VerboseEventType = "agent.finding_submitted"
)

// Mission event types
const (
	EventMissionStarted   VerboseEventType = "mission.started"
	EventMissionProgress  VerboseEventType = "mission.progress"
	EventMissionNode      VerboseEventType = "mission.node"
	EventMissionCompleted VerboseEventType = "mission.completed"
	EventMissionFailed    VerboseEventType = "mission.failed"
)

// Memory event types
const (
	EventMemoryGet    VerboseEventType = "memory.get"
	EventMemorySet    VerboseEventType = "memory.set"
	EventMemorySearch VerboseEventType = "memory.search"
)

// System event types
const (
	EventComponentRegistered VerboseEventType = "system.component_registered"
	EventComponentHealth     VerboseEventType = "system.component_health"
	EventDaemonStarted       VerboseEventType = "system.daemon_started"
)

// String returns the string representation of the event type.
func (t VerboseEventType) String() string {
	return string(t)
}

// VerboseLevel represents the verbosity level for events.
type VerboseLevel int

const (
	LevelNone        VerboseLevel = 0
	LevelVerbose     VerboseLevel = 1
	LevelVeryVerbose VerboseLevel = 2
	LevelDebug       VerboseLevel = 3
)

// String returns the string representation of the verbose level.
func (l VerboseLevel) String() string {
	switch l {
	case LevelNone:
		return "none"
	case LevelVerbose:
		return "verbose"
	case LevelVeryVerbose:
		return "very-verbose"
	case LevelDebug:
		return "debug"
	default:
		return "unknown"
	}
}

// VerboseEvent represents a verbose event with type-specific payload.
type VerboseEvent struct {
	Type      VerboseEventType `json:"type"`
	Level     VerboseLevel     `json:"level"`
	Timestamp time.Time        `json:"timestamp"`
	MissionID types.ID         `json:"mission_id,omitempty"`
	AgentName string           `json:"agent_name,omitempty"`
	Payload   any              `json:"payload,omitempty"`
}

// LLMRequestStartedData contains data for LLM request started events.
type LLMRequestStartedData struct {
	Provider      string  `json:"provider"`
	Model         string  `json:"model"`
	SlotName      string  `json:"slot_name"`
	MessageCount  int     `json:"message_count"`
	MaxTokens     int     `json:"max_tokens,omitempty"`
	Temperature   float64 `json:"temperature,omitempty"`
	Stream        bool    `json:"stream"`
	PromptPreview string  `json:"prompt_preview,omitempty"` // Debug level: truncated prompt content
	ToolCount     int     `json:"tool_count,omitempty"`     // Debug level: number of tools available
}

// LLMRequestCompletedData contains data for LLM request completed events.
type LLMRequestCompletedData struct {
	Provider        string        `json:"provider"`
	Model           string        `json:"model"`
	SlotName        string        `json:"slot_name"`
	Duration        time.Duration `json:"duration"`
	InputTokens     int           `json:"input_tokens"`
	OutputTokens    int           `json:"output_tokens"`
	StopReason      string        `json:"stop_reason,omitempty"`
	ResponseLength  int           `json:"response_length"`
	ResponsePreview string        `json:"response_preview,omitempty"` // Debug level: truncated response content
}

// LLMRequestFailedData contains data for LLM request failed events.
type LLMRequestFailedData struct {
	Provider     string        `json:"provider"`
	Model        string        `json:"model"`
	SlotName     string        `json:"slot_name"`
	Error        string        `json:"error"`
	Duration     time.Duration `json:"duration"`
	Retryable    bool          `json:"retryable"`
	ErrorDetails string        `json:"error_details,omitempty"` // Debug level: full error details
	RetryAttempt int           `json:"retry_attempt,omitempty"` // Debug level: which retry attempt failed
}

// LLMStreamStartedData contains data for LLM stream started events.
type LLMStreamStartedData struct {
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	SlotName     string `json:"slot_name"`
	MessageCount int    `json:"message_count"`
}

// LLMStreamChunkData contains data for LLM stream chunk events.
type LLMStreamChunkData struct {
	Provider      string `json:"provider"`
	ChunkIndex    int    `json:"chunk_index"`
	ContentDelta  string `json:"content_delta"`
	ContentLength int    `json:"content_length"`
}

// LLMStreamCompletedData contains data for LLM stream completed events.
type LLMStreamCompletedData struct {
	Provider           string        `json:"provider"`
	Model              string        `json:"model"`
	SlotName           string        `json:"slot_name"`
	Duration           time.Duration `json:"duration"`
	TotalChunks        int           `json:"total_chunks"`
	InputTokens        int           `json:"input_tokens"`
	OutputTokens       int           `json:"output_tokens"`
	FinalContentLength int           `json:"final_content_length"`
}

// ToolCallStartedData contains data for tool call started events.
type ToolCallStartedData struct {
	ToolName      string         `json:"tool_name"`
	Parameters    map[string]any `json:"parameters,omitempty"`
	ParameterSize int            `json:"parameter_size"`
}

// ToolCallCompletedData contains data for tool call completed events.
type ToolCallCompletedData struct {
	ToolName   string        `json:"tool_name"`
	Duration   time.Duration `json:"duration"`
	ResultSize int           `json:"result_size"`
	Success    bool          `json:"success"`
}

// ToolCallFailedData contains data for tool call failed events.
type ToolCallFailedData struct {
	ToolName string        `json:"tool_name"`
	Error    string        `json:"error"`
	Duration time.Duration `json:"duration"`
}

// ToolNotFoundData contains data for tool not found events.
type ToolNotFoundData struct {
	ToolName    string `json:"tool_name"`
	RequestedBy string `json:"requested_by,omitempty"`
}

// PluginQueryStartedData contains data for plugin query started events.
type PluginQueryStartedData struct {
	PluginName     string         `json:"plugin_name"`
	Method         string         `json:"method"`
	Parameters     map[string]any `json:"parameters,omitempty"`
	ParameterCount int            `json:"parameter_count"`
}

// PluginQueryCompletedData contains data for plugin query completed events.
type PluginQueryCompletedData struct {
	PluginName string        `json:"plugin_name"`
	Method     string        `json:"method"`
	Duration   time.Duration `json:"duration"`
	Success    bool          `json:"success"`
}

// PluginQueryFailedData contains data for plugin query failed events.
type PluginQueryFailedData struct {
	PluginName string        `json:"plugin_name"`
	Method     string        `json:"method"`
	Error      string        `json:"error"`
	Duration   time.Duration `json:"duration"`
}

// AgentStartedData contains data for agent started events.
type AgentStartedData struct {
	AgentName       string   `json:"agent_name"`
	TaskDescription string   `json:"task_description,omitempty"`
	TargetID        types.ID `json:"target_id,omitempty"`
}

// AgentCompletedData contains data for agent completed events.
type AgentCompletedData struct {
	AgentName    string        `json:"agent_name"`
	Duration     time.Duration `json:"duration"`
	FindingCount int           `json:"finding_count"`
	Success      bool          `json:"success"`
}

// AgentFailedData contains data for agent failed events.
type AgentFailedData struct {
	AgentName    string        `json:"agent_name"`
	Error        string        `json:"error"`
	Duration     time.Duration `json:"duration"`
	FindingCount int           `json:"finding_count"`
}

// AgentDelegatedData contains data for agent delegated events.
type AgentDelegatedData struct {
	FromAgent       string `json:"from_agent"`
	ToAgent         string `json:"to_agent"`
	TaskDescription string `json:"task_description,omitempty"`
}

// FindingSubmittedData contains data for finding submitted events.
type FindingSubmittedData struct {
	FindingID    types.ID `json:"finding_id"`
	Title        string   `json:"title"`
	Severity     string   `json:"severity"`
	AgentName    string   `json:"agent_name"`
	TechniqueIDs []string `json:"technique_ids,omitempty"`
}

// MissionStartedData contains data for mission started events.
type MissionStartedData struct {
	MissionID    types.ID `json:"mission_id"`
	WorkflowName string   `json:"workflow_name,omitempty"`
	TargetID     types.ID `json:"target_id,omitempty"`
	NodeCount    int      `json:"node_count"`
}

// MissionProgressData contains data for mission progress events.
type MissionProgressData struct {
	MissionID      types.ID `json:"mission_id"`
	CompletedNodes int      `json:"completed_nodes"`
	TotalNodes     int      `json:"total_nodes"`
	CurrentNode    string   `json:"current_node,omitempty"`
	Message        string   `json:"message,omitempty"`
}

// MissionNodeData contains data for mission node events.
type MissionNodeData struct {
	NodeID   string        `json:"node_id"`
	NodeType string        `json:"node_type"`
	Status   string        `json:"status"`
	Duration time.Duration `json:"duration,omitempty"`
	Error    string        `json:"error,omitempty"`
}

// MissionCompletedData contains data for mission completed events.
type MissionCompletedData struct {
	MissionID     types.ID      `json:"mission_id"`
	Duration      time.Duration `json:"duration"`
	FindingCount  int           `json:"finding_count"`
	NodesExecuted int           `json:"nodes_executed"`
	Success       bool          `json:"success"`
}

// MissionFailedData contains data for mission failed events.
type MissionFailedData struct {
	MissionID     types.ID      `json:"mission_id"`
	Error         string        `json:"error"`
	Duration      time.Duration `json:"duration"`
	FindingCount  int           `json:"finding_count"`
	NodesExecuted int           `json:"nodes_executed"`
}

// MemoryGetData contains data for memory get events.
type MemoryGetData struct {
	Tier  string `json:"tier"`
	Key   string `json:"key"`
	Found bool   `json:"found"`
}

// MemorySetData contains data for memory set events.
type MemorySetData struct {
	Tier      string `json:"tier"`
	Key       string `json:"key"`
	ValueSize int    `json:"value_size"`
}

// MemorySearchData contains data for memory search events.
type MemorySearchData struct {
	Tier        string        `json:"tier"`
	Query       string        `json:"query"`
	ResultCount int           `json:"result_count"`
	Duration    time.Duration `json:"duration"`
}

// ComponentRegisteredData contains data for component registered events.
type ComponentRegisteredData struct {
	ComponentType string   `json:"component_type"`
	ComponentName string   `json:"component_name"`
	Version       string   `json:"version,omitempty"`
	Capabilities  []string `json:"capabilities,omitempty"`
}

// ComponentHealthData contains data for component health events.
type ComponentHealthData struct {
	ComponentType string        `json:"component_type"`
	ComponentName string        `json:"component_name"`
	Healthy       bool          `json:"healthy"`
	Status        string        `json:"status,omitempty"`
	ResponseTime  time.Duration `json:"response_time,omitempty"`
}

// DaemonStartedData contains data for daemon started events.
type DaemonStartedData struct {
	Version       string `json:"version"`
	ConfigPath    string `json:"config_path,omitempty"`
	DataDir       string `json:"data_dir,omitempty"`
	ListenAddress string `json:"listen_address,omitempty"`
}

// NewVerboseEvent creates a new verbose event with the current timestamp.
func NewVerboseEvent(eventType VerboseEventType, level VerboseLevel, payload any) VerboseEvent {
	return VerboseEvent{
		Type:      eventType,
		Level:     level,
		Timestamp: time.Now(),
		Payload:   payload,
	}
}

// WithMissionID sets the mission ID on the event.
func (e VerboseEvent) WithMissionID(missionID types.ID) VerboseEvent {
	e.MissionID = missionID
	return e
}

// WithAgentName sets the agent name on the event.
func (e VerboseEvent) WithAgentName(agentName string) VerboseEvent {
	e.AgentName = agentName
	return e
}
