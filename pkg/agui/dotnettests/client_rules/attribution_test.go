package client_rules_test

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
)

// Attribution that must survive into the conversation and back out to the
// next turn. In .NET this is about stamping "agui.subagentRunId" on
// ChatResponseUpdate.AdditionalProperties so that ToChatResponse() and
// AsAGUIMessages() carry it; in Go the Store assembles AG-UI messages
// directly and Agent.BuildInput sends them, so the assertions read the Store
// and the next run's input.

func turnOwner(m map[string]any) string {
	if s, ok := m["subagentRunId"].(string); ok {
		return s
	}
	return "<parent>"
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_ResponseMessages_CarryAttributionAcrossATurn
func TestCsProtocolRuleTest_Subagent_ResponseMessages_CarryAttributionAcrossATurn(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		subStarted("s1", "researcher"),
		textStart("m1", "subagentRunId", "s1"),
		textContent("m1", "found it", "subagentRunId", "s1"),
		textEnd("m1", "subagentRunId", "s1"),
		toolStart("tc1", "search", "subagentRunId", "s1"),
		toolArgs("tc1", "{}", "subagentRunId", "s1"),
		toolEnd("tc1", "subagentRunId", "s1"),
		subFinished("s1"),
		runFinished("r1"),
	)
	o.accepted(t)
	eq(t, "text message owner", msgOwner(o.message(t, "m1")), "s1")
	eq(t, "tool call holder owner", msgOwner(o.holder(t, "tc1")), "s1")

	n := 0
	for _, m := range o.nextTurn(t) {
		if r := m["role"]; r == "assistant" || r == "tool" {
			n++
			eq(t, "next-turn owner of "+m["id"].(string), turnOwner(m), "s1")
		}
	}
	if n == 0 {
		t.Error("next turn carries no assistant/tool message")
	}
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_AttributionOnlyOutput_SurvivesCoalescingWithoutATextMessage
func TestCsProtocolRuleTest_Subagent_AttributionOnlyOutput_SurvivesCoalescingWithoutATextMessage(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		subStarted("s1", "researcher"),
		toolStart("tc1", "search", "subagentRunId", "s1"),
		toolArgs("tc1", "{}", "subagentRunId", "s1"),
		toolEnd("tc1", "subagentRunId", "s1"),
		toolResult("tr1", "tc1", "42", "subagentRunId", "s1"),
		subFinished("s1"),
		runFinished("r1"),
	)
	o.accepted(t)
	var attributable []map[string]any
	for _, m := range o.nextTurn(t) {
		calls, _ := m["toolCalls"].([]any)
		if (m["role"] == "assistant" && len(calls) > 0) || m["role"] == "tool" {
			attributable = append(attributable, m)
		}
	}
	if len(attributable) == 0 {
		t.Fatal("next turn carries no tool-call or tool message")
	}
	for _, m := range attributable {
		eq(t, "next-turn owner of "+m["id"].(string), turnOwner(m), "s1")
	}
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.EmptyStringParentMessageId_MintsTheToolCallId
//
// ADAPTED TO SPEC. C# pins the TypeScript reducer's truthiness fallback: an
// empty-string parentMessageId is treated as absent and the toolCallId is
// minted as the holding message's id. The spec gives "" no special meaning:
// parentMessageId is a string that "attaches the call to the assistant
// message that carries it" (tool-calls.mdx) and identifiers are opaque
// ("a consumer MUST NOT parse structure out of them", basic/index.mdx). So
// the Go test asserts the stream is accepted and the call is held by the
// assistant message the producer named — the one with id "".
func TestCsProtocolRuleTest_EmptyStringParentMessageId_MintsTheToolCallId(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		toolStart("tc1", "search", "parentMessageId", ""),
		toolArgs("tc1", "{}"),
		toolEnd("tc1"),
		toolResult("tc1", "tc1", "done"),
		runFinished("r1"),
	)
	o.accepted(t)
	eq(t, "tool calls", len(o.calls()), 1)
	eq(t, "holder id = parentMessageId", o.holder(t, "tc1").ID, "")
	eq(t, "result", o.toolMessage(t, "tc1").Content.String(), "done")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_EncryptedOnlyReasoning_SurvivesCoalescing
//
// The C# ReasoningEncryptedValueEvent leaves Subtype unset, which serialises
// as "" — not a member of the closed subtype set, which the spec strips
// together with the event (reasoning.mdx, "Error Handling"). The value names
// the reasoning message r1, so the Go stream sends subtype "message". In .NET
// the value coalesces into an assistant message; Go keeps the reasoning
// message itself (role reasoning), which is what must carry the owner.
func TestCsProtocolRuleTest_Subagent_EncryptedOnlyReasoning_SurvivesCoalescing(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		subStarted("s1", "researcher"),
		ev("REASONING_MESSAGE_START", "messageId", "r1", "role", "reasoning", "subagentRunId", "s1"),
		encrypted("message", "r1", "opaque", "subagentRunId", "s1"),
		ev("REASONING_MESSAGE_END", "messageId", "r1", "subagentRunId", "s1"),
		subFinished("s1"),
		runFinished("r1"),
	)
	o.accepted(t)
	rm, ok := o.message(t, "r1").(*agui.ReasoningMessage)
	if !ok {
		t.Fatalf("r1 is %T", o.message(t, "r1"))
	}
	eq(t, "owner", own(rm.SubagentRunID), "s1")
	eq(t, "encryptedValue", own(rm.EncryptedValue), "opaque")
	found := false
	for _, m := range o.nextTurn(t) {
		if m["id"] == "r1" {
			found = true
			eq(t, "next-turn owner", turnOwner(m), "s1")
			eq(t, "next-turn encryptedValue", m["encryptedValue"], any("opaque"))
		}
	}
	if !found {
		t.Error("r1 not carried into the next turn")
	}
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_InterruptedToolCall_ApprovalKeepsAttributionAcrossTheTurn
//
// ToolApprovalRequestContent is the MEAI rendering of the interrupt; Go
// reports it as RunResult.Interrupts on an interrupted run.
func TestCsProtocolRuleTest_Subagent_InterruptedToolCall_ApprovalKeepsAttributionAcrossTheTurn(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		subStarted("s1", "researcher"),
		toolStart("tc1", "dangerous", "subagentRunId", "s1"),
		toolArgs("tc1", "{}", "subagentRunId", "s1"),
		toolEnd("tc1", "subagentRunId", "s1"),
		subFinished("s1", "outcome", raw(`{"type":"suspended","interruptIds":["int-1"]}`)),
		runFinished("r1", "outcome", raw(`{"type":"interrupt","interrupts":[{"id":"int-1","reason":"tool_call","toolCallId":"tc1","subagentRunId":"s1"}]}`)),
	)
	o.accepted(t)
	eq(t, "status", o.res.Status, client.StatusInterrupted)
	if len(o.res.Interrupts) != 1 {
		t.Fatalf("interrupts: %+v", o.res.Interrupts)
	}
	in := o.res.Interrupts[0]
	eq(t, "interrupt owner", own(in.SubagentRunID), "s1")
	eq(t, "interrupt toolCallId", own(in.ToolCallID), "tc1")
	eq(t, "call holder owner", msgOwner(o.holder(t, "tc1")), "s1")

	n := 0
	for _, m := range o.nextTurn(t, agui.ResumeEntry{InterruptID: "int-1", Status: agui.ResumeStatusResolved, Payload: map[string]any{"approved": true}}) {
		if m["role"] == "assistant" {
			n++
			eq(t, "next-turn owner of "+m["id"].(string), turnOwner(m), "s1")
		}
	}
	if n == 0 {
		t.Error("next turn carries no assistant message")
	}
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_CallIdReuseAcrossRuns_DoesNotInheritTheEarlierRunsMessage
//
// The C# pin is MEAI message identity (the encrypted update's MessageId falls
// back to the call id, never run one's "run1-msg"). Go mints no message for
// the value: reasoning.mdx says a consumer "MUST store the value with the
// message or tool call it names", so the value is stored on tool call tc1
// wherever it lives, and no new message appears.
func TestCsProtocolRuleTest_Subagent_CallIdReuseAcrossRuns_DoesNotInheritTheEarlierRunsMessage(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		toolStart("tc1", "search", "parentMessageId", "run1-msg"),
		toolEnd("tc1"),
		toolResult("tr1", "tc1", "done"),
		runFinished("r1"),
		runStarted("r2"),
		subStarted("s1", "researcher"),
		encrypted("tool-call", "tc1", "opaque", "subagentRunId", "s1"),
		subFinished("s1"),
		runFinished("r2"),
	)
	o.accepted(t)
	evs := o.of(agui.EventTypeReasoningEncryptedValue)
	if len(evs) != 1 {
		t.Fatalf("REASONING_ENCRYPTED_VALUE delivered %d times", len(evs))
	}
	eq(t, "message count", len(o.agent.Store.Messages), 2)
	var tc *agui.ToolCall
	for _, c := range o.calls() {
		if c.ID == "tc1" {
			c := c
			tc = &c
		}
	}
	if tc == nil {
		t.Fatal("tool call tc1 missing")
	}
	eq(t, "value stored on tc1", own(tc.EncryptedValue), "opaque")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Parent_ResponseMessages_StayUnattributed
func TestCsProtocolRuleTest_Parent_ResponseMessages_StayUnattributed(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		textStart("m1"),
		textContent("m1", "hi"),
		textEnd("m1"),
		runFinished("r1"),
	)
	o.accepted(t)
	for _, e := range o.events {
		eq(t, string(e.Type())+" owner", ownerOf(e), "<parent>")
	}
	for _, m := range o.nextTurn(t) {
		eq(t, "next-turn owner", turnOwner(m), "<parent>")
	}
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_OpenerOnlyAttribution_ReachesTheResponse
func TestCsProtocolRuleTest_Subagent_OpenerOnlyAttribution_ReachesTheResponse(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		textStart("m1", "subagentRunId", "s1"),
		textContent("m1", "hi"),
		textEnd("m1"),
		runFinished("r1"),
	)
	o.accepted(t)
	eqStrings(t, "texts", o.texts(), []string{"hi"})
	eq(t, "m1 owner", msgOwner(o.message(t, "m1")), "s1")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_UntaggedToolResult_IsParentOwned
func TestCsProtocolRuleTest_Subagent_UntaggedToolResult_IsParentOwned(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		toolStart("tc1", "search", "subagentRunId", "s1"),
		toolEnd("tc1", "subagentRunId", "s1"),
		toolResult("m9", "tc1", "done"),
		runFinished("r1"),
	)
	o.accepted(t)
	eq(t, "tool message owner", own(o.toolMessage(t, "tc1").SubagentRunID), "<parent>")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_OwnershipDoesNotLeakBetweenRuns
func TestCsProtocolRuleTest_Subagent_OwnershipDoesNotLeakBetweenRuns(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		textStart("m1", "subagentRunId", "s1"),
		textContent("m1", "from s1"),
		textEnd("m1"),
		runFinished("r1"),
		runStarted("r2"),
		textStart("m1"),
		textContent("m1", "from the parent"),
		textEnd("m1"),
		runFinished("r2"),
	)
	o.accepted(t)
	var last agui.Event
	for _, e := range o.of(agui.EventTypeTextMessageContent) {
		if e.(*agui.TextMessageContentEvent).Delta == "from the parent" {
			last = e
		}
	}
	if last == nil {
		t.Fatal(`"from the parent" not delivered`)
	}
	eq(t, "second-run text owner", ownerOf(last), "<parent>")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_ToolResultOwner_DoesNotRestampTheCall
func TestCsProtocolRuleTest_Subagent_ToolResultOwner_DoesNotRestampTheCall(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		toolStart("tc1", "search", "subagentRunId", "s1"),
		toolEnd("tc1", "subagentRunId", "s1"),
		toolResult("m9", "tc1", "done"),
		runFinished("r1"),
	)
	o.accepted(t)
	eq(t, "tool calls", len(o.calls()), 1)
	eq(t, "call holder owner", msgOwner(o.holder(t, "tc1")), "s1")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_EmptyMessageId_StillCarriesAttribution
func TestCsProtocolRuleTest_Subagent_EmptyMessageId_StillCarriesAttribution(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		textStart("", "subagentRunId", "s1"),
		textContent("", "hi"),
		textEnd(""),
		runFinished("r1"),
	)
	o.accepted(t)
	eqStrings(t, "texts", o.texts(), []string{"hi"})
	eq(t, `"" owner`, msgOwner(o.message(t, "")), "s1")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_BufferedUpdates_KeepTheOwnerTheyHadAtCreation
//
// .NET buffers pass-through updates while a tool call is open and stamps
// owners at flush; Go delivers every event in order with its own tag, so the
// assertion is that each ACTIVITY_SNAPSHOT reaches the application with the
// owner it was sent with.
func TestCsProtocolRuleTest_Subagent_BufferedUpdates_KeepTheOwnerTheyHadAtCreation(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		toolStart("tc1", "search"),
		ev("ACTIVITY_SNAPSHOT", "messageId", "a1", "activityType", "search", "content", raw(`{}`), "replace", true, "subagentRunId", "s1"),
		ev("ACTIVITY_SNAPSHOT", "messageId", "a1", "activityType", "search", "content", raw(`{}`), "replace", true, "subagentRunId", "s2"),
		toolEnd("tc1"),
		runFinished("r1"),
	)
	o.accepted(t)
	acts := o.of(agui.EventTypeActivitySnapshot)
	if len(acts) != 2 {
		t.Fatalf("ACTIVITY_SNAPSHOT delivered %d times", len(acts))
	}
	eq(t, "first owner", ownerOf(acts[0]), "s1")
	eq(t, "second owner", ownerOf(acts[1]), "s2")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_BufferedParentOwnedUpdate_StaysParentOwned
func TestCsProtocolRuleTest_Subagent_BufferedParentOwnedUpdate_StaysParentOwned(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		toolStart("tc1", "search"),
		toolEnd("tc1"),
		ev("ACTIVITY_SNAPSHOT", "messageId", "a1", "activityType", "search", "content", raw(`{}`), "replace", true),
		ev("ACTIVITY_SNAPSHOT", "messageId", "a1", "activityType", "search", "content", raw(`{}`), "replace", true, "subagentRunId", "s1"),
		toolResult("m9", "tc1", "done"),
		runFinished("r1"),
	)
	o.accepted(t)
	acts := o.of(agui.EventTypeActivitySnapshot)
	if len(acts) != 2 {
		t.Fatalf("ACTIVITY_SNAPSHOT delivered %d times", len(acts))
	}
	eq(t, "first owner", ownerOf(acts[0]), "<parent>")
	eq(t, "second owner", ownerOf(acts[1]), "s1")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_InternalResolutionMarker_NeverReachesTheCaller
//
// The "agui.__ownerResolved" marker is internal to .NET's update buffering.
// The Go analogue of "no internal key leaks to the caller": every delivered
// event, and every message the next turn sends, validates exactly against the
// 1.0 schema (which forbids unevaluated properties).
func TestCsProtocolRuleTest_Subagent_InternalResolutionMarker_NeverReachesTheCaller(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		toolStart("tc1", "search", "subagentRunId", "s1"),
		toolEnd("tc1", "subagentRunId", "s1"),
		toolResult("m9", "tc1", "done", "subagentRunId", "s1"),
		runFinished("r1"),
	)
	o.accepted(t)
	for _, e := range o.events {
		if err := agui.ValidateEvent(e); err != nil {
			t.Errorf("delivered %s does not validate: %v", e.Type(), err)
		}
	}
	in, _, err := o.agent.BuildInput(client.RunInput{RunID: "next"})
	if err != nil {
		t.Fatal(err)
	}
	if err := agui.Validate("RunAgentInput", mustMarshal(t, in)); err != nil {
		t.Errorf("next-turn input does not validate: %v", err)
	}
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_ResultReusingTheToolCallIdAsMessageId_DoesNotRestampTheCall
func TestCsProtocolRuleTest_Subagent_ResultReusingTheToolCallIdAsMessageId_DoesNotRestampTheCall(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		toolStart("tc1", "search", "subagentRunId", "s1"),
		toolEnd("tc1", "subagentRunId", "s1"),
		toolResult("tc1", "tc1", "done"),
		runFinished("r1"),
	)
	o.accepted(t)
	eq(t, "tool calls", len(o.calls()), 1)
	eq(t, "call holder owner", msgOwner(o.holder(t, "tc1")), "s1")
	eq(t, "result owner", own(o.toolMessage(t, "tc1").SubagentRunID), "<parent>")
}
