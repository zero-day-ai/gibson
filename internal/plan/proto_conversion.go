package plan

import (
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// mapToProtoMessage converts a map[string]any to a proto message of the specified type.
// It does this by marshaling the map to JSON, then unmarshaling to the proto message.
func mapToProtoMessage(input map[string]any, protoTypeName string) (proto.Message, error) {
	if input == nil {
		input = make(map[string]any)
	}

	// Create a new instance of the proto message type
	msg, err := createProtoMessage(protoTypeName)
	if err != nil {
		return nil, fmt.Errorf("failed to create proto message: %w", err)
	}

	// Marshal the map to JSON
	jsonData, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal input to JSON: %w", err)
	}

	// Unmarshal JSON to proto message using protojson
	unmarshaler := protojson.UnmarshalOptions{
		DiscardUnknown: false, // Strict mode - fail on unknown fields
		AllowPartial:   false, // Require all required fields
	}

	if err := unmarshaler.Unmarshal(jsonData, msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON to proto %s: %w", protoTypeName, err)
	}

	return msg, nil
}

// protoMessageToMap converts a proto message to a map[string]any.
// It does this by marshaling the proto to JSON, then unmarshaling to a map.
func protoMessageToMap(msg proto.Message) (map[string]any, error) {
	if msg == nil {
		return make(map[string]any), nil
	}

	// Marshal proto message to JSON using protojson
	marshaler := protojson.MarshalOptions{
		EmitUnpopulated: true,  // Include zero values in output
		UseProtoNames:   false, // Use JSON names (camelCase)
	}

	jsonData, err := marshaler.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal proto to JSON: %w", err)
	}

	// Unmarshal JSON to map
	var result map[string]any
	if err := json.Unmarshal(jsonData, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON to map: %w", err)
	}

	return result, nil
}

// createProtoMessage creates a new instance of a proto message given its full type name.
// The type name should be in the format "package.MessageName" (e.g., "tool.v1.Request").
func createProtoMessage(protoTypeName string) (proto.Message, error) {
	// Look up the message type in the global proto registry
	msgType, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(protoTypeName))
	if err != nil {
		return nil, fmt.Errorf("proto message type %s not found in registry: %w", protoTypeName, err)
	}

	// Create a new instance of the message
	msg := msgType.New().Interface()
	return msg, nil
}
