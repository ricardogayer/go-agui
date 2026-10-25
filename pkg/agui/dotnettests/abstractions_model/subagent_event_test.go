package abstractions_model_test

// Port of sdks/dotnet/tests/AGUI.Abstractions.UnitTests/SubagentEventTest.cs
// (classes SubagentEventTest and SubagentAttributionTest).

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// ---------------------------------------------------------------------------
// SubagentEventTest
// ---------------------------------------------------------------------------

// C#: tests/AGUI.Abstractions.UnitTests/SubagentEventTest.cs::SubagentEventTest.SubagentStarted_Serialize_IncludesAllFields
func TestCsSubagentEventTest_SubagentStarted_Serialize_IncludesAllFields(t *testing.T) {
	o := obj(t, &agui.SubagentStartedEvent{
		SubagentRunID: "sub-1", Name: "researcher", Description: agui.Ptr("digs through sources"),
		ParentSubagentRunID: agui.Ptr("sub-outer"), ParentToolCallID: agui.Ptr("call-9"), ParentMessageID: agui.Ptr("msg-3"),
	})
	assertEq(t, o["type"], "SUBAGENT_STARTED", "type")
	assertEq(t, o["subagentRunId"], "sub-1", "subagentRunId")
	assertEq(t, o["name"], "researcher", "name")
	assertEq(t, o["description"], "digs through sources", "description")
	assertEq(t, o["parentSubagentRunId"], "sub-outer", "parentSubagentRunId")
	assertEq(t, o["parentToolCallId"], "call-9", "parentToolCallId")
	assertEq(t, o["parentMessageId"], "msg-3", "parentMessageId")
}

// C#: tests/AGUI.Abstractions.UnitTests/SubagentEventTest.cs::SubagentEventTest.SubagentStarted_OmitsAbsentOptionals
func TestCsSubagentEventTest_SubagentStarted_OmitsAbsentOptionals(t *testing.T) {
	o := obj(t, &agui.SubagentStartedEvent{SubagentRunID: "sub-1", Name: "researcher"})
	assertOmitted(t, o, "description", "parentSubagentRunId", "parentToolCallId", "parentMessageId")
}

// C#: tests/AGUI.Abstractions.UnitTests/SubagentEventTest.cs::SubagentEventTest.SubagentFinished_RoundTripsWithAndWithoutResult
func TestCsSubagentEventTest_SubagentFinished_RoundTripsWithAndWithoutResult(t *testing.T) {
	back := roundTripEvent(t, &agui.SubagentFinishedEvent{SubagentRunID: "sub-1", Result: decode(t, []byte(`{"answer":42}`))}).(*agui.SubagentFinishedEvent)
	assertEq(t, back.SubagentRunID, "sub-1", "subagentRunId")
	if back.Result == nil {
		t.Fatal("result lost")
	}
	assertEq(t, objOf(t, back.Result)["answer"], float64(42), "result.answer")

	assertOmitted(t, obj(t, &agui.SubagentFinishedEvent{SubagentRunID: "sub-1"}), "result")
}

