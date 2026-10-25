package protobuf_test

// Port of tests/AGUI.Protobuf.UnitTests/WireGuardTest.cs: the decode guards.
// Malformed wire input is answered with an error, never with silently
// different events.
//
// The C# envelopes come from the generated Proto.* types; here they are
// hand-built with protowire (json_test_helpers_test.go) on the same frozen
// field numbers.
//
// Adapted guards. .NET's decoder also VALIDATES the decoded event (closed
// outcome / subtype / status unions, required members, the patch-op enum),
// throwing InvalidDataException at the transport. The binding's spec
// (http-protobuf.mdx, "Semantics") and the TypeScript reference decoder
// (proto.ts decode: "Deliberately unvalidated") put that judgment in the one
// enforcement stage every transport shares, so a stream cannot survive over
// SSE and fail over protobuf: "an unrecognised outcome arriving on this
// binding is stripped by enforcement with a warning, exactly as on SSE, and
// the binding MUST NOT reject it at decode time" — and .NET's closed outcome
// union is a recorded gap (conformance/streams/unknown-outcome-stripped.json,
// expectOverrides.dotnet). Those tests therefore assert the spec rule: the
// decoder surfaces the material exactly as the TypeScript reference rebuilds
// it, and enforcement (agui.Sanitize, the client's processing stage) strips,
// drops or rejects it. The guards both reference runtimes share (structure
// that maps to no event at all) stay decode errors.

import (
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/protobuf"
)

// decodeRaw decodes to the JSON document's bytes, failing on any error.
func decodeRaw(t *testing.T, b []byte) []byte {
	t.Helper()
	doc, err := protobuf.DecodeJSON(b)
	if err != nil {
		t.Fatalf("decode must not reject this frame (judgment belongs to enforcement): %v", err)
	}
	return doc
}

// enforce runs the processing model's enforcement stage on a decoded event.
func enforce(t *testing.T, doc []byte) (*agui.SanitizeResult, error) {
	t.Helper()
	return agui.Sanitize("Event", doc, agui.SanitizeOptions{})
}

func warned(res *agui.SanitizeResult, path string) bool {
	for _, w := range res.Warnings {
		if w.Path == path || strings.HasPrefix(w.Path, path+"/") {
			return true
		}
	}
	return false
}

// strippedWithWarning asserts enforcement keeps the event, removes the
// member at path and names it in a warning.
func strippedWithWarning(t *testing.T, doc []byte, path string) map[string]any {
	t.Helper()
	res, err := enforce(t, doc)
	if err != nil {
		t.Fatalf("enforcement rejected %s: %v", doc, err)
	}
	if res.Dropped {
		t.Fatalf("enforcement dropped %s, want %s stripped", doc, path)
	}
	if !warned(res, path) {
		t.Fatalf("no warning names %s: %v", path, res.Warnings)
	}
	return asType[map[string]any](t, generic(t, res.Value))
}

// rejectedByEnforcement asserts enforcement fails the event as malformed,
// naming what is wrong.
func rejectedByEnforcement(t *testing.T, doc []byte, contains string) {
	t.Helper()
	res, err := enforce(t, doc)
	if err == nil {
		t.Fatalf("enforcement accepted %s (%+v)", doc, res)
	}
	if !strings.Contains(err.Error(), contains) {
		t.Fatalf("enforcement error %q does not mention %q", err, contains)
	}
}

func runFinishedWire(outcome string, extra ...[]byte) []byte {
	fields := []byte{}
	fields = append(fields, sf(2, "t1")...)
	fields = append(fields, sf(3, "r1")...)
	if outcome != "" {
		fields = append(fields, sf(5, outcome)...)
	}
	return wrap(armRunFinished, etRunFinished, fields, cat(extra...))
}

func interruptWire(id, reason string) []byte { return ld(6, sf(1, id), sf(2, reason)) }

func messagesSnapshotWire(messages ...[]byte) []byte {
	var body []byte
	for _, m := range messages {
		body = append(body, ld(2, m)...)
	}
	return wrap(armMessagesSnapshot, etMessagesSnapshot, body)
}

// C#: tests/AGUI.Protobuf.UnitTests/WireGuardTest.cs::WireGuardTest.EmptyEnvelope_Throws
func TestCsWireGuardTest_EmptyEnvelope_Throws(t *testing.T) {
	mustBeMalformed(t, []byte{}, "")
}

