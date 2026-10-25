package events_test

// Port of the official AG-UI Python SDK tests/test_events.py.

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/sse"
)

// =============================================================================
// Shared helpers (used by every file of this package)
// =============================================================================

const ts = int64(1648214400000)

func tsPtr() *int64 { return agui.Ptr(ts) }

// generic marshals v and decodes it back into a generic JSON value, the Go
// analogue of model_dump(by_alias=True).
func generic(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %T: %v", v, err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal %s: %v", b, err)
	}
	return out
}

// dump is generic for a value that must serialise to a JSON object.
func dump(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := generic(t, v).(map[string]any)
	if !ok {
		t.Fatalf("%T did not serialise to an object", v)
	}
	return m
}

// mustJSON marshals v.
func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %T: %v", v, err)
	}
	return b
}

// encodeSSE is the analogue of the Python EventEncoder.encode: one event as
// one SSE frame, "data: <json>\n\n".
func encodeSSE(t *testing.T, e agui.Event) string {
	t.Helper()
	var buf bytes.Buffer
	if err := sse.NewWriter(&buf).WriteData(mustJSON(t, e)); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return buf.String()
}

// sseJSON decodes the JSON carried by an encodeSSE frame.
func sseJSON(t *testing.T, frame string) map[string]any {
	t.Helper()
	if !strings.HasPrefix(frame, "data: ") || !strings.HasSuffix(frame, "\n\n") {
		t.Fatalf("not an SSE data frame: %q", frame)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(frame[len("data: "):len(frame)-2]), &m); err != nil {
		t.Fatalf("frame payload is not a JSON object: %v", err)
	}
	return m
}

func eq(t *testing.T, what string, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %#v, want %#v", what, got, want)
	}
}

func absent(t *testing.T, m map[string]any, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if _, ok := m[k]; ok {
			t.Errorf("key %q present (%v), want absent", k, m[k])
		}
	}
}

func present(t *testing.T, m map[string]any, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			t.Errorf("key %q absent, want present", k)
		}
	}
}

// parse is the analogue of TypeAdapter(Event).validate_json: a schema-checked
// decode into the concrete event type.
func parse(t *testing.T, data []byte) agui.Event {
	t.Helper()
	e, err := agui.ParseEvent(data)
	if err != nil {
		t.Fatalf("ParseEvent(%s): %v", data, err)
	}
	return e
}

// =============================================================================
// TestEvents
// =============================================================================

// Python: tests/test_events.py::TestEvents::test_event_types_enum
func TestPyTestEvents_EventTypesEnum(t *testing.T) {
	eq(t, "TEXT_MESSAGE_START", string(agui.EventTypeTextMessageStart), "TEXT_MESSAGE_START")
	eq(t, "TOOL_CALL_ARGS", string(agui.EventTypeToolCallArgs), "TOOL_CALL_ARGS")
	eq(t, "STATE_SNAPSHOT", string(agui.EventTypeStateSnapshot), "STATE_SNAPSHOT")
	eq(t, "RUN_ERROR", string(agui.EventTypeRunError), "RUN_ERROR")
	eq(t, "STEP_FINISHED", string(agui.EventTypeStepFinished), "STEP_FINISHED")
}

// Python: tests/test_events.py::TestEvents::test_base_event_creation
//
// Go has no free-standing typed BaseEvent; the envelope is embedded in every
// concrete event. NewEvent(RAW) gives the zero RAW event whose envelope is
// checked instead.
func TestPyTestEvents_BaseEventCreation(t *testing.T) {
	now := time.Now().UnixMilli()
	e, err := agui.NewEvent(agui.EventTypeRaw)
	if err != nil {
		t.Fatal(err)
	}
	e.Base().Timestamp = &now
	eq(t, "type", e.Type(), agui.EventTypeRaw)
	eq(t, "timestamp", *e.Base().Timestamp, now)
	if e.Base().RawEvent != nil {
		t.Errorf("rawEvent = %v, want nil", e.Base().RawEvent)
	}
}

