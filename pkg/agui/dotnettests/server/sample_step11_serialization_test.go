package server_test

// Port of sdks/dotnet/tests/AGUI.Hosting.AspNetCore.IntegrationTests/Samples/GettingStarted/Step11_SerializationTest.cs.

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
)

// roundTripEvents marshals events, parses each document back, and checks the
// re-marshalled documents are identical.
func roundTripEvents(t *testing.T, events []agui.Event) {
	t.Helper()
	for i, e := range events {
		first, err := json.Marshal(e)
		if err != nil {
			t.Fatalf("event %d: %v", i, err)
		}
		back, err := agui.ParseEvent(first)
		if err != nil {
			t.Fatalf("event %d %s does not parse back: %v\n%s", i, e.Type(), err, first)
		}
		if reflect.TypeOf(back) != reflect.TypeOf(e) || back.Type() != e.Type() {
			t.Fatalf("event %d: %T became %T", i, e, back)
		}
		second, _ := json.Marshal(back)
		if string(first) != string(second) {
			t.Fatalf("event %d does not round-trip\n first: %s\nsecond: %s", i, first, second)
		}
	}
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/Samples/GettingStarted/Step11_SerializationTest.cs::Step11_SerializationTest.PostRun_EventsSerializeAndDeserializeRoundTrip
func TestCsStep11_SerializationTest_PostRun_EventsSerializeAndDeserializeRoundTrip(t *testing.T) {
	r := newSampleRun(t, "Step11_Serialization")
	for _, m := range r.baselineMessages(1) {
		r.client.AddMessage(m)
	}
	res1 := r.runTurn(1, client.RunInput{})
	r.assertRequest(1, res1)
	turn1RunID := res1.Request.RunID

	// Turn 2 continues the thread as a child of turn 1.
	turn2 := r.baselineMessages(2)
	r.client.AddMessage(turn2[len(turn2)-1])
	res2 := r.runTurn(2, client.RunInput{ParentRunID: agui.Ptr(turn1RunID)})

	roundTripEvents(t, append(append([]agui.Event{}, res1.Events...), res2.Events...))

	if agui.Deref(res2.Request.ParentRunID) != turn1RunID {
		t.Fatalf("turn 2 parentRunId %v, want %s", res2.Request.ParentRunID, turn1RunID)
	}
	if p := agui.Deref(res2.Events[0].(*agui.RunStartedEvent).ParentRunID); p != turn1RunID {
		t.Fatalf("turn 2 RUN_STARTED parentRunId %q", p)
	}
	// Adapted: the .NET sample sends only the new message on turn 2 and has
	// the server rebuild the history. run-input.mdx: "A producer MUST treat
	// [messages] as the complete history it is being shown — the protocol has
	// no side channel through which earlier turns arrive." The Go client sends
	// the whole thread; its tail is the baseline's single new message.
	r.assertRequestWith(2, res2, func(want map[string]any) {
		prev := normalizedRequest(t, toGeneric(t, res1.Request))["messages"].([]any)
		assistant := map[string]any{"role": "assistant", "content": lastAssistantTextOf(res1.Events)}
		want["messages"] = append(append(prev, assistant), want["messages"].([]any)...)
	})
	// The server received the combined history.
	if n := len(r.server.inputs[1].Messages); n <= 1 {
		t.Fatalf("server received %d messages on turn 2", n)
	}
}

func lastAssistantTextOf(events []agui.Event) string { return assistantText(events) }

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/Samples/GettingStarted/Step11_SerializationTest.cs::Step11_SerializationTest.AllEventTypes_SerializeAndDeserializeRoundTrip
func TestCsStep11_SerializationTest_AllEventTypes_SerializeAndDeserializeRoundTrip(t *testing.T) {
	// One instance of every event type of the C# list. Adapted: the C#
	// REASONING_START/END carry no messageId, which the 1.0 schema requires.
	toolResult := agui.NewToolCallResultEvent("call_1", "call_1", `{"temp":72,"condition":"sunny"}`)
	interrupted := agui.NewRunFinishedEvent("t1", "r1")
	interrupted.Outcome = &agui.InterruptOutcome{Interrupts: []agui.Interrupt{{
		ID: "int-1", Reason: "tool_call", ToolCallID: agui.Ptr("call_1"), Message: agui.Ptr("Approval required for tool call: get_weather"),
	}}}
	started := agui.NewRunStartedEvent("t1", "r1")
	started.ParentRunID = agui.Ptr("r0")
	started.Input = &agui.RunAgentInput{ThreadID: "t1", RunID: "r1", Messages: agui.Messages{
		&agui.UserMessage{ID: "msg1", Content: agui.PartsContent(agui.NewTextPart("Hello"))}}}
	toolStart := agui.NewToolCallStartEvent("call_1", "get_weather")
	toolStart.ParentMessageID = agui.Ptr("m1")
	runError := agui.NewRunErrorEvent("Something went wrong")
	runError.Code = agui.Ptr("INTERNAL_ERROR")
	events := []agui.Event{
		started,
		agui.NewStepStartedEvent("step-1"),
		agui.NewTextMessageStartEvent("m1"),
		agui.NewTextMessageContentEvent("m1", "Hello world"),
		agui.NewTextMessageEndEvent("m1"),
		toolStart,
		agui.NewToolCallArgsEvent("call_1", `{"city":"Seattle"}`),
		agui.NewToolCallEndEvent("call_1"),
		toolResult,
		agui.NewStateSnapshotEvent(map[string]any{"counter": 1, "items": []any{"a", "b"}}),
		agui.NewStateDeltaEvent(agui.Replace("/counter", 2)),
		agui.NewMessagesSnapshotEvent(
			&agui.UserMessage{ID: "msg1", Content: agui.PartsContent(agui.NewTextPart("Hello"))},
			&agui.AssistantMessage{ID: "msg2", Content: agui.Ptr("Hello world")}),
		agui.NewReasoningStartEvent("r-span"),
		agui.NewReasoningMessageStartEvent("rm1"),
		agui.NewReasoningMessageContentEvent("rm1", "Let me think..."),
		agui.NewReasoningMessageChunkEvent("rm1", "...about this"),
		agui.NewReasoningMessageEndEvent("rm1"),
		agui.NewReasoningEncryptedValueEvent(agui.EncryptedValueSubtypeMessage, "e1", "encrypted-base64-data"),
		agui.NewReasoningEndEvent("r-span"),
		agui.NewActivitySnapshotEvent("act-1", "search", map[string]any{"title": "Searching...", "description": "Searching for weather data", "state": "in_progress"}),
		agui.NewActivityDeltaEvent("act-1", "search", agui.Replace("/state", "completed")),
		agui.NewCustomEvent("my-custom-event", map[string]any{"key": "value"}),
		agui.NewStepFinishedEvent("step-1"),
		interrupted,
		agui.NewRunFinishedEvent("t1", "r1"),
		runError,
	}
	for i, e := range events {
		if err := agui.ValidateEvent(e); err != nil {
			t.Fatalf("event %d %s: %v", i, e.Type(), err)
		}
	}
	roundTripEvents(t, events)
}
