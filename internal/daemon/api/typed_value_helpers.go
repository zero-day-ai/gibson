package api

import (
	"encoding/json"
	"fmt"

	sdkproto "github.com/zero-day-ai/sdk/api/gen/proto"
)

// StringToTypedMap converts a JSON string to TypedMap (for backward compatibility with events).
// This is used when converting internal event Data (string) to proto TypedMap.
func StringToTypedMap(jsonStr string) *sdkproto.TypedMap {
	if jsonStr == "" {
		return nil
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return nil
	}
	return mapToTypedMap(data)
}

// MapToTypedMap converts map[string]any to *TypedMap.
func MapToTypedMap(m map[string]any) *sdkproto.TypedMap {
	if m == nil {
		return nil
	}
	entries := make(map[string]*sdkproto.TypedValue)
	for k, v := range m {
		entries[k] = AnyToTypedValue(v)
	}
	return &sdkproto.TypedMap{Entries: entries}
}

// mapToTypedMap is the internal unexported version for backward compatibility.
func mapToTypedMap(m map[string]any) *sdkproto.TypedMap {
	return MapToTypedMap(m)
}

// AnyToTypedValue converts any Go value to TypedValue.
func AnyToTypedValue(v any) *sdkproto.TypedValue {
	if v == nil {
		return &sdkproto.TypedValue{Kind: &sdkproto.TypedValue_NullValue{}}
	}
	switch val := v.(type) {
	case string:
		return &sdkproto.TypedValue{Kind: &sdkproto.TypedValue_StringValue{StringValue: val}}
	case float64:
		return &sdkproto.TypedValue{Kind: &sdkproto.TypedValue_DoubleValue{DoubleValue: val}}
	case bool:
		return &sdkproto.TypedValue{Kind: &sdkproto.TypedValue_BoolValue{BoolValue: val}}
	case int:
		return &sdkproto.TypedValue{Kind: &sdkproto.TypedValue_IntValue{IntValue: int64(val)}}
	case int64:
		return &sdkproto.TypedValue{Kind: &sdkproto.TypedValue_IntValue{IntValue: val}}
	case []any:
		items := make([]*sdkproto.TypedValue, len(val))
		for i, item := range val {
			items[i] = AnyToTypedValue(item)
		}
		return &sdkproto.TypedValue{Kind: &sdkproto.TypedValue_ArrayValue{ArrayValue: &sdkproto.TypedArray{Items: items}}}
	case map[string]any:
		return &sdkproto.TypedValue{Kind: &sdkproto.TypedValue_MapValue{MapValue: MapToTypedMap(val)}}
	default:
		// Fallback: convert to string
		return &sdkproto.TypedValue{Kind: &sdkproto.TypedValue_StringValue{StringValue: fmt.Sprintf("%v", v)}}
	}
}

// anyToTypedValue is the internal unexported version for backward compatibility.
func anyToTypedValue(v any) *sdkproto.TypedValue {
	return AnyToTypedValue(v)
}

// TypedMapToMap converts TypedMap back to map[string]any.
func TypedMapToMap(tm *sdkproto.TypedMap) map[string]any {
	if tm == nil {
		return nil
	}
	result := make(map[string]any)
	for k, v := range tm.Entries {
		result[k] = TypedValueToAny(v)
	}
	return result
}

// TypedValueToAny converts TypedValue to any.
func TypedValueToAny(tv *sdkproto.TypedValue) any {
	if tv == nil {
		return nil
	}
	switch v := tv.Kind.(type) {
	case *sdkproto.TypedValue_NullValue:
		return nil
	case *sdkproto.TypedValue_StringValue:
		return v.StringValue
	case *sdkproto.TypedValue_IntValue:
		return v.IntValue
	case *sdkproto.TypedValue_DoubleValue:
		return v.DoubleValue
	case *sdkproto.TypedValue_BoolValue:
		return v.BoolValue
	case *sdkproto.TypedValue_BytesValue:
		return v.BytesValue
	case *sdkproto.TypedValue_ArrayValue:
		if v.ArrayValue == nil {
			return nil
		}
		result := make([]any, len(v.ArrayValue.Items))
		for i, item := range v.ArrayValue.Items {
			result[i] = TypedValueToAny(item)
		}
		return result
	case *sdkproto.TypedValue_MapValue:
		return TypedMapToMap(v.MapValue)
	default:
		return nil
	}
}