// C#: tests/AGUI.Protobuf.UnitTests/WireGuardTest.cs::WireGuardTest.RepeatedEnvelopeTag_Merges
func TestCsWireGuardTest_RepeatedEnvelopeTag_Merges(t *testing.T) {
	first := encode(t, &agui.StepFinishedEvent{StepName: "plan"})
	second := encode(t, &agui.StepFinishedEvent{})
	decoded := asType[*agui.StepFinishedEvent](t, decode(t, cat(first, second)))
	eq(t, "StepName", decoded.StepName, "plan")
}

// C#: tests/AGUI.Protobuf.UnitTests/WireGuardTest.cs::WireGuardTest.TwoDifferentEvents_Throw
func TestCsWireGuardTest_TwoDifferentEvents_Throw(t *testing.T) {
	first := encode(t, &agui.StepFinishedEvent{StepName: "plan"})
	second := encode(t, &agui.TextMessageStartEvent{MessageID: "m1"})
	mustBeMalformed(t, cat(first, second), "")
}

// C#: tests/AGUI.Protobuf.UnitTests/WireGuardTest.cs::WireGuardTest.UnknownEnvelopeFields_AreIgnoredPerProtobufRules
func TestCsWireGuardTest_UnknownEnvelopeFields_AreIgnoredPerProtobufRules(t *testing.T) {
	valid := encode(t, &agui.StepFinishedEvent{StepName: "plan"})
	// Field 90, length-delimited, empty — twice. Unknown, so ignored.
	unknown := []byte{0xd2, 0x05, 0x00, 0xd2, 0x05, 0x00}
	decoded := asType[*agui.StepFinishedEvent](t, decode(t, cat(valid, unknown)))
	eq(t, "StepName", decoded.StepName, "plan")
}

// C#: tests/AGUI.Protobuf.UnitTests/WireGuardTest.cs::WireGuardTest.UnknownGroupFields_AreIgnored_KnownTagInsideDoesNotCount
func TestCsWireGuardTest_UnknownGroupFields_AreIgnored_KnownTagInsideDoesNotCount(t *testing.T) {
	valid := encode(t, &agui.StepFinishedEvent{StepName: "plan"})
	group := []byte{
		0xd3, 0x05, // SGROUP 90
		0x80, 0x01, 0x00, // field 16, varint 0 — inside the group
		0xdb, 0x05, 0xdc, 0x05, // nested SGROUP/EGROUP 91
		0xd4, 0x05, // EGROUP 90
	}
	decoded := asType[*agui.StepFinishedEvent](t, decode(t, cat(valid, group)))
	eq(t, "StepName", decoded.StepName, "plan")
}

// C#: tests/AGUI.Protobuf.UnitTests/WireGuardTest.cs::WireGuardTest.OverlongVarintDuplicate_Merges
func TestCsWireGuardTest_OverlongVarintDuplicate_Merges(t *testing.T) {
	// Field 16 again with the same payload, its tag varint encoded overlong:
	// the reader masks the fifth byte to four bits and still sees field 16.
	// Both reference runtimes accept this (TypeScript: schema-corpus.test.ts
	// "merges a repeated envelope tag encoded with an overlong varint").
	valid := encode(t, &agui.StepFinishedEvent{StepName: "plan"})
	payload := valid[2:]
	overlong := cat([]byte{0x82, 0x81, 0x80, 0x80, 0x10}, payload)
	decoded := asType[*agui.StepFinishedEvent](t, decode(t, cat(valid, overlong)))
	eq(t, "StepName", decoded.StepName, "plan")
}

// C#: tests/AGUI.Protobuf.UnitTests/WireGuardTest.cs::WireGuardTest.FieldZero_Throws
func TestCsWireGuardTest_FieldZero_Throws(t *testing.T) {
	valid := encode(t, &agui.StepFinishedEvent{StepName: "plan"})
	mustBeMalformed(t, cat(valid, []byte{0x00, 0x00}), "")
}