// Python: tests/test_events.py::TestEvents::test_text_message_start
func TestPyTestEvents_TextMessageStart(t *testing.T) {
	e := agui.TextMessageStartEvent{BaseEvent: agui.BaseEvent{Timestamp: tsPtr()}, MessageID: "msg_123"}
	eq(t, "MessageID", e.MessageID, "msg_123")
	// An absent role means assistant (schema prose); it is not materialised.
	if e.Role != nil {
		t.Errorf("Role = %v, want nil", *e.Role)
	}
	s := dump(t, e)
	eq(t, "type", s["type"], "TEXT_MESSAGE_START")
	eq(t, "messageId", s["messageId"], "msg_123")
	eq(t, "timestamp", s["timestamp"], float64(ts))
}

// Python: tests/test_events.py::TestEvents::test_text_message_content
func TestPyTestEvents_TextMessageContent(t *testing.T) {
	e := agui.TextMessageContentEvent{BaseEvent: agui.BaseEvent{Timestamp: tsPtr()}, MessageID: "msg_123", Delta: "Hello, world!"}
	eq(t, "MessageID", e.MessageID, "msg_123")
	eq(t, "Delta", e.Delta, "Hello, world!")
	s := dump(t, e)
	eq(t, "type", s["type"], "TEXT_MESSAGE_CONTENT")
	eq(t, "messageId", s["messageId"], "msg_123")
	eq(t, "delta", s["delta"], "Hello, world!")
}

// Python: tests/test_events.py::TestEvents::test_text_message_end
func TestPyTestEvents_TextMessageEnd(t *testing.T) {
	e := agui.TextMessageEndEvent{BaseEvent: agui.BaseEvent{Timestamp: tsPtr()}, MessageID: "msg_123"}
	eq(t, "MessageID", e.MessageID, "msg_123")
	s := dump(t, e)
	eq(t, "type", s["type"], "TEXT_MESSAGE_END")
	eq(t, "messageId", s["messageId"], "msg_123")
}

// Python: tests/test_events.py::TestEvents::test_tool_call_start
func TestPyTestEvents_ToolCallStart(t *testing.T) {
	e := agui.ToolCallStartEvent{
		BaseEvent:       agui.BaseEvent{Timestamp: tsPtr()},
		ToolCallID:      "call_123",
		ToolCallName:    "get_weather",
		ParentMessageID: agui.Ptr("msg_456"),
	}
	eq(t, "ToolCallID", e.ToolCallID, "call_123")
	eq(t, "ToolCallName", e.ToolCallName, "get_weather")
	eq(t, "ParentMessageID", *e.ParentMessageID, "msg_456")
	s := dump(t, e)
	eq(t, "type", s["type"], "TOOL_CALL_START")
	eq(t, "toolCallId", s["toolCallId"], "call_123")
	eq(t, "toolCallName", s["toolCallName"], "get_weather")
	eq(t, "parentMessageId", s["parentMessageId"], "msg_456")
}

// Python: tests/test_events.py::TestEvents::test_tool_call_args
func TestPyTestEvents_ToolCallArgs(t *testing.T) {
	e := agui.ToolCallArgsEvent{BaseEvent: agui.BaseEvent{Timestamp: tsPtr()}, ToolCallID: "call_123", Delta: `{"location": "New York"}`}
	eq(t, "ToolCallID", e.ToolCallID, "call_123")
	eq(t, "Delta", e.Delta, `{"location": "New York"}`)
	s := dump(t, e)
	eq(t, "type", s["type"], "TOOL_CALL_ARGS")
	eq(t, "toolCallId", s["toolCallId"], "call_123")
	eq(t, "delta", s["delta"], `{"location": "New York"}`)
}

// Python: tests/test_events.py::TestEvents::test_tool_call_end
func TestPyTestEvents_ToolCallEnd(t *testing.T) {
	e := agui.ToolCallEndEvent{BaseEvent: agui.BaseEvent{Timestamp: tsPtr()}, ToolCallID: "call_123"}
	eq(t, "ToolCallID", e.ToolCallID, "call_123")
	s := dump(t, e)
	eq(t, "type", s["type"], "TOOL_CALL_END")
	eq(t, "toolCallId", s["toolCallId"], "call_123")
}

