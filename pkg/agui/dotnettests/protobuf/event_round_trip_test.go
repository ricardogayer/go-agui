package protobuf_test

// Port of tests/AGUI.Protobuf.UnitTests/EventRoundTripTest.cs: every event
// crosses protobuf.Encode / protobuf.Decode (AGUIProtobuf.Encode / Decode)
// as its Go type and comes back with every field intact.

import (
	"bytes"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

func obj(t *testing.T, s string) map[string]any {
	t.Helper()
	return asType[map[string]any](t, parseJSON(t, s))
}

// C#: tests/AGUI.Protobuf.UnitTests/EventRoundTripTest.cs::EventRoundTripTest.RunStarted_RoundTrips
func TestCsEventRoundTripTest_RunStarted_RoundTrips(t *testing.T) {
	e := &agui.RunStartedEvent{ThreadID: "thread-1", RunID: "run-1"}
	e.Timestamp = agui.Ptr[int64](1234567890)
	e.RawEvent = parseJSON(t, `{"source":"x"}`)
	result := roundTrip(t, e)
	eq(t, "ThreadID", result.ThreadID, "thread-1")
	eq(t, "RunID", result.RunID, "run-1")
	eq(t, "Timestamp", deref(t, "Timestamp", result.Timestamp), 1234567890)
	assertJSONEqual(t, parseJSON(t, `{"source":"x"}`), result.RawEvent)
}

// C#: tests/AGUI.Protobuf.UnitTests/EventRoundTripTest.cs::EventRoundTripTest.RunFinished_Success_RoundTrips
func TestCsEventRoundTripTest_RunFinished_Success_RoundTrips(t *testing.T) {
	result := roundTrip(t, &agui.RunFinishedEvent{
		ThreadID: "thread-1", RunID: "run-1",
		Result:  parseJSON(t, `{"answer":42}`),
		Outcome: &agui.SuccessOutcome{},
	})
	eq(t, "ThreadID", result.ThreadID, "thread-1")
	eq(t, "RunID", result.RunID, "run-1")
	assertJSONEqual(t, parseJSON(t, `{"answer":42}`), result.Result)
	asType[*agui.SuccessOutcome](t, result.Outcome)
}

// C#: tests/AGUI.Protobuf.UnitTests/EventRoundTripTest.cs::EventRoundTripTest.RunFinished_Cancelled_RoundTrips
func TestCsEventRoundTripTest_RunFinished_Cancelled_RoundTrips(t *testing.T) {
	result := roundTrip(t, &agui.RunFinishedEvent{ThreadID: "thread-1", RunID: "run-1", Outcome: &agui.CancelledOutcome{}})
	eq(t, "ThreadID", result.ThreadID, "thread-1")
	eq(t, "RunID", result.RunID, "run-1")
	asType[*agui.CancelledOutcome](t, result.Outcome)
}

// C#: tests/AGUI.Protobuf.UnitTests/EventRoundTripTest.cs::EventRoundTripTest.RunFinished_Interrupt_RoundTrips
func TestCsEventRoundTripTest_RunFinished_Interrupt_RoundTrips(t *testing.T) {
	result := roundTrip(t, &agui.RunFinishedEvent{
		ThreadID: "thread-1", RunID: "run-1",
		Outcome: &agui.InterruptOutcome{Interrupts: []agui.Interrupt{{
			ID:             "int-1",
			Reason:         "tool_call",
			Message:        agui.Ptr("needs approval"),
			ToolCallID:     agui.Ptr("tc-1"),
			ResponseSchema: obj(t, `{"type":"object"}`),
			ExpiresAt:      agui.Ptr("2030-01-01T00:00:00Z"),
			Metadata:       obj(t, `{"k":"v"}`),
		}}},
	})
	outcome := asType[*agui.InterruptOutcome](t, result.Outcome)
	eq(t, "len(Interrupts)", len(outcome.Interrupts), 1)
	in := outcome.Interrupts[0]
	eq(t, "ID", in.ID, "int-1")
	eq(t, "Reason", in.Reason, "tool_call")
	eq(t, "Message", deref(t, "Message", in.Message), "needs approval")
	eq(t, "ToolCallID", deref(t, "ToolCallID", in.ToolCallID), "tc-1")
	eq(t, "ExpiresAt", deref(t, "ExpiresAt", in.ExpiresAt), "2030-01-01T00:00:00Z")
	assertJSONEqual(t, parseJSON(t, `{"type":"object"}`), in.ResponseSchema)
	assertJSONEqual(t, parseJSON(t, `{"k":"v"}`), in.Metadata)
}

// C#: tests/AGUI.Protobuf.UnitTests/EventRoundTripTest.cs::EventRoundTripTest.RunFinished_SuccessWithPendingToolCallIds_RoundTrips
func TestCsEventRoundTripTest_RunFinished_SuccessWithPendingToolCallIds_RoundTrips(t *testing.T) {
	result := roundTrip(t, &agui.RunFinishedEvent{ThreadID: "thread-1", RunID: "run-1",
		Outcome: &agui.SuccessOutcome{PendingToolCallIDs: []string{"tc-1", "tc-2"}}})
	outcome := asType[*agui.SuccessOutcome](t, result.Outcome)
	assertJSONEqual(t, []string{"tc-1", "tc-2"}, outcome.PendingToolCallIDs)
}

// C#: tests/AGUI.Protobuf.UnitTests/EventRoundTripTest.cs::EventRoundTripTest.RunFinished_SuccessWithoutPendingToolCallIds_RoundTripsToNull
func TestCsEventRoundTripTest_RunFinished_SuccessWithoutPendingToolCallIds_RoundTripsToNull(t *testing.T) {
	result := roundTrip(t, &agui.RunFinishedEvent{ThreadID: "thread-1", RunID: "run-1", Outcome: &agui.SuccessOutcome{}})
	outcome := asType[*agui.SuccessOutcome](t, result.Outcome)
	if outcome.PendingToolCallIDs != nil {
		t.Fatalf("PendingToolCallIDs = %v, want nil", outcome.PendingToolCallIDs)
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/EventRoundTripTest.cs::EventRoundTripTest.RunFinished_NoOutcome_RoundTripsToNull
func TestCsEventRoundTripTest_RunFinished_NoOutcome_RoundTripsToNull(t *testing.T) {
	result := roundTrip(t, &agui.RunFinishedEvent{ThreadID: "t", RunID: "r"})
	if result.Outcome != nil {
		t.Fatalf("Outcome = %#v, want nil", result.Outcome)
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/EventRoundTripTest.cs::EventRoundTripTest.RunError_RoundTrips
func TestCsEventRoundTripTest_RunError_RoundTrips(t *testing.T) {
	result := roundTrip(t, &agui.RunErrorEvent{Message: "boom", Code: agui.Ptr("E42")})
	eq(t, "Message", result.Message, "boom")
	eq(t, "Code", deref(t, "Code", result.Code), "E42")
}

// C#: tests/AGUI.Protobuf.UnitTests/EventRoundTripTest.cs::EventRoundTripTest.RunError_NoCode_RoundTrips
func TestCsEventRoundTripTest_RunError_NoCode_RoundTrips(t *testing.T) {
	result := roundTrip(t, &agui.RunErrorEvent{Message: "boom"})
	eq(t, "Message", result.Message, "boom")
	isNil(t, "Code", result.Code)
}

// C#: tests/AGUI.Protobuf.UnitTests/EventRoundTripTest.cs::EventRoundTripTest.RunFinished_WithUsage_RoundTrips
func TestCsEventRoundTripTest_RunFinished_WithUsage_RoundTrips(t *testing.T) {
	result := roundTrip(t, &agui.RunFinishedEvent{
		ThreadID: "thread-1", RunID: "run-1",
		Outcome: &agui.SuccessOutcome{},
		Usage: []agui.TokenUsage{
			{
				Provider: agui.Ptr("openai"), Model: agui.Ptr("gpt-4o"),
				InputTokens: agui.Ptr[int64](11), OutputTokens: agui.Ptr[int64](22), TotalTokens: agui.Ptr[int64](33),
				ReasoningTokens: agui.Ptr[int64](44), CachedInputTokens: agui.Ptr[int64](55), CacheWriteInputTokens: agui.Ptr[int64](66),
			},
			{Provider: agui.Ptr("anthropic"), Model: agui.Ptr("claude-opus-4"), InputTokens: agui.Ptr[int64](1)},
		},
	})
	eq(t, "len(Usage)", len(result.Usage), 2)
	first := result.Usage[0]
	eq(t, "Provider", deref(t, "Provider", first.Provider), "openai")
	eq(t, "Model", deref(t, "Model", first.Model), "gpt-4o")
	eq(t, "InputTokens", deref(t, "InputTokens", first.InputTokens), 11)
	eq(t, "OutputTokens", deref(t, "OutputTokens", first.OutputTokens), 22)
	eq(t, "TotalTokens", deref(t, "TotalTokens", first.TotalTokens), 33)
	eq(t, "ReasoningTokens", deref(t, "ReasoningTokens", first.ReasoningTokens), 44)
	eq(t, "CachedInputTokens", deref(t, "CachedInputTokens", first.CachedInputTokens), 55)
	eq(t, "CacheWriteInputTokens", deref(t, "CacheWriteInputTokens", first.CacheWriteInputTokens), 66)
	second := result.Usage[1]
	eq(t, "Provider", deref(t, "Provider", second.Provider), "anthropic")
	eq(t, "InputTokens", deref(t, "InputTokens", second.InputTokens), 1)
	// Counts the producer never reported must stay absent, not collapse to 0.
	isNil(t, "OutputTokens", second.OutputTokens)
	isNil(t, "TotalTokens", second.TotalTokens)
}

// C#: tests/AGUI.Protobuf.UnitTests/EventRoundTripTest.cs::EventRoundTripTest.RunError_WithUsage_RoundTrips
func TestCsEventRoundTripTest_RunError_WithUsage_RoundTrips(t *testing.T) {
	result := roundTrip(t, &agui.RunErrorEvent{Message: "boom", Code: agui.Ptr("E42"),
		Usage: []agui.TokenUsage{{Provider: agui.Ptr("openai"), InputTokens: agui.Ptr[int64](120)}}})
	eq(t, "Message", result.Message, "boom")
	eq(t, "len(Usage)", len(result.Usage), 1)
	eq(t, "Provider", deref(t, "Provider", result.Usage[0].Provider), "openai")
	eq(t, "InputTokens", deref(t, "InputTokens", result.Usage[0].InputTokens), 120)
	isNil(t, "Model", result.Usage[0].Model)
}

// C#: tests/AGUI.Protobuf.UnitTests/EventRoundTripTest.cs::EventRoundTripTest.RunFinished_UsageZeroCounts_SurviveAsZeroNotNull
func TestCsEventRoundTripTest_RunFinished_UsageZeroCounts_SurviveAsZeroNotNull(t *testing.T) {
	// The proto fields are `optional`, so a set-to-zero count must round-trip
	// as 0 rather than collapsing to "not reported".
	zero := func() *int64 { return agui.Ptr[int64](0) }
	result := roundTrip(t, &agui.RunFinishedEvent{ThreadID: "thread-1", RunID: "run-1",
		Usage: []agui.TokenUsage{{InputTokens: zero(), CachedInputTokens: zero(), CacheWriteInputTokens: zero(), ReasoningTokens: zero()}}})
	eq(t, "len(Usage)", len(result.Usage), 1)
	entry := result.Usage[0]
	eq(t, "InputTokens", deref(t, "InputTokens", entry.InputTokens), 0)
	eq(t, "CachedInputTokens", deref(t, "CachedInputTokens", entry.CachedInputTokens), 0)
	eq(t, "CacheWriteInputTokens", deref(t, "CacheWriteInputTokens", entry.CacheWriteInputTokens), 0)
	eq(t, "ReasoningTokens", deref(t, "ReasoningTokens", entry.ReasoningTokens), 0)
	isNil(t, "OutputTokens", entry.OutputTokens)
	isNil(t, "TotalTokens", entry.TotalTokens)
}

// C#: tests/AGUI.Protobuf.UnitTests/EventRoundTripTest.cs::EventRoundTripTest.RunFinished_EmptyUsage_EncodesIdenticallyToAbsentUsage
func TestCsEventRoundTripTest_RunFinished_EmptyUsage_EncodesIdenticallyToAbsentUsage(t *testing.T) {
	absent := encode(t, &agui.RunFinishedEvent{ThreadID: "thread-1", RunID: "run-1", Usage: nil})
	// A Go empty slice is omitted by the typed model (omitzero), so the
	// explicit empty array is carried by the JSON document instead.
	empty := encodeDoc(t, `{"type":"RUN_FINISHED","threadId":"thread-1","runId":"run-1","usage":[]}`)
	if !bytes.Equal(absent, empty) {
		t.Fatalf("absent %x != empty %x", absent, empty)
	}
	if !bytes.Equal(absent, encode(t, &agui.RunFinishedEvent{ThreadID: "thread-1", RunID: "run-1", Usage: []agui.TokenUsage{}})) {
		t.Fatal("typed empty usage encodes differently from absent usage")
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/EventRoundTripTest.cs::EventRoundTripTest.RunFinished_NoUsage_DecodesToNullNotEmptyList
func TestCsEventRoundTripTest_RunFinished_NoUsage_DecodesToNullNotEmptyList(t *testing.T) {
	result := roundTrip(t, &agui.RunFinishedEvent{ThreadID: "thread-1", RunID: "run-1"})
	if result.Usage != nil {
		t.Fatalf("Usage = %v, want nil", result.Usage)
	}
	if _, ok := roundTripDoc(t, `{"type":"RUN_FINISHED","threadId":"thread-1","runId":"run-1"}`)["usage"]; ok {
		t.Fatal("usage present in the decoded document")
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/EventRoundTripTest.cs::EventRoundTripTest.RunError_NoUsage_DecodesToNullNotEmptyList
func TestCsEventRoundTripTest_RunError_NoUsage_DecodesToNullNotEmptyList(t *testing.T) {
	result := roundTrip(t, &agui.RunErrorEvent{Message: "boom"})
	if result.Usage != nil {
		t.Fatalf("Usage = %v, want nil", result.Usage)
	}
	if _, ok := roundTripDoc(t, `{"type":"RUN_ERROR","message":"boom"}`)["usage"]; ok {
		t.Fatal("usage present in the decoded document")
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/EventRoundTripTest.cs::EventRoundTripTest.StepStarted_RoundTrips
func TestCsEventRoundTripTest_StepStarted_RoundTrips(t *testing.T) {
	eq(t, "StepName", roundTrip(t, &agui.StepStartedEvent{StepName: "step-1"}).StepName, "step-1")
}

// C#: tests/AGUI.Protobuf.UnitTests/EventRoundTripTest.cs::EventRoundTripTest.StepFinished_RoundTrips
func TestCsEventRoundTripTest_StepFinished_RoundTrips(t *testing.T) {
	eq(t, "StepName", roundTrip(t, &agui.StepFinishedEvent{StepName: "step-1"}).StepName, "step-1")
}

// C#: tests/AGUI.Protobuf.UnitTests/EventRoundTripTest.cs::EventRoundTripTest.TextMessageStart_RoundTrips
func TestCsEventRoundTripTest_TextMessageStart_RoundTrips(t *testing.T) {
	result := roundTrip(t, &agui.TextMessageStartEvent{MessageID: "msg-1",
		Role: agui.Ptr(agui.TextMessageRoleAssistant), Name: agui.Ptr("bot")})
	eq(t, "MessageID", result.MessageID, "msg-1")
	eq(t, "Role", deref(t, "Role", result.Role), agui.TextMessageRoleAssistant)
	eq(t, "Name", deref(t, "Name", result.Name), "bot")
}

// C#: tests/AGUI.Protobuf.UnitTests/EventRoundTripTest.cs::EventRoundTripTest.TextMessageContent_RoundTrips
func TestCsEventRoundTripTest_TextMessageContent_RoundTrips(t *testing.T) {
	result := roundTrip(t, &agui.TextMessageContentEvent{MessageID: "msg-1", Delta: "hello"})
	eq(t, "MessageID", result.MessageID, "msg-1")
	eq(t, "Delta", result.Delta, "hello")
}

// C#: tests/AGUI.Protobuf.UnitTests/EventRoundTripTest.cs::EventRoundTripTest.TextMessageEnd_RoundTrips
func TestCsEventRoundTripTest_TextMessageEnd_RoundTrips(t *testing.T) {
	eq(t, "MessageID", roundTrip(t, &agui.TextMessageEndEvent{MessageID: "msg-1"}).MessageID, "msg-1")
}

// C#: tests/AGUI.Protobuf.UnitTests/EventRoundTripTest.cs::EventRoundTripTest.ToolCallStart_RoundTrips
func TestCsEventRoundTripTest_ToolCallStart_RoundTrips(t *testing.T) {
	result := roundTrip(t, &agui.ToolCallStartEvent{ToolCallID: "tc-1", ToolCallName: "search", ParentMessageID: agui.Ptr("msg-1")})
	eq(t, "ToolCallID", result.ToolCallID, "tc-1")
	eq(t, "ToolCallName", result.ToolCallName, "search")
	eq(t, "ParentMessageID", deref(t, "ParentMessageID", result.ParentMessageID), "msg-1")
}

// C#: tests/AGUI.Protobuf.UnitTests/EventRoundTripTest.cs::EventRoundTripTest.ToolCallArgs_RoundTrips
func TestCsEventRoundTripTest_ToolCallArgs_RoundTrips(t *testing.T) {
	result := roundTrip(t, &agui.ToolCallArgsEvent{ToolCallID: "tc-1", Delta: `{"q":`})
	eq(t, "ToolCallID", result.ToolCallID, "tc-1")
	eq(t, "Delta", result.Delta, `{"q":`)
}

// C#: tests/AGUI.Protobuf.UnitTests/EventRoundTripTest.cs::EventRoundTripTest.ToolCallEnd_RoundTrips
func TestCsEventRoundTripTest_ToolCallEnd_RoundTrips(t *testing.T) {
	eq(t, "ToolCallID", roundTrip(t, &agui.ToolCallEndEvent{ToolCallID: "tc-1"}).ToolCallID, "tc-1")
}

// C#: tests/AGUI.Protobuf.UnitTests/EventRoundTripTest.cs::EventRoundTripTest.StateSnapshot_RoundTrips
func TestCsEventRoundTripTest_StateSnapshot_RoundTrips(t *testing.T) {
	snapshot := parseJSON(t, `{"counter":5,"items":["a","b"]}`)
	assertJSONEqual(t, snapshot, roundTrip(t, &agui.StateSnapshotEvent{Snapshot: snapshot}).Snapshot)
}

// C#: tests/AGUI.Protobuf.UnitTests/EventRoundTripTest.cs::EventRoundTripTest.StateDelta_RoundTrips
func TestCsEventRoundTripTest_StateDelta_RoundTrips(t *testing.T) {
	const delta = `[{"op":"add","path":"/a","value":1},{"op":"remove","path":"/b"},{"op":"move","from":"/c","path":"/d"}]`
	result := roundTrip(t, &agui.StateDeltaEvent{Delta: agui.JSONPatch{agui.Add("/a", 1), agui.Remove("/b"), agui.Move("/c", "/d")}})
	assertJSONEqual(t, parseJSON(t, delta), result.Delta)
}

// C#: tests/AGUI.Protobuf.UnitTests/EventRoundTripTest.cs::EventRoundTripTest.MessagesSnapshot_RoundTrips
func TestCsEventRoundTripTest_MessagesSnapshot_RoundTrips(t *testing.T) {
	result := roundTrip(t, &agui.MessagesSnapshotEvent{Messages: agui.Messages{
		&agui.SystemMessage{ID: "s1", Content: "be helpful", Name: agui.Ptr("sys")},
		&agui.UserMessage{ID: "u1", Content: agui.TextContent("hi there"), Name: agui.Ptr("alice")},
		&agui.AssistantMessage{ID: "a1", Content: agui.Ptr("calling tool"),
			ToolCalls: []agui.ToolCall{agui.NewToolCall("tc-1", "search", `{"q":"x"}`)}},
		&agui.ToolMessage{ID: "t1", Content: agui.TextContent("result"), ToolCallID: "tc-1"},
		&agui.DeveloperMessage{ID: "d1", Content: "debug"},
	}})
	eq(t, "len(Messages)", len(result.Messages), 5)

	system := asType[*agui.SystemMessage](t, result.Messages[0])
	eq(t, "system.Content", system.Content, "be helpful")
	eq(t, "system.Name", deref(t, "Name", system.Name), "sys")

	user := asType[*agui.UserMessage](t, result.Messages[1])
	eq(t, "user.Name", deref(t, "Name", user.Name), "alice")
	if user.Content.IsParts() {
		t.Fatal("user content decoded as parts, want a string")
	}
	eq(t, "user.Content", user.Content.String(), "hi there")

	assistant := asType[*agui.AssistantMessage](t, result.Messages[2])
	eq(t, "assistant.Content", deref(t, "Content", assistant.Content), "calling tool")
	eq(t, "len(ToolCalls)", len(assistant.ToolCalls), 1)
	eq(t, "Function.Name", assistant.ToolCalls[0].Function.Name, "search")
	eq(t, "Function.Arguments", assistant.ToolCalls[0].Function.Arguments, `{"q":"x"}`)

	tool := asType[*agui.ToolMessage](t, result.Messages[3])
	eq(t, "tool.Content", tool.Content.String(), "result")
	eq(t, "tool.ToolCallID", tool.ToolCallID, "tc-1")

	developer := asType[*agui.DeveloperMessage](t, result.Messages[4])
	eq(t, "developer.Content", developer.Content, "debug")
}

// C#: tests/AGUI.Protobuf.UnitTests/EventRoundTripTest.cs::EventRoundTripTest.MessagesSnapshot_UserMultimodalContent_RoundTrips
func TestCsEventRoundTripTest_MessagesSnapshot_UserMultimodalContent_RoundTrips(t *testing.T) {
	image := agui.NewImagePart(&agui.URLSource{Value: "https://example.com/a.png", MimeType: agui.Ptr("image/png")})
	image.Metadata = parseJSON(t, `{"alt":"pic"}`)
	result := roundTrip(t, &agui.MessagesSnapshotEvent{Messages: agui.Messages{
		&agui.UserMessage{ID: "u1", Content: agui.PartsContent(
			agui.NewTextPart("look at this"),
			image,
			agui.NewAudioPart(&agui.DataSource{Value: "base64data", MimeType: "audio/mpeg"}),
			agui.NewDocumentPart(&agui.FileSource{Value: "file-abc123", Provider: agui.Ptr("openai"), MimeType: agui.Ptr("application/pdf")}),
			agui.NewDocumentPart(&agui.FileSource{Value: "file-bare"}),
		)},
	}})
	eq(t, "len(Messages)", len(result.Messages), 1)
	user := asType[*agui.UserMessage](t, result.Messages[0])
	if !user.Content.IsParts() {
		t.Fatal("content decoded as a string, want parts")
	}
	parts := user.Content.Parts()
	eq(t, "len(parts)", len(parts), 5)

	eq(t, "text", asType[*agui.TextPart](t, parts[0]).Text, "look at this")

	img := asType[*agui.ImagePart](t, parts[1])
	imgSrc := asType[*agui.URLSource](t, img.Source)
	eq(t, "image.Value", imgSrc.Value, "https://example.com/a.png")
	eq(t, "image.MimeType", deref(t, "MimeType", imgSrc.MimeType), "image/png")
	assertJSONEqual(t, parseJSON(t, `{"alt":"pic"}`), img.Metadata)

	audio := asType[*agui.AudioPart](t, parts[2])
	audioSrc := asType[*agui.DataSource](t, audio.Source)
	eq(t, "audio.Value", audioSrc.Value, "base64data")
	eq(t, "audio.MimeType", audioSrc.MimeType, "audio/mpeg")

	doc := asType[*agui.DocumentPart](t, parts[3])
	fileSrc := asType[*agui.FileSource](t, doc.Source)
	eq(t, "file.Value", fileSrc.Value, "file-abc123")
	eq(t, "file.Provider", deref(t, "Provider", fileSrc.Provider), "openai")
	eq(t, "file.MimeType", deref(t, "MimeType", fileSrc.MimeType), "application/pdf")

	bare := asType[*agui.DocumentPart](t, parts[4])
	bareSrc := asType[*agui.FileSource](t, bare.Source)
	eq(t, "bare.Value", bareSrc.Value, "file-bare")
	isNil(t, "bare.Provider", bareSrc.Provider)
	isNil(t, "bare.MimeType", bareSrc.MimeType)
}

// C#: tests/AGUI.Protobuf.UnitTests/EventRoundTripTest.cs::EventRoundTripTest.ToolCallResult_Parts_RoundTrips
func TestCsEventRoundTripTest_ToolCallResult_Parts_RoundTrips(t *testing.T) {
	text := agui.NewTextPart("Invoice attached.")
	text.ID = agui.Ptr("p1")
	text.Metadata = parseJSON(t, `{"title":"INV"}`)
	document := agui.NewDocumentPart(&agui.URLSource{Value: "https://example.com/i.pdf", MimeType: agui.Ptr("application/pdf")})
	document.ID = agui.Ptr("doc")
	result := roundTrip(t, &agui.ToolCallResultEvent{MessageID: "m2", ToolCallID: "c1", Content: agui.PartsContent(text, document)})

	if !result.Content.IsParts() {
		t.Fatal("content decoded as a string, want parts")
	}
	parts := result.Content.Parts()
	gotText := asType[*agui.TextPart](t, parts[0])
	eq(t, "text.ID", deref(t, "ID", gotText.ID), "p1")
	eq(t, "text.Text", gotText.Text, "Invoice attached.")
	assertJSONEqual(t, parseJSON(t, `{"title":"INV"}`), gotText.Metadata)
	gotDoc := asType[*agui.DocumentPart](t, parts[1])
	eq(t, "document.ID", deref(t, "ID", gotDoc.ID), "doc")
	eq(t, "document.Value", asType[*agui.URLSource](t, gotDoc.Source).Value, "https://example.com/i.pdf")
}

// C#: tests/AGUI.Protobuf.UnitTests/EventRoundTripTest.cs::EventRoundTripTest.ToolCallResult_StringContent_RoundTrips
func TestCsEventRoundTripTest_ToolCallResult_StringContent_RoundTrips(t *testing.T) {
	result := roundTrip(t, &agui.ToolCallResultEvent{MessageID: "m2", ToolCallID: "c1", Content: agui.TextContent("3 results")})
	if result.Content.IsParts() {
		t.Fatal("content decoded as parts, want a string")
	}
	eq(t, "Content", result.Content.String(), "3 results")
}

// C#: tests/AGUI.Protobuf.UnitTests/EventRoundTripTest.cs::EventRoundTripTest.MessagesSnapshot_ToolPartsContent_RoundTrips
func TestCsEventRoundTripTest_MessagesSnapshot_ToolPartsContent_RoundTrips(t *testing.T) {
	result := roundTrip(t, &agui.MessagesSnapshotEvent{Messages: agui.Messages{
		&agui.ToolMessage{ID: "t1", ToolCallID: "c1", Content: agui.PartsContent(
			agui.NewTextPart("see attached"),
			agui.NewImagePart(&agui.DataSource{Value: "aGk=", MimeType: "image/png"}),
		)},
	}})
	eq(t, "len(Messages)", len(result.Messages), 1)
	tool := asType[*agui.ToolMessage](t, result.Messages[0])
	if !tool.Content.IsParts() {
		t.Fatal("content decoded as a string, want parts")
	}
	parts := tool.Content.Parts()
	eq(t, "text", asType[*agui.TextPart](t, parts[0]).Text, "see attached")
	eq(t, "image.Value", asType[*agui.DataSource](t, asType[*agui.ImagePart](t, parts[1]).Source).Value, "aGk=")
}

// C#: tests/AGUI.Protobuf.UnitTests/EventRoundTripTest.cs::EventRoundTripTest.Raw_RoundTrips
func TestCsEventRoundTripTest_Raw_RoundTrips(t *testing.T) {
	payload := parseJSON(t, `{"foo":"bar"}`)
	result := roundTrip(t, &agui.RawEvent{Event: payload, Source: agui.Ptr("external")})
	assertJSONEqual(t, payload, result.Event)
	eq(t, "Source", deref(t, "Source", result.Source), "external")
}

// C#: tests/AGUI.Protobuf.UnitTests/EventRoundTripTest.cs::EventRoundTripTest.Custom_RoundTrips
func TestCsEventRoundTripTest_Custom_RoundTrips(t *testing.T) {
	value := parseJSON(t, `[1,2,3]`)
	result := roundTrip(t, &agui.CustomEvent{Name: "ping", Value: value})
	eq(t, "Name", result.Name, "ping")
	assertJSONEqual(t, value, result.Value)
}

// C#: tests/AGUI.Protobuf.UnitTests/EventRoundTripTest.cs::EventRoundTripTest.Custom_NoValue_RoundTripsAsExplicitNull
func TestCsEventRoundTripTest_Custom_NoValue_RoundTripsAsExplicitNull(t *testing.T) {
	// The schema makes value required, so the wire always carries it: a Go
	// nil value crosses as JSON null and comes back as an explicit null.
	b := encode(t, &agui.CustomEvent{Name: "ping"})
	result := asType[*agui.CustomEvent](t, decode(t, b))
	eq(t, "Name", result.Name, "ping")
	if result.Value != nil {
		t.Fatalf("Value = %v, want null", result.Value)
	}
	doc := decodeDoc(t, b)
	v, ok := doc["value"]
	if !ok {
		t.Fatalf("value absent from %v, want an explicit null", doc)
	}
	if v != nil {
		t.Fatalf("value = %v, want null", v)
	}
}