// C#: tests/AGUI.Abstractions.UnitTests/SubagentEventTest.cs::SubagentEventTest.SubagentFinished_RoundTripsOutcomes
func TestCsSubagentEventTest_SubagentFinished_RoundTripsOutcomes(t *testing.T) {
	assertOmitted(t, obj(t, &agui.SubagentFinishedEvent{SubagentRunID: "sub-1"}), "outcome")

	suspended := &agui.SubagentFinishedEvent{SubagentRunID: "sub-1", Outcome: &agui.SubagentSuspendedOutcome{InterruptIDs: []string{"int-1"}}}
	js := string(mustJSON(t, suspended))
	if !strings.Contains(js, `"suspended"`) || !strings.Contains(js, `"interruptIds"`) {
		t.Fatalf("suspended outcome not on the wire: %s", js)
	}
	back := roundTripEvent(t, suspended).(*agui.SubagentFinishedEvent)
	s, ok := back.Outcome.(*agui.SubagentSuspendedOutcome)
	if !ok {
		t.Fatalf("expected *agui.SubagentSuspendedOutcome, got %T", back.Outcome)
	}
	assertEq(t, s.InterruptIDs, []string{"int-1"}, "interruptIds")

	success := roundTripEvent(t, &agui.SubagentFinishedEvent{SubagentRunID: "sub-1", Outcome: &agui.SubagentSuccessOutcome{}}).(*agui.SubagentFinishedEvent)
	if _, ok := success.Outcome.(*agui.SubagentSuccessOutcome); !ok {
		t.Fatalf("expected *agui.SubagentSuccessOutcome, got %T", success.Outcome)
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/SubagentEventTest.cs::SubagentEventTest.Interrupt_CarriesTheRaisingSubagent
func TestCsSubagentEventTest_Interrupt_CarriesTheRaisingSubagent(t *testing.T) {
	owned := agui.Interrupt{ID: "int-1", Reason: "hitl", SubagentRunID: agui.Ptr("tools:s1")}
	js := mustJSON(t, owned)
	if !strings.Contains(string(js), `"subagentRunId":"tools:s1"`) {
		t.Fatalf("subagentRunId missing: %s", js)
	}
	var back agui.Interrupt
	if err := json.Unmarshal(js, &back); err != nil {
		t.Fatal(err)
	}
	assertEq(t, agui.Deref(back.SubagentRunID), "tools:s1", "subagentRunId")

	root := string(mustJSON(t, agui.Interrupt{ID: "int-2", Reason: "hitl"}))
	if strings.Contains(root, "subagentRunId") {
		t.Fatalf("subagentRunId written for a root interrupt: %s", root)
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/SubagentEventTest.cs::SubagentEventTest.SubagentError_RoundTripsWithAndWithoutCode
func TestCsSubagentEventTest_SubagentError_RoundTripsWithAndWithoutCode(t *testing.T) {
	back := roundTripEvent(t, &agui.SubagentErrorEvent{SubagentRunID: "sub-1", Message: "the subagent exploded", Code: agui.Ptr("E_BOOM")}).(*agui.SubagentErrorEvent)
	assertEq(t, back.Message, "the subagent exploded", "message")
	assertEq(t, agui.Deref(back.Code), "E_BOOM", "code")

	assertOmitted(t, obj(t, &agui.SubagentErrorEvent{SubagentRunID: "sub-1", Message: "boom"}), "code")
}

// C#: tests/AGUI.Abstractions.UnitTests/SubagentEventTest.cs::SubagentEventTest.Deserialize_ViaBaseEvent_ReturnsCorrectType
func TestCsSubagentEventTest_Deserialize_ViaBaseEvent_ReturnsCorrectType(t *testing.T) {
	rows := []struct {
		json string
		want reflect.Type
	}{
		{`{"type":"SUBAGENT_STARTED","subagentRunId":"s","name":"n"}`, reflect.TypeOf(&agui.SubagentStartedEvent{})},
		{`{"type":"SUBAGENT_FINISHED","subagentRunId":"s"}`, reflect.TypeOf(&agui.SubagentFinishedEvent{})},
		{`{"type":"SUBAGENT_ERROR","subagentRunId":"s","message":"m"}`, reflect.TypeOf(&agui.SubagentErrorEvent{})},
	}
	for _, r := range rows {
		t.Run(r.want.Elem().Name(), func(t *testing.T) {
			// agui.ParseEvent is the schema-checked polymorphic decode.
			e, err := agui.ParseEvent([]byte(r.json))
			if err != nil {
				t.Fatalf("ParseEvent: %v", err)
			}
			assertEq(t, reflect.TypeOf(e), r.want, "decoded type")
		})
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/SubagentEventTest.cs::SubagentEventTest.Serialize_ViaBaseEvent_KeepsDiscriminator
func TestCsSubagentEventTest_Serialize_ViaBaseEvent_KeepsDiscriminator(t *testing.T) {
	var e agui.Event = &agui.SubagentStartedEvent{SubagentRunID: "sub-1", Name: "researcher"}
	o := obj(t, e)
	assertEq(t, o["type"], "SUBAGENT_STARTED", "type")
	assertEq(t, o["subagentRunId"], "sub-1", "subagentRunId")
	assertEq(t, o["name"], "researcher", "name")
}

// ---------------------------------------------------------------------------
// SubagentAttributionTest
// ---------------------------------------------------------------------------

// assertAttributionRoundTrips is AssertRoundTrips: through the polymorphic
// event decode, the event comes back as the same type, attributed to s1.
func assertAttributionRoundTrips(t *testing.T, e agui.Event) {
	t.Helper()
	var asEvent agui.Event = e
	back, err := agui.UnmarshalEvent(mustJSON(t, asEvent))
	if err != nil {
		t.Fatalf("%T: %v", e, err)
	}
	if reflect.TypeOf(back) != reflect.TypeOf(e) {
		t.Fatalf("decoded %T, want %T", back, e)
	}
	if got := agui.SubagentOf(back); got != "s1" {
		t.Errorf("%T: subagentRunId = %q, want s1", e, got)
	}
}

func s1() agui.Attribution { return agui.Attribution{SubagentRunID: agui.Ptr("s1")} }

// C#: tests/AGUI.Abstractions.UnitTests/SubagentEventTest.cs::SubagentAttributionTest.TextEvents_CarrySubagentRunId
func TestCsSubagentAttributionTest_TextEvents_CarrySubagentRunId(t *testing.T) {
	assertAttributionRoundTrips(t, &agui.TextMessageStartEvent{Attribution: s1(), MessageID: "m1", Role: agui.Ptr(agui.TextMessageRoleAssistant)})
	assertAttributionRoundTrips(t, &agui.TextMessageContentEvent{Attribution: s1(), MessageID: "m1", Delta: "hi"})
	assertAttributionRoundTrips(t, &agui.TextMessageEndEvent{Attribution: s1(), MessageID: "m1"})
}

// C#: tests/AGUI.Abstractions.UnitTests/SubagentEventTest.cs::SubagentAttributionTest.ToolCallEvents_CarrySubagentRunId
func TestCsSubagentAttributionTest_ToolCallEvents_CarrySubagentRunId(t *testing.T) {
	assertAttributionRoundTrips(t, &agui.ToolCallStartEvent{Attribution: s1(), ToolCallID: "tc1", ToolCallName: "search"})
	assertAttributionRoundTrips(t, &agui.ToolCallArgsEvent{Attribution: s1(), ToolCallID: "tc1", Delta: "{}"})
	assertAttributionRoundTrips(t, &agui.ToolCallEndEvent{Attribution: s1(), ToolCallID: "tc1"})
	assertAttributionRoundTrips(t, &agui.ToolCallResultEvent{Attribution: s1(), MessageID: "m1", ToolCallID: "tc1", Content: agui.TextContent("done")})
}

// C#: tests/AGUI.Abstractions.UnitTests/SubagentEventTest.cs::SubagentAttributionTest.ReasoningEvents_CarrySubagentRunId
func TestCsSubagentAttributionTest_ReasoningEvents_CarrySubagentRunId(t *testing.T) {
	assertAttributionRoundTrips(t, &agui.ReasoningStartEvent{Attribution: s1(), MessageID: "r1"})
	assertAttributionRoundTrips(t, &agui.ReasoningMessageStartEvent{Attribution: s1(), MessageID: "r1"})
	assertAttributionRoundTrips(t, &agui.ReasoningMessageContentEvent{Attribution: s1(), MessageID: "r1", Delta: "think"})
	assertAttributionRoundTrips(t, &agui.ReasoningMessageEndEvent{Attribution: s1(), MessageID: "r1"})
	assertAttributionRoundTrips(t, &agui.ReasoningEndEvent{Attribution: s1(), MessageID: "r1"})
	assertAttributionRoundTrips(t, &agui.ReasoningEncryptedValueEvent{Attribution: s1(), Subtype: agui.EncryptedValueSubtypeMessage, EntityID: "r1", EncryptedValue: "opaque"})
}

// C#: tests/AGUI.Abstractions.UnitTests/SubagentEventTest.cs::SubagentAttributionTest.ActivityAndStepEvents_CarrySubagentRunId
func TestCsSubagentAttributionTest_ActivityAndStepEvents_CarrySubagentRunId(t *testing.T) {
	assertAttributionRoundTrips(t, &agui.StepStartedEvent{Attribution: s1(), StepName: "step"})
	assertAttributionRoundTrips(t, &agui.StepFinishedEvent{Attribution: s1(), StepName: "step"})
	assertAttributionRoundTrips(t, &agui.ActivitySnapshotEvent{Attribution: s1(), MessageID: "a1", ActivityType: "search", Content: map[string]any{}})
}

// C#: tests/AGUI.Abstractions.UnitTests/SubagentEventTest.cs::SubagentAttributionTest.SubagentRunId_IsOmittedWhenAbsent
func TestCsSubagentAttributionTest_SubagentRunId_IsOmittedWhenAbsent(t *testing.T) {
	assertOmitted(t, obj(t, &agui.TextMessageStartEvent{MessageID: "m1", Role: agui.Ptr(agui.TextMessageRoleAssistant)}), "subagentRunId")
}

func messageSubagent(t *testing.T, m agui.Message) *string {
	t.Helper()
	switch x := m.(type) {
	case *agui.DeveloperMessage:
		return x.SubagentRunID
	case *agui.SystemMessage:
		return x.SubagentRunID
	case *agui.AssistantMessage:
		return x.SubagentRunID
	case *agui.UserMessage:
		return x.SubagentRunID
	case *agui.ToolMessage:
		return x.SubagentRunID
	case *agui.ActivityMessage:
		return x.SubagentRunID
	case *agui.ReasoningMessage:
		return x.SubagentRunID
	}
	t.Fatalf("unknown message type %T", m)
	return nil
}

// C#: tests/AGUI.Abstractions.UnitTests/SubagentEventTest.cs::SubagentAttributionTest.Messages_CarrySubagentRunIdOnEveryRole
func TestCsSubagentAttributionTest_Messages_CarrySubagentRunIdOnEveryRole(t *testing.T) {
	snapshot := agui.NewMessagesSnapshotEvent(
		&agui.AssistantMessage{ID: "m1", Content: agui.Ptr("hi"), SubagentRunID: agui.Ptr("s1")},
		&agui.ToolMessage{ID: "m2", Content: agui.TextContent("done"), ToolCallID: "tc1", SubagentRunID: agui.Ptr("s1")},
		&agui.ReasoningMessage{ID: "m3", Content: "think", SubagentRunID: agui.Ptr("s2")},
		&agui.UserMessage{ID: "m4", Content: agui.TextContent("hello"), SubagentRunID: agui.Ptr("s3")},
	)
	back := roundTripEvent(t, snapshot).(*agui.MessagesSnapshotEvent)
	var got []string
	for _, m := range back.Messages {
		got = append(got, agui.Deref(messageSubagent(t, m)))
	}
	assertEq(t, got, []string{"s1", "s1", "s2", "s3"}, "subagentRunIds")
}

// C#: tests/AGUI.Abstractions.UnitTests/SubagentEventTest.cs::SubagentAttributionTest.Messages_OmitSubagentRunIdWhenAbsent
func TestCsSubagentAttributionTest_Messages_OmitSubagentRunIdWhenAbsent(t *testing.T) {
	var m agui.Message = &agui.AssistantMessage{ID: "m1", Content: agui.Ptr("hi")}
	assertOmitted(t, obj(t, m), "subagentRunId")
}

// C#: tests/AGUI.Abstractions.UnitTests/SubagentEventTest.cs::SubagentAttributionTest.SubagentRunId_SurvivesTheChatMessageRoundTrip
//
// The ChatMessage (Microsoft.Extensions.AI) leg has no Go counterpart, nor the
// .NET tool-message rekeying to the call id. The AG-UI-side claim is that
// attribution on request messages reaches the producer: here the messages
// travel in a RunAgentInput and are read with agui.ParseRunAgentInput, as a
// Go server reads a request body.
func TestCsSubagentAttributionTest_SubagentRunId_SurvivesTheChatMessageRoundTrip(t *testing.T) {
	in := agui.RunAgentInput{ThreadID: "t", RunID: "r", Messages: agui.Messages{
		&agui.AssistantMessage{ID: "m1", Content: agui.Ptr("from a subagent"), SubagentRunID: agui.Ptr("s1")},
		&agui.ToolMessage{ID: "m2", Content: agui.TextContent("done"), ToolCallID: "tc1", SubagentRunID: agui.Ptr("s2")},
		&agui.UserMessage{ID: "m3", Content: agui.TextContent("hi"), SubagentRunID: agui.Ptr("s3")},
		&agui.AssistantMessage{ID: "m4", Content: agui.Ptr("from the parent")},
	}}
	back, _, err := agui.ParseRunAgentInput(mustJSON(t, in))
	if err != nil {
		t.Fatalf("ParseRunAgentInput: %v", err)
	}
	byID := map[string]*string{}
	for _, m := range back.Messages {
		byID[m.MessageID()] = messageSubagent(t, m)
	}
	assertEq(t, agui.Deref(byID["m1"]), "s1", "m1")
	assertEq(t, agui.Deref(byID["m2"]), "s2", "m2")
	assertEq(t, agui.Deref(byID["m3"]), "s3", "m3")
	if byID["m4"] != nil {
		t.Errorf("m4 gained attribution %q", *byID["m4"])
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/SubagentEventTest.cs::SubagentAttributionTest.EmptySubagentRunId_SurvivesTheRoundTrip
//
// As above, without the ChatMessage leg: an empty id is a valid opaque id and
// must not collapse into "absent" (parent attribution).
func TestCsSubagentAttributionTest_EmptySubagentRunId_SurvivesTheRoundTrip(t *testing.T) {
	in := agui.RunAgentInput{ThreadID: "t", RunID: "r", Messages: agui.Messages{
		&agui.AssistantMessage{ID: "m1", Content: agui.Ptr("hi"), SubagentRunID: agui.Ptr("")},
	}}
	back, _, err := agui.ParseRunAgentInput(mustJSON(t, in))
	if err != nil {
		t.Fatalf("ParseRunAgentInput: %v", err)
	}
	if len(back.Messages) != 1 {
		t.Fatalf("expected one message, got %d", len(back.Messages))
	}
	got := messageSubagent(t, back.Messages[0])
	if got == nil || *got != "" {
		t.Fatalf("empty subagentRunId must survive as \"\", got %v", got)
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/SubagentEventTest.cs::SubagentAttributionTest.ParallelCallsFromDifferentSubagents_KeepProviderValidGroupingAndLoseTheSecondOwner
func TestCsSubagentAttributionTest_ParallelCallsFromDifferentSubagents_KeepProviderValidGroupingAndLoseTheSecondOwner(t *testing.T) {
	t.Skip("N/A in Go: pins how AsChatMessages coalesces split parallel tool calls into one Microsoft.Extensions.AI ChatMessage and carries the owner in AdditionalProperties; Go has no MEAI conversion layer")
}

// C#: tests/AGUI.Abstractions.UnitTests/SubagentEventTest.cs::SubagentAttributionTest.ParentFirstParallelCalls_StayParentOwned
func TestCsSubagentAttributionTest_ParentFirstParallelCalls_StayParentOwned(t *testing.T) {
	t.Skip("N/A in Go: AsChatMessages merge/attribution rule for Microsoft.Extensions.AI ChatMessage.AdditionalProperties; no MEAI conversion in Go")
}

// C#: tests/AGUI.Abstractions.UnitTests/SubagentEventTest.cs::SubagentAttributionTest.ToolCallRunOwner_IsCapturedAfreshAfterAFlush
func TestCsSubagentAttributionTest_ToolCallRunOwner_IsCapturedAfreshAfterAFlush(t *testing.T) {
	t.Skip("N/A in Go: AsChatMessages tool-call run capture into Microsoft.Extensions.AI ChatMessage.AdditionalProperties; no MEAI conversion in Go")
}

// C#: tests/AGUI.Abstractions.UnitTests/SubagentEventTest.cs::SubagentAttributionTest.UnattributedMessages_DoNotGainAnAdditionalProperty
func TestCsSubagentAttributionTest_UnattributedMessages_DoNotGainAnAdditionalProperty(t *testing.T) {
	t.Skip("N/A in Go: asserts on Microsoft.Extensions.AI ChatMessage.AdditionalProperties; the AG-UI-side omission is covered by Messages_OmitSubagentRunIdWhenAbsent")
}