// Python: tests/test_events.py::TestEvents::test_state_snapshot
func TestPyTestEvents_StateSnapshot(t *testing.T) {
	state := map[string]any{"conversation_state": "active", "user_info": map[string]any{"name": "John"}}
	e := agui.StateSnapshotEvent{BaseEvent: agui.BaseEvent{Timestamp: tsPtr()}, Snapshot: state}
	eq(t, "Snapshot", e.Snapshot, any(state))
	s := dump(t, e)
	eq(t, "type", s["type"], "STATE_SNAPSHOT")
	snap := s["snapshot"].(map[string]any)
	eq(t, "snapshot.conversation_state", snap["conversation_state"], "active")
	eq(t, "snapshot.user_info.name", snap["user_info"].(map[string]any)["name"], "John")
}

// Python: tests/test_events.py::TestEvents::test_state_delta
func TestPyTestEvents_StateDelta(t *testing.T) {
	deltaJSON := `[{"op":"replace","path":"/conversation_state","value":"paused"},{"op":"add","path":"/user_info/age","value":30}]`
	var delta agui.JSONPatch
	if err := json.Unmarshal([]byte(deltaJSON), &delta); err != nil {
		t.Fatal(err)
	}
	e := agui.StateDeltaEvent{BaseEvent: agui.BaseEvent{Timestamp: tsPtr()}, Delta: delta}
	// Typed operations, identical wire form.
	var want any
	_ = json.Unmarshal([]byte(deltaJSON), &want)
	eq(t, "delta round trip", generic(t, e.Delta), want)
	eq(t, "Delta[0].Op", e.Delta[0].Op, "replace")

	s := dump(t, e)
	eq(t, "type", s["type"], "STATE_DELTA")
	d := s["delta"].([]any)
	eq(t, "len(delta)", len(d), 2)
	eq(t, "delta[0].op", d[0].(map[string]any)["op"], "replace")
	eq(t, "delta[1].path", d[1].(map[string]any)["path"], "/user_info/age")
}

// Python: tests/test_events.py::TestEvents::test_messages_snapshot
func TestPyTestEvents_MessagesSnapshot(t *testing.T) {
	msgs := agui.Messages{
		&agui.UserMessage{ID: "user_1", Content: agui.TextContent("Hello")},
		&agui.AssistantMessage{ID: "asst_1", Content: agui.Ptr("Hi there"), ToolCalls: []agui.ToolCall{
			{ID: "call_1", Function: agui.FunctionCall{Name: "get_weather", Arguments: `{"location": "New York"}`}},
		}},
	}
	e := agui.MessagesSnapshotEvent{BaseEvent: agui.BaseEvent{Timestamp: tsPtr()}, Messages: msgs}
	eq(t, "len(Messages)", len(e.Messages), 2)
	eq(t, "Messages[0].ID", e.Messages[0].MessageID(), "user_1")
	eq(t, "Messages[1].ToolCalls[0].Function.Name", e.Messages[1].(*agui.AssistantMessage).ToolCalls[0].Function.Name, "get_weather")

	s := dump(t, e)
	eq(t, "type", s["type"], "MESSAGES_SNAPSHOT")
	ms := s["messages"].([]any)
	eq(t, "len(messages)", len(ms), 2)
	eq(t, "messages[0].role", ms[0].(map[string]any)["role"], "user")
	tc := ms[1].(map[string]any)["toolCalls"].([]any)[0].(map[string]any)
	eq(t, "messages[1].toolCalls[0].function.name", tc["function"].(map[string]any)["name"], "get_weather")
}

