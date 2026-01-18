package internal

import (
	"context"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/zero-day-ai/gibson/internal/database"
	"github.com/zero-day-ai/gibson/internal/mission"
	"github.com/zero-day-ai/gibson/internal/types"
)

// CompletionFunc is a Cobra ValidArgsFunction that returns completion suggestions
type CompletionFunc func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective)

// CompletionContext holds dependencies for completion functions
type CompletionContext struct {
	DB *database.DB
}

// NewCompletionContext creates a new completion context
// It attempts to load the database, but returns
// a minimal context on error to allow completions to work even if
// the system is not fully initialized
func NewCompletionContext() *CompletionContext {
	ctx := &CompletionContext{}

	// Try to open database
	homeDir, err := os.UserHomeDir()
	if err == nil {
		dbPath := filepath.Join(homeDir, ".gibson", "gibson.db")
		if db, err := database.Open(dbPath); err == nil {
			ctx.DB = db
		}
	}

	return ctx
}

// Close closes any open resources in the completion context
func (c *CompletionContext) Close() {
	if c.DB != nil {
		_ = c.DB.Close()
	}
}

// CompleteAgentNames returns completion suggestions for agent names
// Note: Component completions are currently disabled as they require
// access to etcd which is not available in shell completion contexts
func CompleteAgentNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return []string{}, cobra.ShellCompDirectiveNoFileComp
}

// CompleteToolNames returns completion suggestions for tool names
// Note: Component completions are currently disabled as they require
// access to etcd which is not available in shell completion contexts
func CompleteToolNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return []string{}, cobra.ShellCompDirectiveNoFileComp
}

// CompletePluginNames returns completion suggestions for plugin names
// Note: Component completions are currently disabled as they require
// access to etcd which is not available in shell completion contexts
func CompletePluginNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return []string{}, cobra.ShellCompDirectiveNoFileComp
}

// CompleteComponentNames returns completion suggestions for component names (all types)
// Note: Component completions are currently disabled as they require
// access to etcd which is not available in shell completion contexts
func CompleteComponentNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return []string{}, cobra.ShellCompDirectiveNoFileComp
}

// CompleteTargetNames returns completion suggestions for target names
func CompleteTargetNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	ctx := NewCompletionContext()
	defer ctx.Close()

	if ctx.DB == nil {
		return []string{}, cobra.ShellCompDirectiveNoFileComp
	}

	targetDAO := database.NewTargetDAO(ctx.DB)
	targets, err := targetDAO.List(context.Background(), nil)
	if err != nil {
		return []string{}, cobra.ShellCompDirectiveNoFileComp
	}

	names := make([]string, 0, len(targets))
	for _, target := range targets {
		names = append(names, target.Name)
	}

	return names, cobra.ShellCompDirectiveNoFileComp
}

// CompleteTargetIDs returns completion suggestions for target IDs
func CompleteTargetIDs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	ctx := NewCompletionContext()
	defer ctx.Close()

	if ctx.DB == nil {
		return []string{}, cobra.ShellCompDirectiveNoFileComp
	}

	targetDAO := database.NewTargetDAO(ctx.DB)
	targets, err := targetDAO.List(context.Background(), nil)
	if err != nil {
		return []string{}, cobra.ShellCompDirectiveNoFileComp
	}

	ids := make([]string, 0, len(targets))
	for _, target := range targets {
		ids = append(ids, target.ID.String())
	}

	return ids, cobra.ShellCompDirectiveNoFileComp
}

// CompleteCredentialNames returns completion suggestions for credential names
func CompleteCredentialNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	ctx := NewCompletionContext()
	defer ctx.Close()

	if ctx.DB == nil {
		return []string{}, cobra.ShellCompDirectiveNoFileComp
	}

	credDAO := database.NewCredentialDAO(ctx.DB)
	creds, err := credDAO.List(context.Background(), nil)
	if err != nil {
		return []string{}, cobra.ShellCompDirectiveNoFileComp
	}

	names := make([]string, 0, len(creds))
	for _, cred := range creds {
		names = append(names, cred.Name)
	}

	return names, cobra.ShellCompDirectiveNoFileComp
}

// CompleteCredentialIDs returns completion suggestions for credential IDs
func CompleteCredentialIDs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	ctx := NewCompletionContext()
	defer ctx.Close()

	if ctx.DB == nil {
		return []string{}, cobra.ShellCompDirectiveNoFileComp
	}

	credDAO := database.NewCredentialDAO(ctx.DB)
	creds, err := credDAO.List(context.Background(), nil)
	if err != nil {
		return []string{}, cobra.ShellCompDirectiveNoFileComp
	}

	ids := make([]string, 0, len(creds))
	for _, cred := range creds {
		ids = append(ids, cred.ID.String())
	}

	return ids, cobra.ShellCompDirectiveNoFileComp
}

