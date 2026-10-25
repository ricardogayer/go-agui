package protobuf_test

// Port of tests/AGUI.Protobuf.UnitTests/SubagentRoundTripTest.cs: the
// subagent events and subagentRunId over the binary transport.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// C#: tests/AGUI.Protobuf.UnitTests/SubagentRoundTripTest.cs::SubagentRoundTripTest.SubagentStarted_RoundTrips_WithEveryField
func TestCsSubagentRoundTripTest_SubagentStarted_RoundTrips_WithEveryField(t *testing.T) {
	e := &agui.SubagentStartedEvent{
		SubagentRunID:       "sub-1",
		Name:                "researcher",
		Description:         agui.Ptr("digs through sources"),
		ParentSubagentRunID: agui.Ptr("sub-outer"),
		ParentToolCallID:    agui.Ptr("call-9"),
		ParentMessageID:     agui.Ptr("msg-3"),
	}
	e.Timestamp = agui.Ptr[int64](1234567890)
	result := roundTrip(t, e)
	eq(t, "SubagentRunID", result.SubagentRunID, "sub-1")
	eq(t, "Name", result.Name, "researcher")
	eq(t, "Description", deref(t, "Description", result.Description), "digs through sources")
	eq(t, "ParentSubagentRunID", deref(t, "ParentSubagentRunID", result.ParentSubagentRunID), "sub-outer")
	eq(t, "ParentToolCallID", deref(t, "ParentToolCallID", result.ParentToolCallID), "call-9")
	eq(t, "ParentMessageID", deref(t, "ParentMessageID", result.ParentMessageID), "msg-3")
	eq(t, "Timestamp", deref(t, "Timestamp", result.Timestamp), 1234567890)
}

// C#: tests/AGUI.Protobuf.UnitTests/SubagentRoundTripTest.cs::SubagentRoundTripTest.SubagentStarted_RoundTrips_WithOnlyRequiredFields
func TestCsSubagentRoundTripTest_SubagentStarted_RoundTrips_WithOnlyRequiredFields(t *testing.T) {
	// Absent optionals must come back absent, not as empty strings.
	result := roundTrip(t, &agui.SubagentStartedEvent{SubagentRunID: "sub-1", Name: "researcher"})
	eq(t, "SubagentRunID", result.SubagentRunID, "sub-1")
	isNil(t, "Description", result.Description)
	isNil(t, "ParentSubagentRunID", result.ParentSubagentRunID)
	isNil(t, "ParentToolCallID", result.ParentToolCallID)
	isNil(t, "ParentMessageID", result.ParentMessageID)
}