// Python: tests/test_events.py::TestEvents::test_activity_snapshot
func TestPyTestEvents_ActivitySnapshot(t *testing.T) {
	content := map[string]any{"tasks": []any{"search", "summarize"}}
	e := agui.ActivitySnapshotEvent{BaseEvent: agui.BaseEvent{Timestamp: tsPtr()}, MessageID: "msg_activity", ActivityType: "PLAN", Content: content}
	eq(t, "MessageID", e.MessageID, "msg_activity")
	eq(t, "ActivityType", e.ActivityType, "PLAN")
	eq(t, "Content", e.Content, content)
	if e.Replace != nil {
		t.Errorf("Replace = %v, want nil (absent means replace)", *e.Replace)
	}
	s := dump(t, e)
	eq(t, "type", s["type"], "ACTIVITY_SNAPSHOT")
	eq(t, "messageId", s["messageId"], "msg_activity")
	eq(t, "activityType", s["activityType"], "PLAN")
	eq(t, "content", s["content"], any(content))
	absent(t, s, "replace")

	f := agui.ActivitySnapshotEvent{MessageID: "msg_activity", ActivityType: "PLAN", Content: content, Replace: agui.Ptr(false)}
	if f.Replace == nil || *f.Replace {
		t.Errorf("Replace = %v, want false", f.Replace)
	}
	sf := dump(t, f)
	eq(t, "replace", sf["replace"], false)
}

// Python: tests/test_events.py::TestEvents::test_activity_delta
func TestPyTestEvents_ActivityDelta(t *testing.T) {
	patchJSON := `[{"op":"replace","path":"/tasks/0","value":"✓ search"}]`
	var patch agui.JSONPatch
	if err := json.Unmarshal([]byte(patchJSON), &patch); err != nil {
		t.Fatal(err)
	}
	var want any
	_ = json.Unmarshal([]byte(patchJSON), &want)
	e := agui.ActivityDeltaEvent{BaseEvent: agui.BaseEvent{Timestamp: tsPtr()}, MessageID: "msg_activity", ActivityType: "PLAN", Patch: patch}
	eq(t, "MessageID", e.MessageID, "msg_activity")
	eq(t, "ActivityType", e.ActivityType, "PLAN")
	eq(t, "patch round trip", generic(t, e.Patch), want)

	s := dump(t, e)
	eq(t, "type", s["type"], "ACTIVITY_DELTA")
	eq(t, "messageId", s["messageId"], "msg_activity")
	eq(t, "activityType", s["activityType"], "PLAN")
	eq(t, "patch", s["patch"], want)
}

// Python: tests/test_events.py::TestEvents::test_raw_event
func TestPyTestEvents_RawEvent(t *testing.T) {
	raw := map[string]any{"origin": "server", "data": map[string]any{"key": "value"}}
	e := agui.RawEvent{BaseEvent: agui.BaseEvent{Timestamp: tsPtr()}, Event: raw, Source: agui.Ptr("api")}
	eq(t, "Event", e.Event, any(raw))
	eq(t, "Source", *e.Source, "api")
	s := dump(t, e)
	eq(t, "type", s["type"], "RAW")
	eq(t, "event.origin", s["event"].(map[string]any)["origin"], "server")
	eq(t, "source", s["source"], "api")
}

// Python: tests/test_events.py::TestEvents::test_custom_event
func TestPyTestEvents_CustomEvent(t *testing.T) {
	e := agui.CustomEvent{BaseEvent: agui.BaseEvent{Timestamp: tsPtr()}, Name: "user_action", Value: map[string]any{"action": "click", "element": "button"}}
	eq(t, "Name", e.Name, "user_action")
	eq(t, "Value.action", e.Value.(map[string]any)["action"], "click")
	s := dump(t, e)
	eq(t, "type", s["type"], "CUSTOM")
	eq(t, "name", s["name"], "user_action")
	eq(t, "value.element", s["value"].(map[string]any)["element"], "button")
}

// Python: tests/test_events.py::TestEvents::test_run_started
func TestPyTestEvents_RunStarted(t *testing.T) {
	e := agui.RunStartedEvent{BaseEvent: agui.BaseEvent{Timestamp: tsPtr()}, ThreadID: "thread_123", RunID: "run_456"}
	eq(t, "ThreadID", e.ThreadID, "thread_123")
	eq(t, "RunID", e.RunID, "run_456")
	s := dump(t, e)
	eq(t, "type", s["type"], "RUN_STARTED")
	eq(t, "threadId", s["threadId"], "thread_123")
	eq(t, "runId", s["runId"], "run_456")
}

