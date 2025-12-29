package prompt

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPosition_Order(t *testing.T) {
	tests := []struct {
		name     string
		position Position
		expected int
	}{
		{
			name:     "system_prefix order",
			position: PositionSystemPrefix,
			expected: 0,
		},
		{
			name:     "system order",
			position: PositionSystem,
			expected: 1,
		},
		{
			name:     "system_suffix order",
			position: PositionSystemSuffix,
			expected: 2,
		},
		{
			name:     "context order",
			position: PositionContext,
			expected: 3,
		},
		{
			name:     "tools order",
			position: PositionTools,
			expected: 4,
		},
		{
			name:     "plugins order",
			position: PositionPlugins,
			expected: 5,
		},
		{
			name:     "agents order",
			position: PositionAgents,
			expected: 6,
		},
		{
			name:     "constraints order",
			position: PositionConstraints,
			expected: 7,
		},
		{
			name:     "examples order",
			position: PositionExamples,
			expected: 8,
		},
		{
			name:     "user_prefix order",
			position: PositionUserPrefix,
			expected: 9,
		},
		{
			name:     "user order",
			position: PositionUser,
			expected: 10,
		},
		{
			name:     "user_suffix order",
			position: PositionUserSuffix,
			expected: 11,
		},
		{
			name:     "invalid position returns -1",
			position: Position("invalid"),
			expected: -1,
		},
		{
			name:     "empty position returns -1",
			position: Position(""),
			expected: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.position.Order()
			assert.Equal(t, tt.expected, result,
				"Position %s should have order %d, got %d",
				tt.position, tt.expected, result)
		})
	}
}

func TestPosition_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		position Position
		expected bool
	}{
		{
			name:     "system_prefix is valid",
			position: PositionSystemPrefix,
			expected: true,
		},
		{
			name:     "system is valid",
			position: PositionSystem,
			expected: true,
		},
		{
			name:     "system_suffix is valid",
			position: PositionSystemSuffix,
			expected: true,
		},
		{
			name:     "context is valid",
			position: PositionContext,
			expected: true,
		},
		{
			name:     "tools is valid",
			position: PositionTools,
			expected: true,
		},
		{
			name:     "plugins is valid",
			position: PositionPlugins,
			expected: true,
		},
		{
			name:     "agents is valid",
			position: PositionAgents,
			expected: true,
		},
		{
			name:     "constraints is valid",
			position: PositionConstraints,
			expected: true,
		},
		{
			name:     "examples is valid",
			position: PositionExamples,
			expected: true,
		},
		{
			name:     "user_prefix is valid",
			position: PositionUserPrefix,
			expected: true,
		},
		{
			name:     "user is valid",
			position: PositionUser,
			expected: true,
		},
		{
			name:     "user_suffix is valid",
			position: PositionUserSuffix,
			expected: true,
		},
		{
			name:     "invalid position is not valid",
			position: Position("invalid"),
			expected: false,
		},
		{
			name:     "empty position is not valid",
			position: Position(""),
			expected: false,
		},
		{
			name:     "random string is not valid",
			position: Position("random_position"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.position.IsValid()
			assert.Equal(t, tt.expected, result,
				"Position %s validity should be %v, got %v",
				tt.position, tt.expected, result)
		})
	}
}

func TestAllPositions(t *testing.T) {
	t.Run("returns all positions in correct order", func(t *testing.T) {
		positions := AllPositions()

		// Check we have exactly 12 positions
		require.Len(t, positions, 12, "Should have exactly 12 positions")

		// Check positions are in the correct order
		expected := []Position{
			PositionSystemPrefix,
			PositionSystem,
			PositionSystemSuffix,
			PositionContext,
			PositionTools,
			PositionPlugins,
			PositionAgents,
			PositionConstraints,
			PositionExamples,
			PositionUserPrefix,
			PositionUser,
			PositionUserSuffix,
		}

		assert.Equal(t, expected, positions,
			"Positions should be in the correct sequential order")
	})

	t.Run("all positions are valid", func(t *testing.T) {
		positions := AllPositions()

		for i, pos := range positions {
			assert.True(t, pos.IsValid(),
				"Position at index %d (%s) should be valid", i, pos)
		}
	})

	t.Run("all positions have correct order", func(t *testing.T) {
		positions := AllPositions()

		for i, pos := range positions {
			assert.Equal(t, i, pos.Order(),
				"Position %s at index %d should have order %d, got %d",
				pos, i, i, pos.Order())
		}
	})
}

func TestPosition_OrderSequence(t *testing.T) {
	t.Run("positions are strictly sequential", func(t *testing.T) {
		positions := AllPositions()

		for i := 0; i < len(positions)-1; i++ {
			current := positions[i]
			next := positions[i+1]

			assert.Equal(t, current.Order()+1, next.Order(),
				"Position %s (order %d) should be followed by position %s (order %d)",
				current, current.Order(), next, next.Order())
		}
	})

	t.Run("first position has order 0", func(t *testing.T) {
		positions := AllPositions()
		assert.Equal(t, 0, positions[0].Order(),
			"First position should have order 0")
	})

	t.Run("last position has order 11", func(t *testing.T) {
		positions := AllPositions()
		assert.Equal(t, 11, positions[len(positions)-1].Order(),
			"Last position should have order 11")
	})
}

func TestPosition_Constants(t *testing.T) {
	t.Run("position constants have correct string values", func(t *testing.T) {
		tests := []struct {
			position Position
			expected string
		}{
			{PositionSystemPrefix, "system_prefix"},
			{PositionSystem, "system"},
			{PositionSystemSuffix, "system_suffix"},
			{PositionContext, "context"},
			{PositionTools, "tools"},
			{PositionPlugins, "plugins"},
			{PositionAgents, "agents"},
			{PositionConstraints, "constraints"},
			{PositionExamples, "examples"},
			{PositionUserPrefix, "user_prefix"},
			{PositionUser, "user"},
			{PositionUserSuffix, "user_suffix"},
		}

		for _, tt := range tests {
			assert.Equal(t, tt.expected, string(tt.position),
				"Position constant should have string value %s", tt.expected)
		}
	})
}

func TestPosition_Comparison(t *testing.T) {
	t.Run("can compare positions using Order", func(t *testing.T) {
		// System positions should come before user positions
		assert.Less(t, PositionSystem.Order(), PositionUser.Order(),
			"System position should come before user position")

		// Context should come after system
		assert.Greater(t, PositionContext.Order(), PositionSystem.Order(),
			"Context position should come after system position")

		// Tools should come before constraints
		assert.Less(t, PositionTools.Order(), PositionConstraints.Order(),
			"Tools position should come before constraints position")
	})

	t.Run("system prefix is first", func(t *testing.T) {
		positions := AllPositions()
		for _, pos := range positions {
			if pos != PositionSystemPrefix {
				assert.Less(t, PositionSystemPrefix.Order(), pos.Order(),
					"SystemPrefix should come before %s", pos)
			}
		}
	})

	t.Run("user suffix is last", func(t *testing.T) {
		positions := AllPositions()
		for _, pos := range positions {
			if pos != PositionUserSuffix {
				assert.Greater(t, PositionUserSuffix.Order(), pos.Order(),
					"UserSuffix should come after %s", pos)
			}
		}
	})
}