// C#: tests/AGUI.Protobuf.UnitTests/WireGuardTest.cs::WireGuardTest.UnknownOutcome_Throws
func TestCsWireGuardTest_UnknownOutcome_Throws(t *testing.T) {
	// Adapted (see file comment): .NET throws; the spec forbids rejecting an
	// unrecognised outcome at decode time — enforcement strips it with a
	// warning (conformance unknown-outcome-stripped).
	doc := decodeRaw(t, runFinishedWire("expired"))
	assertJSONEqual(t, parseJSON(t, `{"type":"RUN_FINISHED","threadId":"t1","runId":"r1","outcome":{"type":"expired"}}`), parseJSON(t, string(doc)))
	ev := strippedWithWarning(t, doc, "/outcome")
	if _, ok := ev["outcome"]; ok {
		t.Fatalf("enforcement kept the unrecognised outcome: %v", ev)
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/WireGuardTest.cs::WireGuardTest.SuccessOutcomeCarryingInterrupts_Throws
func TestCsWireGuardTest_SuccessOutcomeCarryingInterrupts_Throws(t *testing.T) {
	// Adapted: the interrupts are not silently dropped — they ride on the
	// success outcome (as the TypeScript reference rebuilds it), and
	// enforcement strips them from the closed success outcome with a warning.
	doc := decodeRaw(t, runFinishedWire("success", interruptWire("i1", "r")))
	assertJSONEqual(t, parseJSON(t, `{"type":"RUN_FINISHED","threadId":"t1","runId":"r1","outcome":{"type":"success","interrupts":[{"id":"i1","reason":"r"}]}}`), parseJSON(t, string(doc)))
	if err := agui.Validate("Event", doc); err == nil {
		t.Fatal("a success outcome carrying interrupts must not validate")
	}
	strippedWithWarning(t, doc, "/outcome")
}

// C#: tests/AGUI.Protobuf.UnitTests/WireGuardTest.cs::WireGuardTest.AbsentOutcomeCarryingInterrupts_Throws
func TestCsWireGuardTest_AbsentOutcomeCarryingInterrupts_Throws(t *testing.T) {
	// Adapted: with no outcome the payload surfaces as it arrived, for
	// enforcement to judge (TypeScript reference), and enforcement strips it.
	doc := decodeRaw(t, runFinishedWire("", interruptWire("i1", "r")))
	assertJSONEqual(t, parseJSON(t, `{"type":"RUN_FINISHED","threadId":"t1","runId":"r1","interrupts":[{"id":"i1","reason":"r"}]}`), parseJSON(t, string(doc)))
	ev := strippedWithWarning(t, doc, "/interrupts")
	if _, ok := ev["interrupts"]; ok {
		t.Fatalf("enforcement kept the stray interrupts: %v", ev)
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/WireGuardTest.cs::WireGuardTest.InterruptOutcomeCarryingPendingToolCallIds_Throws
func TestCsWireGuardTest_InterruptOutcomeCarryingPendingToolCallIds_Throws(t *testing.T) {
	// Adapted: surfaced on the interrupt outcome, stripped by enforcement.
	doc := decodeRaw(t, runFinishedWire("interrupt", interruptWire("i1", "r"), sf(8, "tc-1")))
	assertJSONEqual(t, parseJSON(t, `{"type":"RUN_FINISHED","threadId":"t1","runId":"r1","outcome":{"type":"interrupt","interrupts":[{"id":"i1","reason":"r"}],"pendingToolCallIds":["tc-1"]}}`), parseJSON(t, string(doc)))
	if err := agui.Validate("Event", doc); err == nil {
		t.Fatal("an interrupt outcome carrying pendingToolCallIds must not validate")
	}
	strippedWithWarning(t, doc, "/outcome")
}

// C#: tests/AGUI.Protobuf.UnitTests/WireGuardTest.cs::WireGuardTest.AbsentOutcomeCarryingPendingToolCallIds_Throws
func TestCsWireGuardTest_AbsentOutcomeCarryingPendingToolCallIds_Throws(t *testing.T) {
	// Adapted: surfaced at the top level as it arrived, stripped by enforcement.
	doc := decodeRaw(t, runFinishedWire("", sf(8, "tc-1")))
	assertJSONEqual(t, parseJSON(t, `{"type":"RUN_FINISHED","threadId":"t1","runId":"r1","pendingToolCallIds":["tc-1"]}`), parseJSON(t, string(doc)))
	ev := strippedWithWarning(t, doc, "/pendingToolCallIds")
	if _, ok := ev["pendingToolCallIds"]; ok {
		t.Fatalf("enforcement kept the stray pendingToolCallIds: %v", ev)
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/WireGuardTest.cs::WireGuardTest.UnknownPatchOperation_Throws
func TestCsWireGuardTest_UnknownPatchOperation_Throws(t *testing.T) {
	// Adapted: an operation this build cannot name rides on as its decimal
	// spelling (TypeScript reference); enforcement drops the operation as an
	// unrecognised union member and names it (state-delta-unknown-op-dropped).
	doc := decodeRaw(t, wrap(armStateDelta, etStateDelta, ld(2, vf(1, 99), sf(2, "/x"))))
	assertJSONEqual(t, parseJSON(t, `{"type":"STATE_DELTA","delta":[{"op":"99","path":"/x"}]}`), parseJSON(t, string(doc)))
	ev := strippedWithWarning(t, doc, "/delta/0")
	eq(t, "len(delta)", len(asType[[]any](t, ev["delta"])), 0)
}

// C#: tests/AGUI.Protobuf.UnitTests/WireGuardTest.cs::WireGuardTest.ActivityContentOnNonActivityRole_Throws
func TestCsWireGuardTest_ActivityContentOnNonActivityRole_Throws(t *testing.T) {
	msg := cat(sf(1, "a1"), sf(2, "assistant"), sf(3, "hi"), ld(13))
	mustBeMalformed(t, messagesSnapshotWire(msg), "activity content")
}

// C#: tests/AGUI.Protobuf.UnitTests/WireGuardTest.cs::WireGuardTest.ContentPartsOnPartlessRole_Throw
func TestCsWireGuardTest_ContentPartsOnPartlessRole_Throw(t *testing.T) {
	msg := cat(sf(1, "a1"), sf(2, "assistant"), sf(3, "hi"), ld(8, ld(1, sf(1, "erased"))))
	mustBeMalformed(t, messagesSnapshotWire(msg), "content parts")
}

// C#: tests/AGUI.Protobuf.UnitTests/WireGuardTest.cs::WireGuardTest.ToolCallResult_StringContentAlongsideParts_Throws
func TestCsWireGuardTest_ToolCallResult_StringContentAlongsideParts_Throws(t *testing.T) {
	b := wrap(armToolCallResult, etToolCallResult, sf(3, "m1"), sf(4, "c1"), sf(5, "hi"), ld(7, ld(1, sf(1, "also"))))
	mustBeMalformed(t, b, "string content and content parts")
}

// C#: tests/AGUI.Protobuf.UnitTests/WireGuardTest.cs::WireGuardTest.StringContentAlongsideParts_Throws
func TestCsWireGuardTest_StringContentAlongsideParts_Throws(t *testing.T) {
	msg := cat(sf(1, "u1"), sf(2, "user"), sf(3, "hi"), ld(8, ld(1, sf(1, "also"))))
	mustBeMalformed(t, messagesSnapshotWire(msg), "string content and content parts")
}

// C#: tests/AGUI.Protobuf.UnitTests/WireGuardTest.cs::WireGuardTest.BaseEventTypeDisagreeingWithEnvelopeEntry_Throws
func TestCsWireGuardTest_BaseEventTypeDisagreeingWithEnvelopeEntry_Throws(t *testing.T) {
	b := wrap(armStepStarted, etStepFinished, sf(2, "plan"))
	mustBeMalformed(t, b, "envelope carries")
}

// C#: tests/AGUI.Protobuf.UnitTests/WireGuardTest.cs::WireGuardTest.MissingBaseEvent_Throws
func TestCsWireGuardTest_MissingBaseEvent_Throws(t *testing.T) {
	mustBeMalformed(t, ld(armStepFinished, sf(2, "plan")), "")
}

// C#: tests/AGUI.Protobuf.UnitTests/WireGuardTest.cs::WireGuardTest.InterruptOutcomeWithNoInterrupts_Throws
func TestCsWireGuardTest_InterruptOutcomeWithNoInterrupts_Throws(t *testing.T) {
	// Adapted: rebuilt as an interrupt outcome with an empty list (TypeScript
	// reference); a known outcome breaking its own rule (minItems 1) is a
	// malformed value, fatal at enforcement.
	doc := decodeRaw(t, runFinishedWire("interrupt"))
	assertJSONEqual(t, parseJSON(t, `{"type":"RUN_FINISHED","threadId":"t1","runId":"r1","outcome":{"type":"interrupt","interrupts":[]}}`), parseJSON(t, string(doc)))
	rejectedByEnforcement(t, doc, "interrupts")
}

// C#: tests/AGUI.Protobuf.UnitTests/WireGuardTest.cs::WireGuardTest.OutOfEnumSubtype_Throws
func TestCsWireGuardTest_OutOfEnumSubtype_Throws(t *testing.T) {
	// Adapted: the subtype rides on; enforcement treats a closed-set value it
	// does not recognise in a required position as unrecognised material and
	// drops the event, naming the value.
	b := wrap(armReasoningEncryptedValue, etReasoningEncryptedValue, sf(3, "bogus"), sf(4, "m1"), sf(5, "v"))
	doc := decodeRaw(t, b)
	assertJSONEqual(t, parseJSON(t, `{"type":"REASONING_ENCRYPTED_VALUE","subtype":"bogus","entityId":"m1","encryptedValue":"v"}`), parseJSON(t, string(doc)))
	res, err := enforce(t, doc)
	if err == nil && !res.Dropped {
		t.Fatalf("enforcement delivered an out-of-enum subtype: %+v", res)
	}
	text := ""
	if err != nil {
		text = err.Error()
	} else {
		for _, w := range res.Warnings {
			text += w.String() + "\n"
		}
	}
	if !strings.Contains(text, "bogus") {
		t.Fatalf("enforcement does not name the value: %q", text)
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/WireGuardTest.cs::WireGuardTest.MissingRequiredJsonField_Throws
func TestCsWireGuardTest_MissingRequiredJsonField_Throws(t *testing.T) {
	// Adapted: a missing required member is a malformed event, fatal at
	// enforcement.
	doc := decodeRaw(t, wrap(armStateSnapshot, etStateSnapshot))
	rejectedByEnforcement(t, doc, "snapshot")
}

// envelopeWithRawContentPart wraps raw InputContent bytes as: user message ->
// messages snapshot -> envelope (Message.content_parts = 8,
// MessagesSnapshotEvent.messages = 2, Event.messages_snapshot = 9).
func envelopeWithRawContentPart(partBytes []byte) []byte {
	return messagesSnapshotWire(cat(sf(1, "u1"), sf(2, "user"), ld(8, partBytes)))
}

// C#: tests/AGUI.Protobuf.UnitTests/WireGuardTest.cs::WireGuardTest.ContentPartCarryingTwoArms_Throws
func TestCsWireGuardTest_ContentPartCarryingTwoArms_Throws(t *testing.T) {
	twoArms := cat(
		ld(1, sf(1, "a")),               // InputContent.text
		ld(2, ld(1, ld(2, sf(1, "u")))), // InputContent.image{source{url{value}}}
	)
	mustBeMalformed(t, envelopeWithRawContentPart(twoArms), "more than one arm")
}

// C#: tests/AGUI.Protobuf.UnitTests/WireGuardTest.cs::WireGuardTest.ContentPartRepeatingTheSameArm_Merges
func TestCsWireGuardTest_ContentPartRepeatingTheSameArm_Merges(t *testing.T) {
	// An omitted scalar preserves the earlier value; a supplied scalar wins.
	for _, row := range []struct{ laterText, expectedText string }{{"", "a"}, {"b", "b"}} {
		t.Run("later="+row.laterText, func(t *testing.T) {
			var later []byte
			if row.laterText != "" {
				later = sf(1, row.laterText)
			}
			repeated := cat(ld(1, sf(1, "a")), ld(1, later))
			snapshot := asType[*agui.MessagesSnapshotEvent](t, decode(t, envelopeWithRawContentPart(repeated)))
			eq(t, "len(Messages)", len(snapshot.Messages), 1)
			user := asType[*agui.UserMessage](t, snapshot.Messages[0])
			parts := user.Content.Parts()
			eq(t, "len(parts)", len(parts), 1)
			eq(t, "Text", asType[*agui.TextPart](t, parts[0]).Text, row.expectedText)
		})
	}
}

func stepFinishedBytes() []byte { return cat(baseEvent(etStepFinished), sf(2, "plan")) }

// C#: tests/AGUI.Protobuf.UnitTests/WireGuardTest.cs::WireGuardTest.HandFramedValidEvent_Decodes
func TestCsWireGuardTest_HandFramedValidEvent_Decodes(t *testing.T) {
	// The positive control for the doctored-bytes tests.
	decoded := asType[*agui.StepFinishedEvent](t, decode(t, ld(armStepFinished, stepFinishedBytes())))
	eq(t, "StepName", decoded.StepName, "plan")
}

// C#: tests/AGUI.Protobuf.UnitTests/WireGuardTest.cs::WireGuardTest.DuplicatedBaseEvent_Merges
func TestCsWireGuardTest_DuplicatedBaseEvent_Merges(t *testing.T) {
	// The standard parser merges base-event fields across occurrences.
	extraBaseEvent := ld(1, vf(1, etStepFinished), vf(2, 9))
	doctored := ld(armStepFinished, stepFinishedBytes(), extraBaseEvent)
	decoded := asType[*agui.StepFinishedEvent](t, decode(t, doctored))
	eq(t, "StepName", decoded.StepName, "plan")
	eq(t, "Timestamp", deref(t, "Timestamp", decoded.Timestamp), 9)
}

// C#: tests/AGUI.Protobuf.UnitTests/WireGuardTest.cs::WireGuardTest.UnknownResumeStatus_Throws
func TestCsWireGuardTest_UnknownResumeStatus_Throws(t *testing.T) {
	// Adapted: the status rides on (TypeScript reference); enforcement drops
	// the resume entry carrying a status it does not recognise, naming it.
	input := cat(sf(1, "t1"), sf(2, "r1"), ld(9, sf(1, "i1"), sf(2, "bogus")))
	doc := decodeRaw(t, wrap(armRunStarted, etRunStarted, sf(2, "t1"), sf(3, "r1"), ld(5, input)))
	in := asType[map[string]any](t, parseJSON(t, string(doc)).(map[string]any)["input"])
	assertJSONEqual(t, parseJSON(t, `[{"interruptId":"i1","status":"bogus"}]`), in["resume"])
	res, err := enforce(t, doc)
	if err != nil {
		t.Fatalf("enforcement rejected the event: %v", err)
	}
	if !warned(res, "/input/resume/0") {
		t.Fatalf("no warning names the resume entry: %v", res.Warnings)
	}
	if !strings.Contains(res.Warnings[0].String(), "bogus") {
		t.Fatalf("the warning does not name the value: %v", res.Warnings)
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/WireGuardTest.cs::WireGuardTest.ToolCallMissingFunction_Throws
func TestCsWireGuardTest_ToolCallMissingFunction_Throws(t *testing.T) {
	// Adapted: a missing required member is malformed, fatal at enforcement.
	msg := cat(sf(1, "a1"), sf(2, "assistant"), ld(5, sf(1, "c1"), sf(2, "function")))
	doc := decodeRaw(t, messagesSnapshotWire(msg))
	rejectedByEnforcement(t, doc, "function")
}

// C#: tests/AGUI.Protobuf.UnitTests/WireGuardTest.cs::WireGuardTest.MissingActivitySnapshotContent_Throws
func TestCsWireGuardTest_MissingActivitySnapshotContent_Throws(t *testing.T) {
	// Adapted: a missing required member is malformed, fatal at enforcement.
	doc := decodeRaw(t, wrap(armActivitySnapshot, etActivitySnapshot, sf(3, "a1"), sf(4, "search")))
	rejectedByEnforcement(t, doc, "content")
}

// C#: tests/AGUI.Protobuf.UnitTests/WireGuardTest.cs::WireGuardTest.SourceCarryingTwoArms_Throws
func TestCsWireGuardTest_SourceCarryingTwoArms_Throws(t *testing.T) {
	// InputContentSource.data = 1, .url = 2; ImageInputPart.source = 1;
	// InputContent.image = 2.
	twoArmSource := cat(
		ld(1, sf(1, "aGk="), sf(2, "a/b")),
		ld(2, sf(1, "u")),
	)
	partBytes := ld(2, ld(1, twoArmSource))
	mustBeMalformed(t, envelopeWithRawContentPart(partBytes), "more than one arm")
}

// C#: tests/AGUI.Protobuf.UnitTests/WireGuardTest.cs::WireGuardTest.EmptyUserPartsArray_IsValidContent
func TestCsWireGuardTest_EmptyUserPartsArray_IsValidContent(t *testing.T) {
	decoded := asType[*agui.MessagesSnapshotEvent](t, decode(t, messagesSnapshotWire(cat(sf(1, "u1"), sf(2, "user")))))
	user := asType[*agui.UserMessage](t, decoded.Messages[0])
	if !user.Content.IsParts() || user.Content.Parts() == nil {
		t.Fatal("an empty parts array is valid content of its own, want []")
	}
	doc := decodeDoc(t, messagesSnapshotWire(cat(sf(1, "u1"), sf(2, "user"))))
	assertJSONEqual(t, parseJSON(t, `[{"id":"u1","role":"user","content":[]}]`), doc["messages"])
}