// Python: tests/test_events.py::TestEvents::test_run_finished
func TestPyTestEvents_RunFinished(t *testing.T) {
	e := agui.RunFinishedEvent{BaseEvent: agui.BaseEvent{Timestamp: tsPtr()}, ThreadID: "thread_123", RunID: "run_456"}
	eq(t, "ThreadID", e.ThreadID, "thread_123")
	eq(t, "RunID", e.RunID, "run_456")
	s := dump(t, e)
	eq(t, "type", s["type"], "RUN_FINISHED")
	eq(t, "threadId", s["threadId"], "thread_123")
	eq(t, "runId", s["runId"], "run_456")
}

// Python: tests/test_events.py::TestEvents::test_run_error
func TestPyTestEvents_RunError(t *testing.T) {
	e := agui.RunErrorEvent{BaseEvent: agui.BaseEvent{Timestamp: tsPtr()}, Message: "An error occurred during execution", Code: agui.Ptr("ERROR_001")}
	eq(t, "Message", e.Message, "An error occurred during execution")
	eq(t, "Code", *e.Code, "ERROR_001")
	s := dump(t, e)
	eq(t, "type", s["type"], "RUN_ERROR")
	eq(t, "message", s["message"], "An error occurred during execution")
	eq(t, "code", s["code"], "ERROR_001")
}

// Python: tests/test_events.py::TestEvents::test_step_started
func TestPyTestEvents_StepStarted(t *testing.T) {
	e := agui.StepStartedEvent{BaseEvent: agui.BaseEvent{Timestamp: tsPtr()}, StepName: "process_data"}
	eq(t, "StepName", e.StepName, "process_data")
	s := dump(t, e)
	eq(t, "type", s["type"], "STEP_STARTED")
	eq(t, "stepName", s["stepName"], "process_data")
}

// Python: tests/test_events.py::TestEvents::test_step_finished
func TestPyTestEvents_StepFinished(t *testing.T) {
	e := agui.StepFinishedEvent{BaseEvent: agui.BaseEvent{Timestamp: tsPtr()}, StepName: "process_data"}
	eq(t, "StepName", e.StepName, "process_data")
	s := dump(t, e)
	eq(t, "type", s["type"], "STEP_FINISHED")
	eq(t, "stepName", s["stepName"], "process_data")
}

// Python: tests/test_events.py::TestEvents::test_event_union_deserialization
func TestPyTestEvents_EventUnionDeserialization(t *testing.T) {
	cases := []struct {
		data string
		want agui.Event
	}{
		{`{"type":"TEXT_MESSAGE_START","messageId":"msg_start","role":"assistant","timestamp":1648214400000}`, &agui.TextMessageStartEvent{}},
		{`{"type":"TEXT_MESSAGE_CONTENT","messageId":"msg_content","delta":"Hello!","timestamp":1648214400000}`, &agui.TextMessageContentEvent{}},
		{`{"type":"TOOL_CALL_START","toolCallId":"call_start","toolCallName":"get_info","timestamp":1648214400000}`, &agui.ToolCallStartEvent{}},
		{`{"type":"STATE_SNAPSHOT","snapshot":{"status":"active"},"timestamp":1648214400000}`, &agui.StateSnapshotEvent{}},
		{`{"type":"ACTIVITY_SNAPSHOT","messageId":"msg_activity","activityType":"PLAN","content":{"tasks":[]},"timestamp":1648214400000}`, &agui.ActivitySnapshotEvent{}},
		{`{"type":"RUN_ERROR","message":"Error occurred","code":"ERR_001","timestamp":1648214400000}`, &agui.RunErrorEvent{}},
	}
	for _, c := range cases {
		e := parse(t, []byte(c.data))
		if reflect.TypeOf(e) != reflect.TypeOf(c.want) {
			t.Errorf("%s decoded as %T, want %T", c.data, e, c.want)
			continue
		}
		var m map[string]any
		_ = json.Unmarshal([]byte(c.data), &m)
		eq(t, "type", string(e.Type()), m["type"])
		if e.Base().Timestamp == nil || *e.Base().Timestamp != ts {
			t.Errorf("%s: timestamp = %v, want %d", e.Type(), e.Base().Timestamp, ts)
		}
	}
}

