package memory

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewMemoryItem tests the creation of a new MemoryItem
func TestNewMemoryItem(t *testing.T) {
	key := "test-key"
	value := "test-value"
	metadata := map[string]any{"source": "test"}

	before := time.Now()
	item := NewMemoryItem(key, value, metadata)
	after := time.Now()

	assert.Equal(t, key, item.Key)
	assert.Equal(t, value, item.Value)
	assert.Equal(t, metadata, item.Metadata)
	assert.True(t, item.CreatedAt.After(before) || item.CreatedAt.Equal(before))
	assert.True(t, item.CreatedAt.Before(after) || item.CreatedAt.Equal(after))
	assert.Equal(t, item.CreatedAt, item.UpdatedAt)
}

// TestMemoryItem_Validate tests validation of MemoryItem
func TestMemoryItem_Validate(t *testing.T) {
	tests := []struct {
		name    string
		item    *MemoryItem
		wantErr bool
	}{
		{
			name:    "valid item",
			item:    NewMemoryItem("key", "value", nil),
			wantErr: false,
		},
		{
			name:    "empty key",
			item:    &MemoryItem{Key: "", Value: "value"},
			wantErr: true,
		},
		{
			name:    "valid with metadata",
			item:    NewMemoryItem("key", map[string]string{"data": "value"}, map[string]any{"source": "test"}),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.item.Validate()
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestMemoryItem_MarshalValue tests JSON marshaling of MemoryItem values
func TestMemoryItem_MarshalValue(t *testing.T) {
	tests := []struct {
		name    string
		value   any
		wantErr bool
	}{
		{
			name:    "string value",
			value:   "test-string",
			wantErr: false,
		},
		{
			name:    "int value",
			value:   42,
			wantErr: false,
		},
		{
			name:    "map value",
			value:   map[string]string{"key": "value"},
			wantErr: false,
		},
		{
			name:    "struct value",
			value:   struct{ Name string }{"test"},
			wantErr: false,
		},
		{
			name:    "nil value",
			value:   nil,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := NewMemoryItem("key", tt.value, nil)
			data, err := item.MarshalValue()

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, data)

				// Verify we can unmarshal it back
				var unmarshaled any
				err = json.Unmarshal(data, &unmarshaled)
				require.NoError(t, err)
			}
		})
	}
}

// TestMemoryItem_UnmarshalValue tests JSON unmarshaling into MemoryItem values
func TestMemoryItem_UnmarshalValue(t *testing.T) {
	// Create a test value
	originalValue := map[string]string{"key": "value", "foo": "bar"}
	item := NewMemoryItem("test", originalValue, nil)

	// Marshal it
	data, err := item.MarshalValue()
	require.NoError(t, err)

	// Unmarshal into the item's Value field directly
	err = item.UnmarshalValue(data)
	require.NoError(t, err)

	// Convert to map for comparison
	valueMap, ok := item.Value.(map[string]any)
	require.True(t, ok, "value should be a map")

	// Compare values
	assert.Equal(t, "value", valueMap["key"])
	assert.Equal(t, "bar", valueMap["foo"])
}

// TestNewMemoryResult tests the creation of a new MemoryResult
func TestNewMemoryResult(t *testing.T) {
	item := *NewMemoryItem("key", "value", nil)
	score := 0.95

	result := NewMemoryResult(item, score)

	assert.Equal(t, item, result.Item)
	assert.Equal(t, score, result.Score)
}

// TestJSON_Serialization tests that all types can be serialized to/from JSON
func TestJSON_Serialization(t *testing.T) {
	t.Run("MemoryItem", func(t *testing.T) {
		original := NewMemoryItem("key", map[string]string{"data": "value"}, map[string]any{"source": "test"})

		data, err := json.Marshal(original)
		require.NoError(t, err)

		var decoded MemoryItem
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.Equal(t, original.Key, decoded.Key)
		assert.Equal(t, original.CreatedAt.Unix(), decoded.CreatedAt.Unix())
	})

	t.Run("MemoryResult", func(t *testing.T) {
		item := *NewMemoryItem("key", "value", nil)
		original := NewMemoryResult(item, 0.85)

		data, err := json.Marshal(original)
		require.NoError(t, err)

		var decoded MemoryResult
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.Equal(t, original.Item.Key, decoded.Item.Key)
		assert.Equal(t, original.Score, decoded.Score)
	})
}
