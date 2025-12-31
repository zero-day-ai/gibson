# Agent Streaming SDK Guide

This guide explains how to implement interactive streaming support in Gibson agents, enabling real-time observation and steering by operators.

## Table of Contents

1. [Overview](#overview)
2. [Protocol](#protocol)
3. [Message Types](#message-types)
4. [Implementing a Streaming Agent](#implementing-a-streaming-agent)
5. [Go Implementation](#go-implementation)
6. [Python Implementation](#python-implementation)
7. [Best Practices](#best-practices)
8. [Testing](#testing)
9. [Error Handling](#error-handling)
10. [Observability Integration](#observability-integration)

---

## Overview

Gibson supports bidirectional gRPC streaming for agents, enabling:

- **Real-time output streaming** to operators
- **Mid-execution steering messages** from operators
- **Interrupt/pause/resume capabilities**
- **Mode switching** between autonomous and interactive execution
- **Rich event types** for tool calls, findings, and status changes

### Why Use Streaming?

Traditional request-response patterns are limiting for long-running agent tasks:

| Request-Response | Streaming |
|-----------------|-----------|
| No progress visibility | Real-time output |
| No mid-execution control | Interactive steering |
| Binary success/failure | Rich event stream |
| Timeout management issues | Graceful interrupts |
| No operator interaction | Collaborative execution |

---

## Protocol

### StreamExecute RPC

The `StreamExecute` RPC enables bidirectional communication between Gibson and agents:

```protobuf
service AgentService {
    rpc StreamExecute(stream ClientMessage) returns (stream AgentMessage);
}
```

### Communication Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                     Gibson (Client)                              │
└───────────────┬─────────────────────────────────────────────────┘
                │
                │ (1) StartExecutionRequest
                ├──────────────────────────────────────────────────>
                │
                │                                    (2) StatusChange (RUNNING)
                │ <─────────────────────────────────────────────────┤
                │                                                    │
                │                                    (3) OutputChunk │
                │ <─────────────────────────────────────────────────┤
                │                                                    │
                │ (4) SteeringMessage                                │
                ├──────────────────────────────────────────────────> │
                │                                                    │
                │                                    (5) SteeringAck │
                │ <──────────────────────────────────────────────────┤
                │                                                    │
                │                                  (6) ToolCallEvent │
                │ <──────────────────────────────────────────────────┤
                │                                                    │
                │                                (7) ToolResultEvent │
                │ <──────────────────────────────────────────────────┤
                │                                                    │
                │                                   (8) FindingEvent │
                │ <──────────────────────────────────────────────────┤
                │                                                    │
                │                       (9) StatusChange (COMPLETED) │
                │ <──────────────────────────────────────────────────┤
                │
┌───────────────┴─────────────────────────────────────────────────┐
│                       Agent (Server)                             │
└─────────────────────────────────────────────────────────────────┘
```

---

## Message Types

### Client → Agent Messages

All client messages use the `ClientMessage` wrapper with a oneof payload:

```protobuf
message ClientMessage {
    oneof payload {
        StartExecutionRequest start = 1;
        SteeringMessage steering = 2;
        InterruptRequest interrupt = 3;
        SetModeRequest set_mode = 4;
        ResumeRequest resume = 5;
    }
}
```

#### StartExecutionRequest

Initiates task execution. **Must be the first message** sent.

```protobuf
message StartExecutionRequest {
    string task_json = 1;           // JSON-serialized Task object
    AgentMode initial_mode = 2;     // AUTONOMOUS or INTERACTIVE
}
```

**Example:**
```json
{
    "task_json": "{\"id\":\"task-123\",\"goal\":\"Test for prompt injection\"}",
    "initial_mode": "AGENT_MODE_AUTONOMOUS"
}
```

#### SteeringMessage

Send guidance or corrections to the agent during execution.

```protobuf
message SteeringMessage {
    string id = 1;                     // Unique message ID (for acknowledgment)
    string content = 2;                // Steering guidance text
    map<string, string> metadata = 3;  // Optional metadata
}
```

**Example:**
```json
{
    "id": "steer-001",
    "content": "Focus on testing the authentication endpoint instead",
    "metadata": {
        "priority": "high",
        "source": "operator"
    }
}
```

**Agent must acknowledge** steering messages with `SteeringAck`.

#### InterruptRequest

Pause execution immediately.

```protobuf
message InterruptRequest {
    string reason = 1;  // Human-readable reason for interrupt
}
```

**Example:**
```json
{
    "reason": "Agent is exploring unproductive path"
}
```

Agent should:
1. Stop current operation gracefully
2. Send `StatusChange` with `AGENT_STATUS_INTERRUPTED`
3. Clean up resources
4. Wait for `ResumeRequest` or stream closure

#### SetModeRequest

Switch between autonomous and interactive modes.

```protobuf
message SetModeRequest {
    AgentMode mode = 1;  // AUTONOMOUS or INTERACTIVE
}

enum AgentMode {
    AGENT_MODE_AUTONOMOUS = 0;   // Agent runs independently
    AGENT_MODE_INTERACTIVE = 1;  // Agent waits for operator input
}
```

**Autonomous mode**: Agent proceeds without waiting for input.
**Interactive mode**: Agent may pause and request guidance via `AGENT_STATUS_WAITING_FOR_INPUT`.

#### ResumeRequest

Continue execution after interrupt.

```protobuf
message ResumeRequest {
    string guidance = 1;  // Optional guidance for resumption
}
```

**Example:**
```json
{
    "guidance": "Continue with the original plan but be more conservative"
}
```

---

### Agent → Client Messages

All agent messages use the `AgentMessage` wrapper:

```protobuf
message AgentMessage {
    oneof payload {
        OutputChunk output = 1;
        ToolCallEvent tool_call = 2;
        ToolResultEvent tool_result = 3;
        FindingEvent finding = 4;
        StatusChange status = 5;
        SteeringAck steering_ack = 6;
        ErrorEvent error = 7;
    }

    // Observability fields (always set)
    string trace_id = 10;      // OpenTelemetry trace ID
    string span_id = 11;       // OpenTelemetry span ID
    int64 sequence = 12;       // Monotonically increasing sequence number
    int64 timestamp_ms = 13;   // Unix timestamp in milliseconds
}
```

#### OutputChunk

Stream text output to the operator.

```protobuf
message OutputChunk {
    string content = 1;      // Text content
    bool is_reasoning = 2;   // True if internal reasoning, false for action
}
```

**Use cases:**
- LLM responses
- Agent reasoning steps
- Progress updates
- Informational messages

**Example:**
```go
stream.Send(&proto.AgentMessage{
    Payload: &proto.AgentMessage_Output{
        Output: &proto.OutputChunk{
            Content: "Analyzing target endpoint for vulnerabilities...",
            IsReasoning: false,
        },
    },
    TraceId: ctx.Value("trace_id").(string),
    Sequence: seq.Next(),
    TimestampMs: time.Now().UnixMilli(),
})
```

#### ToolCallEvent

Notify when invoking a tool.

```protobuf
message ToolCallEvent {
    string tool_name = 1;   // Name of the tool
    string input_json = 2;  // JSON-serialized input
    string call_id = 3;     // Unique call ID (correlate with result)
}
```

**Example:**
```json
{
    "tool_name": "http-client",
    "input_json": "{\"url\":\"https://api.example.com\",\"method\":\"POST\"}",
    "call_id": "call-001"
}
```

#### ToolResultEvent

Report tool execution result.

```protobuf
message ToolResultEvent {
    string call_id = 1;     // Matches ToolCallEvent.call_id
    string output_json = 2; // JSON-serialized output
    bool success = 3;       // True if tool succeeded
}
```

**Example:**
```json
{
    "call_id": "call-001",
    "output_json": "{\"status_code\":200,\"body\":\"...\"}",
    "success": true
}
```

#### FindingEvent

Report a security finding.

```protobuf
message FindingEvent {
    string finding_json = 1;  // JSON-serialized Finding object
}
```

**Example Finding JSON:**
```json
{
    "id": "finding-123",
    "title": "Prompt Injection Vulnerability",
    "description": "System prompt disclosed via injection",
    "severity": "high",
    "confidence": 0.95,
    "category": "prompt_injection",
    "mitre_techniques": ["AML.T0051"],
    "evidence": [
        {
            "type": "http_request",
            "title": "Malicious Prompt",
            "content": "Ignore previous instructions..."
        }
    ]
}
```

#### StatusChange

Report agent status transitions.

```protobuf
message StatusChange {
    AgentStatus status = 1;  // New status
    string message = 2;      // Optional human-readable message
}

enum AgentStatus {
    AGENT_STATUS_RUNNING = 0;
    AGENT_STATUS_PAUSED = 1;
    AGENT_STATUS_WAITING_FOR_INPUT = 2;
    AGENT_STATUS_INTERRUPTED = 3;
    AGENT_STATUS_COMPLETED = 4;
    AGENT_STATUS_FAILED = 5;
}
```

**Example:**
```go
stream.Send(&proto.AgentMessage{
    Payload: &proto.AgentMessage_Status{
        Status: &proto.StatusChange{
            Status: proto.AgentStatus_AGENT_STATUS_RUNNING,
            Message: "Task execution started",
        },
    },
    // ... observability fields
})
```

#### SteeringAck

Acknowledge receipt of steering message.

```protobuf
message SteeringAck {
    string message_id = 1;  // Matches SteeringMessage.id
    string response = 2;    // Agent's response/acknowledgment
}
```

**Example:**
```json
{
    "message_id": "steer-001",
    "response": "Acknowledged. Switching focus to authentication endpoint."
}
```

#### ErrorEvent

Report errors during execution.

```protobuf
message ErrorEvent {
    string code = 1;      // Error code (e.g., "TOOL_FAILED", "LLM_ERROR")
    string message = 2;   // Human-readable error message
    bool fatal = 3;       // True if error terminates execution
}
```

**Example:**
```json
{
    "code": "LLM_TIMEOUT",
    "message": "LLM completion timed out after 30s",
    "fatal": false
}
```

---

## Implementing a Streaming Agent

### Key Requirements

1. **Always acknowledge steering messages** - Operators expect confirmation
2. **Graceful interrupt handling** - Clean up resources on interrupt
3. **Include trace context** - Set `trace_id` and `span_id` for observability
4. **Sequence all messages** - Use monotonically increasing sequence numbers
5. **Rate limit output** - Don't flood with too many small messages (batch if possible)
6. **Report status changes** - Keep operator informed of agent state
7. **Handle backpressure** - Respect stream send buffer limits

### State Machine

```
┌──────────────┐
│   INITIAL    │
└──────┬───────┘
       │ Receive StartExecutionRequest
       ▼
┌──────────────┐  Receive InterruptRequest   ┌──────────────┐
│   RUNNING    ├─────────────────────────────>│ INTERRUPTED  │
└──────┬───────┘                              └──────┬───────┘
       │                                             │
       │ Task complete                  Receive ResumeRequest
       │                                             │
       ▼                                             ▼
┌──────────────┐                              ┌──────────────┐
│  COMPLETED   │                              │   RUNNING    │
└──────────────┘                              └──────────────┘
```

---

## Go Implementation

### Basic Streaming Agent

```go
package main

import (
    "context"
    "fmt"
    "io"
    "sync"
    "time"

    "github.com/zero-day-ai/gibson/api/gen/proto"
    "google.golang.org/grpc"
)

type StreamingAgent struct {
    proto.UnimplementedAgentServiceServer
    name string
}

func (a *StreamingAgent) StreamExecute(stream proto.AgentService_StreamExecuteServer) error {
    ctx := stream.Context()

    // 1. Receive StartExecutionRequest
    msg, err := stream.Recv()
    if err != nil {
        return fmt.Errorf("failed to receive start request: %w", err)
    }

    startReq := msg.GetStart()
    if startReq == nil {
        return fmt.Errorf("expected StartExecutionRequest as first message")
    }

    // Parse task
    task, err := parseTask(startReq.TaskJson)
    if err != nil {
        return err
    }

    // 2. Create execution context with cancellation
    execCtx, cancel := context.WithCancel(ctx)
    defer cancel()

    // 3. Setup state management
    state := &executionState{
        mode:      startReq.InitialMode,
        sequence:  0,
        traceID:   generateTraceID(),
        spanID:    generateSpanID(),
    }

    // 4. Listen for client messages in background
    clientMsgCh := make(chan *proto.ClientMessage, 10)
    errCh := make(chan error, 1)

    go func() {
        for {
            msg, err := stream.Recv()
            if err == io.EOF {
                close(clientMsgCh)
                return
            }
            if err != nil {
                errCh <- err
                return
            }

            select {
            case clientMsgCh <- msg:
            case <-execCtx.Done():
                return
            }
        }
    }()

    // 5. Send initial status
    if err := a.sendStatus(stream, state, proto.AgentStatus_AGENT_STATUS_RUNNING, "Starting task execution"); err != nil {
        return err
    }

    // 6. Execute task with message handling
    if err := a.executeTask(execCtx, stream, state, task, clientMsgCh, errCh, cancel); err != nil {
        a.sendError(stream, state, "EXECUTION_FAILED", err.Error(), true)
        return err
    }

    // 7. Send completion status
    return a.sendStatus(stream, state, proto.AgentStatus_AGENT_STATUS_COMPLETED, "Task completed successfully")
}

func (a *StreamingAgent) executeTask(
    ctx context.Context,
    stream proto.AgentService_StreamExecuteServer,
    state *executionState,
    task *Task,
    clientMsgCh <-chan *proto.ClientMessage,
    errCh <-chan error,
    cancel context.CancelFunc,
) error {
    // Main execution loop
    for step := range generateSteps(ctx, task) {
        // Check for client messages
        select {
        case msg := <-clientMsgCh:
            if msg == nil {
                return nil // Client closed stream
            }

            if err := a.handleClientMessage(ctx, stream, state, msg, cancel); err != nil {
                return err
            }

        case err := <-errCh:
            return fmt.Errorf("stream error: %w", err)

        case <-ctx.Done():
            return ctx.Err()

        default:
            // Continue execution
        }

        // Stream output
        if err := a.sendOutput(stream, state, step.Output, step.IsReasoning); err != nil {
            return err
        }

        // Execute tools if needed
        if step.ToolCall != nil {
            if err := a.executeToolCall(ctx, stream, state, step.ToolCall); err != nil {
                return err
            }
        }

        // Report findings
        if step.Finding != nil {
            if err := a.sendFinding(stream, state, step.Finding); err != nil {
                return err
            }
        }
    }

    return nil
}

func (a *StreamingAgent) handleClientMessage(
    ctx context.Context,
    stream proto.AgentService_StreamExecuteServer,
    state *executionState,
    msg *proto.ClientMessage,
    cancel context.CancelFunc,
) error {
    switch payload := msg.Payload.(type) {
    case *proto.ClientMessage_Steering:
        return a.handleSteering(stream, state, payload.Steering)

    case *proto.ClientMessage_Interrupt:
        return a.handleInterrupt(stream, state, payload.Interrupt, cancel)

    case *proto.ClientMessage_SetMode:
        return a.handleSetMode(stream, state, payload.SetMode)

    case *proto.ClientMessage_Resume:
        return a.handleResume(stream, state, payload.Resume)

    default:
        return fmt.Errorf("unknown client message type")
    }
}

func (a *StreamingAgent) handleSteering(
    stream proto.AgentService_StreamExecuteServer,
    state *executionState,
    steering *proto.SteeringMessage,
) error {
    // Process steering message
    // ... apply guidance to execution ...

    // Send acknowledgment
    return stream.Send(&proto.AgentMessage{
        Payload: &proto.AgentMessage_SteeringAck{
            SteeringAck: &proto.SteeringAck{
                MessageId: steering.Id,
                Response:  fmt.Sprintf("Acknowledged: %s", steering.Content),
            },
        },
        TraceId:     state.traceID,
        SpanId:      state.spanID,
        Sequence:    state.nextSequence(),
        TimestampMs: time.Now().UnixMilli(),
    })
}

func (a *StreamingAgent) handleInterrupt(
    stream proto.AgentService_StreamExecuteServer,
    state *executionState,
    interrupt *proto.InterruptRequest,
    cancel context.CancelFunc,
) error {
    // Send status change
    if err := a.sendStatus(stream, state, proto.AgentStatus_AGENT_STATUS_INTERRUPTED, interrupt.Reason); err != nil {
        return err
    }

    // Cancel execution context
    cancel()

    return nil
}

func (a *StreamingAgent) sendOutput(
    stream proto.AgentService_StreamExecuteServer,
    state *executionState,
    content string,
    isReasoning bool,
) error {
    return stream.Send(&proto.AgentMessage{
        Payload: &proto.AgentMessage_Output{
            Output: &proto.OutputChunk{
                Content:     content,
                IsReasoning: isReasoning,
            },
        },
        TraceId:     state.traceID,
        SpanId:      state.spanID,
        Sequence:    state.nextSequence(),
        TimestampMs: time.Now().UnixMilli(),
    })
}

func (a *StreamingAgent) sendStatus(
    stream proto.AgentService_StreamExecuteServer,
    state *executionState,
    status proto.AgentStatus,
    message string,
) error {
    return stream.Send(&proto.AgentMessage{
        Payload: &proto.AgentMessage_Status{
            Status: &proto.StatusChange{
                Status:  status,
                Message: message,
            },
        },
        TraceId:     state.traceID,
        SpanId:      state.spanID,
        Sequence:    state.nextSequence(),
        TimestampMs: time.Now().UnixMilli(),
    })
}

type executionState struct {
    mu       sync.Mutex
    mode     proto.AgentMode
    sequence int64
    traceID  string
    spanID   string
}

func (s *executionState) nextSequence() int64 {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.sequence++
    return s.sequence
}

func main() {
    lis, err := net.Listen("tcp", ":50051")
    if err != nil {
        log.Fatalf("failed to listen: %v", err)
    }

    grpcServer := grpc.NewServer()
    proto.RegisterAgentServiceServer(grpcServer, &StreamingAgent{name: "my-streaming-agent"})

    log.Printf("Agent listening on :50051")
    if err := grpcServer.Serve(lis); err != nil {
        log.Fatalf("failed to serve: %v", err)
    }
}
```

---

## Python Implementation

### Using grpcio

```python
import grpc
import time
import threading
from queue import Queue
from typing import Optional
from concurrent import futures

import agent_pb2
import agent_pb2_grpc


class StreamingAgent(agent_pb2_grpc.AgentServiceServicer):
    def __init__(self, name: str):
        self.name = name

    def StreamExecute(self, request_iterator, context):
        """Bidirectional streaming RPC implementation."""

        # 1. Receive first message (StartExecutionRequest)
        first_msg = next(request_iterator)
        if not first_msg.HasField('start'):
            context.abort(grpc.StatusCode.INVALID_ARGUMENT,
                         "First message must be StartExecutionRequest")
            return

        start_req = first_msg.start
        task = self._parse_task(start_req.task_json)

        # 2. Setup state
        state = ExecutionState(
            mode=start_req.initial_mode,
            trace_id=self._generate_trace_id(),
            span_id=self._generate_span_id()
        )

        # 3. Setup message queue for client messages
        client_msg_queue = Queue()
        stop_event = threading.Event()

        # 4. Listen for client messages in background
        def listen_for_messages():
            try:
                for msg in request_iterator:
                    client_msg_queue.put(msg)
            except grpc.RpcError:
                pass
            finally:
                stop_event.set()

        listener_thread = threading.Thread(target=listen_for_messages)
        listener_thread.daemon = True
        listener_thread.start()

        # 5. Send initial status
        yield self._create_status_message(
            state,
            agent_pb2.AGENT_STATUS_RUNNING,
            "Starting task execution"
        )

        # 6. Execute task
        try:
            for msg in self._execute_task(task, state, client_msg_queue, stop_event):
                yield msg
        except Exception as e:
            yield self._create_error_message(
                state,
                "EXECUTION_FAILED",
                str(e),
                fatal=True
            )
            return

        # 7. Send completion
        yield self._create_status_message(
            state,
            agent_pb2.AGENT_STATUS_COMPLETED,
            "Task completed successfully"
        )

    def _execute_task(self, task, state, client_msg_queue, stop_event):
        """Main execution loop."""

        for step in self._generate_steps(task):
            # Check for stop
            if stop_event.is_set():
                break

            # Check for client messages
            while not client_msg_queue.empty():
                msg = client_msg_queue.get()

                if msg.HasField('steering'):
                    yield self._handle_steering(state, msg.steering)

                elif msg.HasField('interrupt'):
                    yield self._handle_interrupt(state, msg.interrupt)
                    return  # Stop execution

                elif msg.HasField('set_mode'):
                    state.mode = msg.set_mode.mode

            # Stream output
            yield self._create_output_message(
                state,
                step['output'],
                step.get('is_reasoning', False)
            )

            # Execute tool calls
            if 'tool_call' in step:
                yield self._create_tool_call_message(state, step['tool_call'])
                result = self._execute_tool(step['tool_call'])
                yield self._create_tool_result_message(state, result)

            # Report findings
            if 'finding' in step:
                yield self._create_finding_message(state, step['finding'])

    def _handle_steering(self, state, steering):
        """Handle steering message and return acknowledgment."""
        # Process steering guidance
        # ... update execution plan ...

        # Create acknowledgment
        return agent_pb2.AgentMessage(
            steering_ack=agent_pb2.SteeringAck(
                message_id=steering.id,
                response=f"Acknowledged: {steering.content}"
            ),
            trace_id=state.trace_id,
            span_id=state.span_id,
            sequence=state.next_sequence(),
            timestamp_ms=int(time.time() * 1000)
        )

    def _handle_interrupt(self, state, interrupt):
        """Handle interrupt request."""
        return agent_pb2.AgentMessage(
            status=agent_pb2.StatusChange(
                status=agent_pb2.AGENT_STATUS_INTERRUPTED,
                message=interrupt.reason
            ),
            trace_id=state.trace_id,
            span_id=state.span_id,
            sequence=state.next_sequence(),
            timestamp_ms=int(time.time() * 1000)
        )

    def _create_output_message(self, state, content, is_reasoning):
        """Create output chunk message."""
        return agent_pb2.AgentMessage(
            output=agent_pb2.OutputChunk(
                content=content,
                is_reasoning=is_reasoning
            ),
            trace_id=state.trace_id,
            span_id=state.span_id,
            sequence=state.next_sequence(),
            timestamp_ms=int(time.time() * 1000)
        )

    def _create_status_message(self, state, status, message):
        """Create status change message."""
        return agent_pb2.AgentMessage(
            status=agent_pb2.StatusChange(
                status=status,
                message=message
            ),
            trace_id=state.trace_id,
            span_id=state.span_id,
            sequence=state.next_sequence(),
            timestamp_ms=int(time.time() * 1000)
        )


class ExecutionState:
    def __init__(self, mode, trace_id, span_id):
        self.mode = mode
        self.trace_id = trace_id
        self.span_id = span_id
        self._sequence = 0
        self._lock = threading.Lock()

    def next_sequence(self):
        with self._lock:
            self._sequence += 1
            return self._sequence


def serve():
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    agent_pb2_grpc.add_AgentServiceServicer_to_server(
        StreamingAgent("my-streaming-agent"),
        server
    )
    server.add_insecure_port('[::]:50051')
    server.start()
    print("Agent listening on :50051")
    server.wait_for_termination()


if __name__ == '__main__':
    serve()
```

---

## Best Practices

### 1. Message Sequencing

Always use monotonically increasing sequence numbers:

```go
type sequenceGenerator struct {
    mu  sync.Mutex
    seq int64
}

func (s *sequenceGenerator) Next() int64 {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.seq++
    return s.seq
}
```

### 2. Observability Integration

Set trace context on all messages:

```go
// Extract trace context from incoming request
traceID := extractTraceID(ctx)
spanID := generateSpanID()

// Set on all outgoing messages
msg := &proto.AgentMessage{
    TraceId:     traceID,
    SpanId:      spanID,
    Sequence:    seq.Next(),
    TimestampMs: time.Now().UnixMilli(),
    // ... payload
}
```

### 3. Graceful Shutdown

Handle context cancellation properly:

```go
func (a *Agent) executeTask(ctx context.Context, ...) error {
    defer cleanup()

    for {
        select {
        case <-ctx.Done():
            a.sendStatus(stream, state, proto.AgentStatus_AGENT_STATUS_INTERRUPTED, "Context cancelled")
            return ctx.Err()

        case step := <-stepCh:
            // Process step
        }
    }
}
```

### 4. Rate Limiting

Batch small output chunks to avoid flooding:

```go
type outputBuffer struct {
    buf     strings.Builder
    lastSent time.Time
    minInterval time.Duration
}

func (b *outputBuffer) Add(text string) {
    b.buf.WriteString(text)
}

func (b *outputBuffer) ShouldFlush() bool {
    return b.buf.Len() > 1000 || time.Since(b.lastSent) > b.minInterval
}

func (b *outputBuffer) Flush(send func(string) error) error {
    if b.buf.Len() == 0 {
        return nil
    }

    if err := send(b.buf.String()); err != nil {
        return err
    }

    b.buf.Reset()
    b.lastSent = time.Now()
    return nil
}
```

### 5. Error Recovery

Send non-fatal errors without terminating the stream:

```go
if err := a.executeTool(ctx, toolCall); err != nil {
    // Send error event but continue execution
    a.sendError(stream, state, "TOOL_FAILED", err.Error(), false)
    continue
}
```

### 6. Backpressure Handling

Respect stream send buffer limits:

```go
func (a *Agent) sendWithBackpressure(stream Stream, msg *proto.AgentMessage) error {
    ctx, cancel := context.WithTimeout(stream.Context(), 5*time.Second)
    defer cancel()

    // This will block if buffer is full
    return stream.Send(msg)
}
```

---

## Testing

### Test Harness

Use the provided test harness to validate streaming implementation:

```bash
gibson agent test --stream my-agent
```

This will:
1. Start your agent
2. Send StartExecutionRequest
3. Send steering messages
4. Test interrupt/resume
5. Verify all message types
6. Check observability fields
7. Validate graceful shutdown

### Manual Testing

Test streaming manually with `grpcurl`:

```bash
# Start interactive stream
grpcurl -plaintext -d @ localhost:50051 gibson.agent.AgentService/StreamExecute <<EOF
{
    "start": {
        "task_json": "{\"goal\":\"test task\"}",
        "initial_mode": "AGENT_MODE_AUTONOMOUS"
    }
}
EOF
```

### Unit Testing

Mock the stream for unit tests:

```go
type mockStream struct {
    grpc.ServerStream
    sent     []*proto.AgentMessage
    received []*proto.ClientMessage
    recvIdx  int
}

func (m *mockStream) Send(msg *proto.AgentMessage) error {
    m.sent = append(m.sent, msg)
    return nil
}

func (m *mockStream) Recv() (*proto.ClientMessage, error) {
    if m.recvIdx >= len(m.received) {
        return nil, io.EOF
    }
    msg := m.received[m.recvIdx]
    m.recvIdx++
    return msg, nil
}

func TestStreamExecute(t *testing.T) {
    agent := &StreamingAgent{}
    stream := &mockStream{
        received: []*proto.ClientMessage{
            {Payload: &proto.ClientMessage_Start{...}},
        },
    }

    err := agent.StreamExecute(stream)
    assert.NoError(t, err)
    assert.Greater(t, len(stream.sent), 0)
}
```

---

## Error Handling

### Error Categories

| Error Type | Fatal | Action |
|------------|-------|--------|
| Invalid start request | Yes | Abort stream |
| LLM timeout | No | Send ErrorEvent, retry |
| Tool failure | No | Send ErrorEvent, continue |
| Context cancelled | Yes | Send INTERRUPTED status |
| Stream send failure | Yes | Return error |

### Error Reporting

```go
func (a *Agent) sendError(
    stream Stream,
    state *executionState,
    code string,
    message string,
    fatal bool,
) error {
    err := stream.Send(&proto.AgentMessage{
        Payload: &proto.AgentMessage_Error{
            Error: &proto.ErrorEvent{
                Code:    code,
                Message: message,
                Fatal:   fatal,
            },
        },
        TraceId:     state.traceID,
        SpanId:      state.spanID,
        Sequence:    state.nextSequence(),
        TimestampMs: time.Now().UnixMilli(),
    })

    if fatal {
        return fmt.Errorf("fatal error: %s", message)
    }

    return err
}
```

---

## Observability Integration

### OpenTelemetry Spans

Create spans for major operations:

```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/trace"
)

func (a *Agent) StreamExecute(stream Stream) error {
    tracer := otel.Tracer("agent")
    ctx, span := tracer.Start(stream.Context(), "StreamExecute")
    defer span.End()

    // Extract trace context
    traceID := span.SpanContext().TraceID().String()
    spanID := span.SpanContext().SpanID().String()

    // Use in all messages
    state := &executionState{
        traceID: traceID,
        spanID:  spanID,
    }

    // ... rest of implementation
}
```

### Metrics

Record key metrics:

```go
var (
    messagesReceived = promauto.NewCounter(prometheus.CounterOpts{
        Name: "agent_stream_messages_received_total",
    })

    messagesSent = promauto.NewCounter(prometheus.CounterOpts{
        Name: "agent_stream_messages_sent_total",
    })

    steeringMessages = promauto.NewCounter(prometheus.CounterOpts{
        Name: "agent_steering_messages_total",
    })
)
```

### Structured Logging

Use structured logging with trace correlation:

```go
logger.Info("received steering message",
    "trace_id", state.traceID,
    "span_id", state.spanID,
    "steering_id", steering.Id,
    "content", steering.Content,
)
```

---

## Summary

Implementing streaming in Gibson agents enables:

1. **Real-time visibility** - Operators see progress as it happens
2. **Interactive control** - Steer agents mid-execution
3. **Better observability** - Rich event stream with full context
4. **Graceful interrupts** - Stop runaway agents safely
5. **Flexible execution modes** - Switch between autonomous and interactive

**Key Points:**
- StreamExecute is bidirectional - both sides send and receive
- Always send StartExecutionRequest first
- Acknowledge all steering messages
- Include observability fields (trace_id, span_id, sequence, timestamp)
- Handle interrupts gracefully with proper cleanup
- Rate limit output to avoid flooding
- Test thoroughly with the test harness

For more information, see:
- [Gibson SDK Guide](./SDK_AND_KNOWLEDGE_GRAPH_GUIDE.md)
- [Observability Guide](./OBSERVABILITY.md)
- [Agent Protocol Reference](../api/proto/agent.proto)