// Python: tests/test_events.py::TestEvents::test_empty_delta_accepted
func TestPyTestEvents_EmptyDeltaAccepted(t *testing.T) {
	e := agui.TextMessageContentEvent{MessageID: "msg_123", Delta: ""}
	eq(t, "Delta", e.Delta, "")
	// The schema states the empty string is a valid delta.
	if err := agui.ValidateEvent(&e); err != nil {
		t.Errorf("empty delta rejected: %v", err)
	}
	s := dump(t, e)
	present(t, s, "delta")
	eq(t, "delta", s["delta"], "")
}

// Python: tests/test_events.py::TestEvents::test_serialization_round_trip
func TestPyTestEvents_SerializationRoundTrip(t *testing.T) {
	var patch agui.JSONPatch
	_ = json.Unmarshal([]byte(`[{"op":"add","path":"/tasks/-","value":"search"}]`), &patch)
	events := []agui.Event{
		&agui.TextMessageStartEvent{MessageID: "msg_123"},
		&agui.TextMessageContentEvent{MessageID: "msg_123", Delta: "Hello, world!"},
		&agui.ToolCallStartEvent{ToolCallID: "call_123", ToolCallName: "get_weather"},
		&agui.StateSnapshotEvent{Snapshot: map[string]any{"status": "active"}},
		&agui.MessagesSnapshotEvent{Messages: agui.Messages{&agui.UserMessage{ID: "user_1", Content: agui.TextContent("Hello")}}},
		&agui.ActivitySnapshotEvent{MessageID: "msg_activity", ActivityType: "PLAN", Content: map[string]any{"tasks": []any{}}},
		&agui.ActivityDeltaEvent{MessageID: "msg_activity", ActivityType: "PLAN", Patch: patch},
		&agui.RunStartedEvent{ThreadID: "thread_123", RunID: "run_456"},
	}
	for _, orig := range events {
		d := parse(t, mustJSON(t, orig))
		if reflect.TypeOf(d) != reflect.TypeOf(orig) {
			t.Errorf("%T decoded as %T", orig, d)
			continue
		}
		eq(t, "type", d.Type(), orig.Type())
		switch o := orig.(type) {
		case *agui.TextMessageStartEvent:
			got := d.(*agui.TextMessageStartEvent)
			eq(t, "MessageID", got.MessageID, o.MessageID)
			eq(t, "Role", got.Role, o.Role)
		case *agui.TextMessageContentEvent:
			got := d.(*agui.TextMessageContentEvent)
			eq(t, "MessageID", got.MessageID, o.MessageID)
			eq(t, "Delta", got.Delta, o.Delta)
		case *agui.ToolCallStartEvent:
			got := d.(*agui.ToolCallStartEvent)
			eq(t, "ToolCallID", got.ToolCallID, o.ToolCallID)
			eq(t, "ToolCallName", got.ToolCallName, o.ToolCallName)
		case *agui.StateSnapshotEvent:
			eq(t, "Snapshot", d.(*agui.StateSnapshotEvent).Snapshot, o.Snapshot)
		case *agui.MessagesSnapshotEvent:
			got := d.(*agui.MessagesSnapshotEvent)
			eq(t, "len(Messages)", len(got.Messages), len(o.Messages))
			eq(t, "Messages[0].ID", got.Messages[0].MessageID(), o.Messages[0].MessageID())
		case *agui.RunStartedEvent:
			got := d.(*agui.RunStartedEvent)
			eq(t, "ThreadID", got.ThreadID, o.ThreadID)
			eq(t, "RunID", got.RunID, o.RunID)
		}
	}
}

// Python: tests/test_events.py::TestEvents::test_raw_event_with_null_source
func TestPyTestEvents_RawEventWithNullSource(t *testing.T) {
	e := agui.RawEvent{Event: map[string]any{"data": "test"}, Source: nil}
	if e.Source != nil {
		t.Errorf("Source = %v, want nil", *e.Source)
	}
	s := dump(t, e)
	eq(t, "type", s["type"], "RAW")
	eq(t, "event.data", s["event"].(map[string]any)["data"], "test")
	absent(t, s, "source")

	d := parse(t, mustJSON(t, e)).(*agui.RawEvent)
	if d.Source != nil {
		t.Errorf("round-tripped Source = %v, want nil", *d.Source)
	}
}