// CompleteMissionNames returns completion suggestions for mission names
func CompleteMissionNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	ctx := NewCompletionContext()
	defer ctx.Close()

	if ctx.DB == nil {
		return []string{}, cobra.ShellCompDirectiveNoFileComp
	}

	missionStore := mission.NewDBMissionStore(ctx.DB)
	// List all missions (empty status filter)
	missions, err := missionStore.List(context.Background(), mission.NewMissionFilter())
	if err != nil {
		return []string{}, cobra.ShellCompDirectiveNoFileComp
	}

	names := make([]string, 0, len(missions))
	for _, m := range missions {
		names = append(names, m.Name)
	}

	return names, cobra.ShellCompDirectiveNoFileComp
}

// CompleteMissionIDs returns completion suggestions for mission IDs
func CompleteMissionIDs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	ctx := NewCompletionContext()
	defer ctx.Close()

	if ctx.DB == nil {
		return []string{}, cobra.ShellCompDirectiveNoFileComp
	}

	missionStore := mission.NewDBMissionStore(ctx.DB)
	// List all missions (empty status filter)
	missions, err := missionStore.List(context.Background(), mission.NewMissionFilter())
	if err != nil {
		return []string{}, cobra.ShellCompDirectiveNoFileComp
	}

	ids := make([]string, 0, len(missions))
	for _, m := range missions {
		ids = append(ids, m.ID.String())
	}

	return ids, cobra.ShellCompDirectiveNoFileComp
}

// CompleteOutputFormat returns completion suggestions for output format values
func CompleteOutputFormat(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	formats := []string{
		"text",
		"json",
		"sarif",
		"csv",
		"html",
	}
	return formats, cobra.ShellCompDirectiveNoFileComp
}

// CompleteProvider returns completion suggestions for provider values
func CompleteProvider(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	providers := []string{
		string(types.ProviderOpenAI),
		string(types.ProviderAnthropic),
		string(types.ProviderGoogle),
		string(types.ProviderAzure),
		string(types.ProviderOllama),
		string(types.ProviderCustom),
	}
	return providers, cobra.ShellCompDirectiveNoFileComp
}

// CompleteTargetType returns completion suggestions for target type values
func CompleteTargetType(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	targetTypes := []string{
		string(types.TargetTypeLLMChat),
		string(types.TargetTypeLLMAPI),
		string(types.TargetTypeRAG),
		string(types.TargetTypeAgent),
		string(types.TargetTypeEmbedding),
		string(types.TargetTypeMultimodal),
	}
	return targetTypes, cobra.ShellCompDirectiveNoFileComp
}

// CompleteTargetStatus returns completion suggestions for target status values
func CompleteTargetStatus(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	statuses := []string{
		string(types.TargetStatusActive),
		string(types.TargetStatusInactive),
		string(types.TargetStatusError),
	}
	return statuses, cobra.ShellCompDirectiveNoFileComp
}

// CompleteCredentialType returns completion suggestions for credential type values
func CompleteCredentialType(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	credTypes := []string{
		string(types.CredentialTypeAPIKey),
		string(types.CredentialTypeBearer),
		string(types.CredentialTypeBasic),
		string(types.CredentialTypeOAuth),
		string(types.CredentialTypeCustom),
	}
	return credTypes, cobra.ShellCompDirectiveNoFileComp
}

// CompleteMissionStatus returns completion suggestions for mission status values
func CompleteMissionStatus(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	statuses := []string{
		string(mission.MissionStatusPending),
		string(mission.MissionStatusRunning),
		string(mission.MissionStatusCompleted),
		string(mission.MissionStatusFailed),
		string(mission.MissionStatusCancelled),
	}
	return statuses, cobra.ShellCompDirectiveNoFileComp
}

// CompleteSeverity returns completion suggestions for finding severity values
func CompleteSeverity(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	severities := []string{
		"critical",
		"high",
		"medium",
		"low",
		"info",
	}
	return severities, cobra.ShellCompDirectiveNoFileComp
}

// CompleteYAMLFile returns completion for YAML files in the current directory
func CompleteYAMLFile(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return []string{"yaml", "yml"}, cobra.ShellCompDirectiveFilterFileExt
}

// NoCompletion returns no completion suggestions
func NoCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return []string{}, cobra.ShellCompDirectiveNoFileComp
}
