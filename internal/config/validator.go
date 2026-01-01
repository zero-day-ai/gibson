package config

import (
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
)

// ConfigValidator validates configuration values.
type ConfigValidator interface {
	Validate(cfg *Config) error
}

// validatorImpl implements ConfigValidator using go-playground/validator.
type validatorImpl struct {
	validate *validator.Validate
}

// NewValidator creates a new ConfigValidator instance.
func NewValidator() ConfigValidator {
	return &validatorImpl{
		validate: validator.New(),
	}
}

// Validate validates the configuration and returns detailed error messages.
func (v *validatorImpl) Validate(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("configuration is nil")
	}

	// Perform struct tag validation first
	err := v.validate.Struct(cfg)
	if err != nil {
		// Convert validation errors to detailed messages
		validationErrs, ok := err.(validator.ValidationErrors)
		if !ok {
			return fmt.Errorf("validation error: %w", err)
		}

		var errorMessages []string
		for _, e := range validationErrs {
			errorMessages = append(errorMessages, formatValidationError(e))
		}

		return fmt.Errorf("configuration validation failed:\n  - %s", strings.Join(errorMessages, "\n  - "))
	}

	// Custom validation for RegistrationConfig
	if cfg.Registration.Enabled {
		if cfg.Registration.Port < 1024 || cfg.Registration.Port > 65535 {
			return fmt.Errorf("configuration validation failed:\n  - registration.port must be between 1024 and 65535 when enabled (got: %d)", cfg.Registration.Port)
		}
	}

	// Custom validation for RemoteComponentConfig addresses
	if err := v.validateRemoteComponents(cfg); err != nil {
		return err
	}

	return nil
}

// validateRemoteComponents validates remote component configurations
func (v *validatorImpl) validateRemoteComponents(cfg *Config) error {
	var errorMessages []string

	// Validate remote agents
	for name, agentCfg := range cfg.RemoteAgents {
		if agentCfg.Address == "" {
			errorMessages = append(errorMessages, fmt.Sprintf("remote_agents.%s.address is required", name))
		}
		if agentCfg.HealthCheck != "" && agentCfg.HealthCheck != "grpc" && agentCfg.HealthCheck != "http" {
			errorMessages = append(errorMessages, fmt.Sprintf("remote_agents.%s.health_check must be 'grpc' or 'http' (got: %s)", name, agentCfg.HealthCheck))
		}
	}

	// Validate remote tools
	for name, toolCfg := range cfg.RemoteTools {
		if toolCfg.Address == "" {
			errorMessages = append(errorMessages, fmt.Sprintf("remote_tools.%s.address is required", name))
		}
		if toolCfg.HealthCheck != "" && toolCfg.HealthCheck != "grpc" && toolCfg.HealthCheck != "http" {
			errorMessages = append(errorMessages, fmt.Sprintf("remote_tools.%s.health_check must be 'grpc' or 'http' (got: %s)", name, toolCfg.HealthCheck))
		}
	}

	// Validate remote plugins
	for name, pluginCfg := range cfg.RemotePlugins {
		if pluginCfg.Address == "" {
			errorMessages = append(errorMessages, fmt.Sprintf("remote_plugins.%s.address is required", name))
		}
		if pluginCfg.HealthCheck != "" && pluginCfg.HealthCheck != "grpc" && pluginCfg.HealthCheck != "http" {
			errorMessages = append(errorMessages, fmt.Sprintf("remote_plugins.%s.health_check must be 'grpc' or 'http' (got: %s)", name, pluginCfg.HealthCheck))
		}
	}

	if len(errorMessages) > 0 {
		return fmt.Errorf("configuration validation failed:\n  - %s", strings.Join(errorMessages, "\n  - "))
	}

	return nil
}

// formatValidationError formats a single validation error with field path and details.
func formatValidationError(e validator.FieldError) string {
	fieldPath := formatFieldPath(e.Namespace())

	switch e.Tag() {
	case "required":
		return fmt.Sprintf("%s is required", fieldPath)
	case "min":
		return fmt.Sprintf("%s must be at least %s (got: %v)", fieldPath, e.Param(), e.Value())
	case "max":
		return fmt.Sprintf("%s must be at most %s (got: %v)", fieldPath, e.Param(), e.Value())
	case "oneof":
		return fmt.Sprintf("%s must be one of [%s] (got: %v)", fieldPath, e.Param(), e.Value())
	case "url":
		return fmt.Sprintf("%s must be a valid URL (got: %v)", fieldPath, e.Value())
	case "filepath":
		return fmt.Sprintf("%s must be a valid file path (got: %v)", fieldPath, e.Value())
	default:
		return fmt.Sprintf("%s failed validation '%s' (got: %v)", fieldPath, e.Tag(), e.Value())
	}
}

// formatFieldPath converts validator namespace to a more readable field path.
// Example: "Config.Core.ParallelLimit" -> "core.parallel_limit"
func formatFieldPath(namespace string) string {
	// Remove the root struct name
	parts := strings.Split(namespace, ".")
	if len(parts) <= 1 {
		return namespace
	}

	// Skip the first part (struct name) and convert to lowercase with underscores
	result := make([]string, 0, len(parts)-1)
	for i := 1; i < len(parts); i++ {
		result = append(result, camelToSnake(parts[i]))
	}

	return strings.Join(result, ".")
}

// camelToSnake converts CamelCase to snake_case.
func camelToSnake(s string) string {
	var result strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteRune('_')
		}
		result.WriteRune(r)
	}
	return strings.ToLower(result.String())
}