// Python: tests/test_events.py::TestEvents::test_complex_nested_event_structures
func TestPyTestEvents_ComplexNestedEventStructures(t *testing.T) {
	state := map[string]any{
		"session": map[string]any{
			"user": map[string]any{
				"id": "user_123",
				"preferences": map[string]any{
					"theme": "dark", "notifications": true,
					"filters": []any{"news", "social", "tech"},
				},
			},
			"stats": map[string]any{
				"messages":     42,
				"interactions": map[string]any{"clicks": 18, "searches": 7},
			},
		},
		"active_tools": []any{"search", "calculator", "weather"},
		"settings":     map[string]any{"language": "en", "timezone": "UTC-5"},
	}
	e := agui.StateSnapshotEvent{BaseEvent: agui.BaseEvent{Timestamp: tsPtr()}, Snapshot: state}
	snap := e.Snapshot.(map[string]any)
	session := snap["session"].(map[string]any)
	user := session["user"].(map[string]any)
	eq(t, "session.user.id", user["id"], "user_123")
	eq(t, "session.user.preferences.theme", user["preferences"].(map[string]any)["theme"], "dark")
	eq(t, "session.stats.interactions.searches", session["stats"].(map[string]any)["interactions"].(map[string]any)["searches"], 7)
	eq(t, "active_tools[1]", snap["active_tools"].([]any)[1], "calculator")

	d := parse(t, mustJSON(t, e)).(*agui.StateSnapshotEvent)
	ds := d.Snapshot.(map[string]any)
	prefs := ds["session"].(map[string]any)["user"].(map[string]any)["preferences"].(map[string]any)
	eq(t, "filters", prefs["filters"], []any{"news", "social", "tech"})
	eq(t, "settings.timezone", ds["settings"].(map[string]any)["timezone"], "UTC-5")
}

// Python: tests/test_events.py::TestEvents::test_text_message_start_with_name
func TestPyTestEvents_TextMessageStartWithName(t *testing.T) {
	e := agui.TextMessageStartEvent{MessageID: "msg_123", Name: agui.Ptr("research-agent")}
	eq(t, "Name", *e.Name, "research-agent")
	if e.Role != nil {
		t.Errorf("Role = %v, want nil", *e.Role)
	}
	s := dump(t, e)
	eq(t, "name", s["name"], "research-agent")
	eq(t, "messageId", s["messageId"], "msg_123")
}

// Python: tests/test_events.py::TestEvents::test_text_message_start_without_name
func TestPyTestEvents_TextMessageStartWithoutName(t *testing.T) {
	e := agui.TextMessageStartEvent{MessageID: "msg_123"}
	if e.Name != nil {
		t.Errorf("Name = %v, want nil", *e.Name)
	}
}

// Python: tests/test_events.py::TestEvents::test_text_message_chunk_with_name
func TestPyTestEvents_TextMessageChunkWithName(t *testing.T) {
	e := agui.TextMessageChunkEvent{MessageID: agui.Ptr("msg_123"), Delta: agui.Ptr("Hello"), Name: agui.Ptr("research-agent")}
	eq(t, "Name", *e.Name, "research-agent")
	s := dump(t, e)
	eq(t, "name", s["name"], "research-agent")
}

// Python: tests/test_events.py::TestEvents::test_text_message_chunk_without_name
func TestPyTestEvents_TextMessageChunkWithoutName(t *testing.T) {
	e := agui.TextMessageChunkEvent{MessageID: agui.Ptr("msg_123"), Delta: agui.Ptr("Hello")}
	if e.Name != nil {
		t.Errorf("Name = %v, want nil", *e.Name)
	}
}