// C#: tests/AGUI.Protobuf.UnitTests/SubagentRoundTripTest.cs::SubagentRoundTripTest.SubagentFinished_RoundTrips_WithAndWithoutResult
func TestCsSubagentRoundTripTest_SubagentFinished_RoundTrips_WithAndWithoutResult(t *testing.T) {
	withResult := roundTrip(t, &agui.SubagentFinishedEvent{SubagentRunID: "sub-1", Result: parseJSON(t, `{"answer":42}`)})
	eq(t, "SubagentRunID", withResult.SubagentRunID, "sub-1")
	assertJSONEqual(t, parseJSON(t, `{"answer":42}`), withResult.Result)

	bare := roundTrip(t, &agui.SubagentFinishedEvent{SubagentRunID: "sub-1"})
	if bare.Result != nil {
		t.Fatalf("Result = %v, want nil", bare.Result)
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/SubagentRoundTripTest.cs::SubagentRoundTripTest.SubagentFinished_RoundTrips_Outcomes
func TestCsSubagentRoundTripTest_SubagentFinished_RoundTrips_Outcomes(t *testing.T) {
	// Legacy: no outcome comes back as no outcome.
	legacy := roundTrip(t, &agui.SubagentFinishedEvent{SubagentRunID: "sub-1"})
	if legacy.Outcome != nil {
		t.Fatalf("Outcome = %#v, want nil", legacy.Outcome)
	}

	success := roundTrip(t, &agui.SubagentFinishedEvent{SubagentRunID: "sub-1", Outcome: &agui.SubagentSuccessOutcome{}})
	asType[*agui.SubagentSuccessOutcome](t, success.Outcome)

	suspended := roundTrip(t, &agui.SubagentFinishedEvent{SubagentRunID: "sub-1",
		Outcome: &agui.SubagentSuspendedOutcome{InterruptIDs: []string{"int-1", "int-2"}}})
	assertJSONEqual(t, []string{"int-1", "int-2"}, asType[*agui.SubagentSuspendedOutcome](t, suspended.Outcome).InterruptIDs)

	// An ancestor suspended by a descendant owns no interrupts itself.
	ancestor := roundTrip(t, &agui.SubagentFinishedEvent{SubagentRunID: "sub-outer", Outcome: &agui.SubagentSuspendedOutcome{}})
	if ids := asType[*agui.SubagentSuspendedOutcome](t, ancestor.Outcome).InterruptIDs; ids != nil {
		t.Fatalf("InterruptIDs = %v, want nil", ids)
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/SubagentRoundTripTest.cs::SubagentRoundTripTest.Interrupt_SubagentRunId_SurvivesTheRunFinishedOutcome
func TestCsSubagentRoundTripTest_Interrupt_SubagentRunId_SurvivesTheRunFinishedOutcome(t *testing.T) {
	result := roundTrip(t, &agui.RunFinishedEvent{ThreadID: "t1", RunID: "r1",
		Outcome: &agui.InterruptOutcome{Interrupts: []agui.Interrupt{
			{ID: "int-1", Reason: "hitl", SubagentRunID: agui.Ptr("tools:s1")},
			{ID: "int-2", Reason: "hitl"},
		}}})
	outcome := asType[*agui.InterruptOutcome](t, result.Outcome)
	eq(t, "Interrupts[0].SubagentRunID", deref(t, "SubagentRunID", outcome.Interrupts[0].SubagentRunID), "tools:s1")
	isNil(t, "Interrupts[1].SubagentRunID", outcome.Interrupts[1].SubagentRunID)
}

// C#: tests/AGUI.Protobuf.UnitTests/SubagentRoundTripTest.cs::SubagentRoundTripTest.SubagentError_RoundTrips_WithAndWithoutCode
func TestCsSubagentRoundTripTest_SubagentError_RoundTrips_WithAndWithoutCode(t *testing.T) {
	withCode := roundTrip(t, &agui.SubagentErrorEvent{SubagentRunID: "sub-1", Message: "the subagent exploded", Code: agui.Ptr("E_BOOM")})
	eq(t, "Message", withCode.Message, "the subagent exploded")
	eq(t, "Code", deref(t, "Code", withCode.Code), "E_BOOM")

	bare := roundTrip(t, &agui.SubagentErrorEvent{SubagentRunID: "sub-1", Message: "boom"})
	isNil(t, "Code", bare.Code)
}

// C#: tests/AGUI.Protobuf.UnitTests/SubagentRoundTripTest.cs::SubagentRoundTripTest.SubagentRunId_SurvivesOnEveryAttributableEvent
func TestCsSubagentRoundTripTest_SubagentRunId_SurvivesOnEveryAttributableEvent(t *testing.T) {
	s1 := agui.Attribution{SubagentRunID: agui.Ptr("s1")}
	events := []agui.Event{
		&agui.TextMessageStartEvent{Attribution: s1, MessageID: "m1", Role: agui.Ptr(agui.TextMessageRoleAssistant)},
		&agui.TextMessageContentEvent{Attribution: s1, MessageID: "m1", Delta: "hi"},
		&agui.TextMessageEndEvent{Attribution: s1, MessageID: "m1"},
		&agui.ToolCallStartEvent{Attribution: s1, ToolCallID: "tc1", ToolCallName: "search"},
		&agui.ToolCallArgsEvent{Attribution: s1, ToolCallID: "tc1", Delta: "{}"},
		&agui.ToolCallEndEvent{Attribution: s1, ToolCallID: "tc1"},
		&agui.StepStartedEvent{Attribution: s1, StepName: "step"},
		&agui.StepFinishedEvent{Attribution: s1, StepName: "step"},
		&agui.CustomEvent{Attribution: s1, Name: "thing"},
		&agui.RawEvent{Attribution: s1, Event: map[string]any{}},
	}
	for _, e := range events {
		t.Run(string(e.Type()), func(t *testing.T) {
			got := decode(t, encode(t, e))
			eq(t, "type", got.Type(), e.Type())
			eq(t, "SubagentRunID", agui.SubagentOf(got), "s1")
		})
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/SubagentRoundTripTest.cs::SubagentRoundTripTest.SubagentRunId_StaysNullWhenAbsent
func TestCsSubagentRoundTripTest_SubagentRunId_StaysNullWhenAbsent(t *testing.T) {
	// A round trip must not invent an empty-string attribution.
	isNil(t, "TEXT_MESSAGE_START", roundTrip(t, &agui.TextMessageStartEvent{MessageID: "m1", Role: agui.Ptr(agui.TextMessageRoleAssistant)}).SubagentRunID)
	isNil(t, "TOOL_CALL_START", roundTrip(t, &agui.ToolCallStartEvent{ToolCallID: "tc1", ToolCallName: "search"}).SubagentRunID)
	isNil(t, "STEP_STARTED", roundTrip(t, &agui.StepStartedEvent{StepName: "step"}).SubagentRunID)
}

// C#: tests/AGUI.Protobuf.UnitTests/SubagentRoundTripTest.cs::SubagentRoundTripTest.Messages_KeepPerMessageSubagentRunId_ThroughSnapshot
func TestCsSubagentRoundTripTest_Messages_KeepPerMessageSubagentRunId_ThroughSnapshot(t *testing.T) {
	result := roundTrip(t, &agui.MessagesSnapshotEvent{Messages: agui.Messages{
		&agui.AssistantMessage{ID: "m1", Content: agui.Ptr("parent speaking")},
		&agui.AssistantMessage{ID: "m2", Content: agui.Ptr("subagent speaking"), SubagentRunID: agui.Ptr("sub-1")},
		&agui.ToolMessage{ID: "m3", Content: agui.TextContent("done"), ToolCallID: "tc1", SubagentRunID: agui.Ptr("sub-2")},
	}})
	isNil(t, "Messages[0].SubagentRunID", asType[*agui.AssistantMessage](t, result.Messages[0]).SubagentRunID)
	eq(t, "Messages[1].SubagentRunID", deref(t, "SubagentRunID", asType[*agui.AssistantMessage](t, result.Messages[1]).SubagentRunID), "sub-1")
	eq(t, "Messages[2].SubagentRunID", deref(t, "SubagentRunID", asType[*agui.ToolMessage](t, result.Messages[2]).SubagentRunID), "sub-2")
}

// requiredMembersStay checks the Go-side equivalent of the .NET encoder's
// "'<field>' is required" guard. The Go model's required members are plain
// strings, so an event cannot reach the encoder without them (a Go zero value
// is "", which the schema accepts — see EmptyRequiredFields_StillEncode); a
// JSON document that omits one is rejected by the schema with a message
// naming the event and the member.
func requiredMembersStay(t *testing.T, e agui.Event, doc, field string) {
	t.Helper()
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"`+field+`":`) {
		t.Fatalf("%s marshals without %q: %s", e.Type(), field, b)
	}
	err = agui.Validate("Event", []byte(doc))
	if err == nil {
		t.Fatalf("%s without %q validated", e.Type(), field)
	}
	if !strings.Contains(err.Error(), field) {
		t.Fatalf("the error %q does not name %q", err, field)
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/SubagentRoundTripTest.cs::SubagentRoundTripTest.SubagentStarted_WithoutItsRequiredFields_FailsWithADescriptiveMessage
func TestCsSubagentRoundTripTest_SubagentStarted_WithoutItsRequiredFields_FailsWithADescriptiveMessage(t *testing.T) {
	// Adapted (see requiredMembersStay): .NET's nullable model needs an
	// encode-time guard; the TypeScript reference encodes an invalid event
	// "as given" with a warning, so the spec asks nothing of the encoder here.
	requiredMembersStay(t, &agui.SubagentStartedEvent{Name: "researcher"}, `{"type":"SUBAGENT_STARTED","name":"researcher"}`, "subagentRunId")
	requiredMembersStay(t, &agui.SubagentStartedEvent{SubagentRunID: "s1"}, `{"type":"SUBAGENT_STARTED","subagentRunId":"s1"}`, "name")
}

// C#: tests/AGUI.Protobuf.UnitTests/SubagentRoundTripTest.cs::SubagentRoundTripTest.SubagentFinished_WithoutItsRequiredField_FailsWithADescriptiveMessage
func TestCsSubagentRoundTripTest_SubagentFinished_WithoutItsRequiredField_FailsWithADescriptiveMessage(t *testing.T) {
	// Adapted: see SubagentStarted_WithoutItsRequiredFields.
	requiredMembersStay(t, &agui.SubagentFinishedEvent{}, `{"type":"SUBAGENT_FINISHED"}`, "subagentRunId")
}

// C#: tests/AGUI.Protobuf.UnitTests/SubagentRoundTripTest.cs::SubagentRoundTripTest.SubagentError_WithoutItsRequiredFields_FailsWithADescriptiveMessage
func TestCsSubagentRoundTripTest_SubagentError_WithoutItsRequiredFields_FailsWithADescriptiveMessage(t *testing.T) {
	// Adapted: see SubagentStarted_WithoutItsRequiredFields.
	requiredMembersStay(t, &agui.SubagentErrorEvent{Message: "boom"}, `{"type":"SUBAGENT_ERROR","message":"boom"}`, "subagentRunId")
	requiredMembersStay(t, &agui.SubagentErrorEvent{SubagentRunID: "s1"}, `{"type":"SUBAGENT_ERROR","subagentRunId":"s1"}`, "message")
}

// C#: tests/AGUI.Protobuf.UnitTests/SubagentRoundTripTest.cs::SubagentRoundTripTest.EmptyRequiredFields_StillEncode
func TestCsSubagentRoundTripTest_EmptyRequiredFields_StillEncode(t *testing.T) {
	// "" is a value the schemas accept, so only an ABSENT field is an error.
	result := roundTrip(t, &agui.SubagentStartedEvent{SubagentRunID: "", Name: ""})
	eq(t, "SubagentRunID", result.SubagentRunID, "")
	eq(t, "Name", result.Name, "")
	doc := roundTripDoc(t, `{"type":"SUBAGENT_STARTED","subagentRunId":"","name":""}`)
	if doc["subagentRunId"] != "" || doc["name"] != "" {
		t.Fatalf("decoded %v, want both members present and empty", doc)
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/SubagentRoundTripTest.cs::SubagentRoundTripTest.StateEvents_PreserveAttribution_SoNonConformanceStaysDiagnosable
func TestCsSubagentRoundTripTest_StateEvents_PreserveAttribution_SoNonConformanceStaysDiagnosable(t *testing.T) {
	s1 := agui.Attribution{SubagentRunID: agui.Ptr("s1")}
	snap := roundTrip(t, &agui.StateSnapshotEvent{Attribution: s1, Snapshot: parseJSON(t, `{"a":1}`)})
	eq(t, "STATE_SNAPSHOT", deref(t, "SubagentRunID", snap.SubagentRunID), "s1")
	delta := roundTrip(t, &agui.StateDeltaEvent{Attribution: s1, Delta: agui.JSONPatch{}})
	eq(t, "STATE_DELTA", deref(t, "SubagentRunID", delta.SubagentRunID), "s1")
}

// C#: tests/AGUI.Protobuf.UnitTests/SubagentRoundTripTest.cs::SubagentRoundTripTest.ParentMessageId_Null_StillDecodes
func TestCsSubagentRoundTripTest_ParentMessageId_Null_StillDecodes(t *testing.T) {
	// The interop regression PNI-200 warned about: a producer that emits
	// "parentMessageId": null. Typed, nil is absent; as a document the
	// explicit null must encode and come back absent.
	result := roundTrip(t, &agui.ToolCallStartEvent{Attribution: agui.Attribution{SubagentRunID: agui.Ptr("s1")},
		ToolCallID: "tc1", ToolCallName: "search", ParentMessageID: nil})
	isNil(t, "ParentMessageID", result.ParentMessageID)
	eq(t, "SubagentRunID", deref(t, "SubagentRunID", result.SubagentRunID), "s1")

	doc := roundTripDoc(t, `{"type":"TOOL_CALL_START","toolCallId":"tc1","toolCallName":"search","parentMessageId":null,"subagentRunId":"s1"}`)
	if _, ok := doc["parentMessageId"]; ok {
		t.Fatalf("parentMessageId survived as %v, want absent", doc["parentMessageId"])
	}
	eq(t, "subagentRunId", doc["subagentRunId"], any("s1"))
}