// Python: tests/test_events.py::TestEvents::test_event_with_unicode_and_special_chars
func TestPyTestEvents_EventWithUnicodeAndSpecialChars(t *testing.T) {
	text := "Hello 你好 こんにちは 안녕하세요 👋 🌍 \n\t\"'\\/<>{}[]"
	e := agui.TextMessageContentEvent{BaseEvent: agui.BaseEvent{Timestamp: tsPtr()}, MessageID: "msg_unicode", Delta: text}
	eq(t, "Delta", e.Delta, text)
	d := parse(t, mustJSON(t, e)).(*agui.TextMessageContentEvent)
	eq(t, "round-tripped Delta", d.Delta, text)
}

// Python: tests/test_events.py::TestEvents::test_all_event_subclasses_in_event_union
//
// Go analogue of "every event class is in the Event union": every event type
// of the protocol has a concrete Go type reachable through NewEvent (the
// decoder's dispatch), reports its own type, and is a member the schema's
// Event union describes.
func TestPyTestEvents_AllEventSubclassesInEventUnion(t *testing.T) {
	defs := map[string]bool{}
	for _, d := range agui.Definitions() {
		defs[d] = true
	}
	if len(agui.EventTypes) != 31 {
		t.Errorf("len(EventTypes) = %d, want 31 (AG-UI 1.0)", len(agui.EventTypes))
	}
	for _, et := range agui.EventTypes {
		e, err := agui.NewEvent(et)
		if err != nil {
			t.Errorf("NewEvent(%s): %v", et, err)
			continue
		}
		eq(t, "Type()", e.Type(), et)
		name := reflect.TypeOf(e).Elem().Name()
		if !defs[name] {
			t.Errorf("%s: Go type %s has no schema definition of that name", et, name)
		}
		// Decoding the type tag alone must dispatch to the same Go type.
		d, err := agui.UnmarshalEvent([]byte(`{"type":"` + string(et) + `"}`))
		if err != nil {
			t.Errorf("UnmarshalEvent(%s): %v", et, err)
			continue
		}
		if reflect.TypeOf(d) != reflect.TypeOf(e) {
			t.Errorf("%s dispatches to %T, want %T", et, d, e)
		}
	}
}

// Python: tests/test_events.py::TestEvents::test_reasoning_message_start_event_role_is_reasoning
func TestPyTestEvents_ReasoningMessageStartEventRoleIsReasoning(t *testing.T) {
	// The Go type has no Role field: the role is fixed and always emitted.
	e := agui.ReasoningMessageStartEvent{BaseEvent: agui.BaseEvent{Timestamp: tsPtr()}, MessageID: "msg_reasoning_1"}
	eq(t, "MessageID", e.MessageID, "msg_reasoning_1")
	s := dump(t, e)
	eq(t, "role", s["role"], "reasoning")
	eq(t, "type", s["type"], "REASONING_MESSAGE_START")
	if err := agui.ValidateEvent(&e); err != nil {
		t.Errorf("ValidateEvent: %v", err)
	}

	d := parse(t, []byte(`{"type":"REASONING_MESSAGE_START","messageId":"msg_reasoning_2","role":"reasoning","timestamp":1648214400000}`))
	re, ok := d.(*agui.ReasoningMessageStartEvent)
	if !ok {
		t.Fatalf("decoded as %T", d)
	}
	eq(t, "MessageID", re.MessageID, "msg_reasoning_2")
	eq(t, "role on re-marshal", dump(t, re)["role"], "reasoning")
}

// Python: tests/test_events.py::TestEvents::test_reasoning_message_start_event_rejects_assistant_role
//
// The Go type cannot even hold role "assistant"; the wire document carrying
// it must be rejected by the strict schema check.
func TestPyTestEvents_ReasoningMessageStartEventRejectsAssistantRole(t *testing.T) {
	doc := []byte(`{"type":"REASONING_MESSAGE_START","messageId":"msg_bad","role":"assistant","timestamp":1648214400000}`)
	if err := agui.Validate("ReasoningMessageStartEvent", doc); err == nil {
		t.Error("Validate(ReasoningMessageStartEvent) accepted role assistant")
	}
	if _, err := agui.ParseEvent(doc); err == nil {
		t.Error("ParseEvent accepted REASONING_MESSAGE_START with role assistant")
	}
}
