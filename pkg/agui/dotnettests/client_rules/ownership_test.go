package client_rules_test

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// Ownership: once an entity is opened under an owner, every continuation must
// agree (subagents.mdx, "Ownership"). Go's error wording differs from .NET's
// but uses the same "does not match" phrase; each rejection is also checked
// to be a *agui.ProtocolError naming the entity.

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_ToolCallArgsWithDifferentOwner_Throws
func TestCsProtocolRuleTest_Subagent_ToolCallArgsWithDifferentOwner_Throws(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		toolStart("tc1", "search", "subagentRunId", "s1"),
		toolArgs("tc1", "{}", "subagentRunId", "s2"),
	)
	o.protocolRejected(t, "does not match", "tc1")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_ToolCallEndWithDifferentOwner_Throws
func TestCsProtocolRuleTest_Subagent_ToolCallEndWithDifferentOwner_Throws(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		toolStart("tc1", "search", "subagentRunId", "s1"),
		toolEnd("tc1", "subagentRunId", "s2"),
	)
	o.protocolRejected(t, "does not match", "tc1")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_TextMessageContentWithDifferentOwner_Throws
func TestCsProtocolRuleTest_Subagent_TextMessageContentWithDifferentOwner_Throws(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		textStart("m1", "subagentRunId", "s1"),
		textContent("m1", "x", "subagentRunId", "s2"),
	)
	o.protocolRejected(t, "does not match", "m1")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_UntaggedContinuationOfATaggedOpener_IsAccepted
func TestCsProtocolRuleTest_Subagent_UntaggedContinuationOfATaggedOpener_IsAccepted(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		toolStart("tc1", "search", "subagentRunId", "s1"),
		toolArgs("tc1", "{}"),
		toolEnd("tc1"),
		runFinished("r1"),
	)
	o.accepted(t)
	eq(t, "RUN_FINISHED", o.count(agui.EventTypeRunFinished), 1)
	eq(t, "call holder owner", msgOwner(o.holder(t, "tc1")), "s1")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_ReasoningContinuationWithDifferentOwner_Throws
func TestCsProtocolRuleTest_Subagent_ReasoningContinuationWithDifferentOwner_Throws(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		ev("REASONING_MESSAGE_START", "messageId", "r1", "role", "reasoning", "subagentRunId", "s1"),
		ev("REASONING_MESSAGE_CONTENT", "messageId", "r1", "delta", "x", "subagentRunId", "s2"),
	)
	o.protocolRejected(t, "does not match", "r1")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_ToolCallEncryptedValueWithDifferentOwner_Throws
func TestCsProtocolRuleTest_Subagent_ToolCallEncryptedValueWithDifferentOwner_Throws(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		toolStart("tc1", "search", "subagentRunId", "s1"),
		encrypted("tool-call", "tc1", "opaque", "subagentRunId", "s2"),
	)
	o.protocolRejected(t, "does not match", "tc1")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_NonReplacingActivitySnapshot_DoesNotTakeOwnership
func TestCsProtocolRuleTest_Subagent_NonReplacingActivitySnapshot_DoesNotTakeOwnership(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		ev("ACTIVITY_SNAPSHOT", "messageId", "a1", "activityType", "search", "content", raw(`{}`), "replace", false, "subagentRunId", "s1"),
		ev("ACTIVITY_SNAPSHOT", "messageId", "a1", "activityType", "search", "content", raw(`{}`), "replace", false, "subagentRunId", "s2"),
		ev("ACTIVITY_DELTA", "messageId", "a1", "activityType", "search", "patch", raw(`[]`), "subagentRunId", "s2"),
	)
	o.protocolRejected(t, "does not match", "a1")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_ActivitySnapshotWithoutReplace_DefaultsToReplacing
func TestCsProtocolRuleTest_Subagent_ActivitySnapshotWithoutReplace_DefaultsToReplacing(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		ev("ACTIVITY_SNAPSHOT", "messageId", "a1", "activityType", "search", "content", raw(`{}`), "subagentRunId", "s1"),
		ev("ACTIVITY_SNAPSHOT", "messageId", "a1", "activityType", "search", "content", raw(`{}`), "subagentRunId", "s2"),
		ev("ACTIVITY_DELTA", "messageId", "a1", "activityType", "search", "patch", raw(`[]`), "subagentRunId", "s2"),
		runFinished("r1"),
	)
	o.accepted(t)
	eq(t, "RUN_FINISHED", o.count(agui.EventTypeRunFinished), 1)
	eq(t, "a1 owner after re-mint", msgOwner(o.message(t, "a1")), "s2")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_ReasoningStartIsAnOpener
func TestCsProtocolRuleTest_Subagent_ReasoningStartIsAnOpener(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		ev("REASONING_START", "messageId", "r1", "subagentRunId", "s1"),
		ev("REASONING_END", "messageId", "r1", "subagentRunId", "s2"),
	)
	o.protocolRejected(t, "does not match", "r1")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_SecondReasoningOpenerThatContradictsTheFirst_Throws
//
// Kept as in C# (TypeScript, .NET and Go all reject). Note that reasoning.mdx
// calls spans and reasoning messages "separate namespaces"; the spec does not
// spell out that a span and a message sharing an id share an owner, so this
// rejection is cross-SDK parity rather than a rule quoted from the prose.
func TestCsProtocolRuleTest_Subagent_SecondReasoningOpenerThatContradictsTheFirst_Throws(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		ev("REASONING_START", "messageId", "r1", "subagentRunId", "s1"),
		ev("REASONING_MESSAGE_START", "messageId", "r1", "role", "reasoning", "subagentRunId", "s2"),
	)
	o.protocolRejected(t, "does not match", "r1")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_SecondReasoningOpenerThatAgrees_IsAccepted
func TestCsProtocolRuleTest_Subagent_SecondReasoningOpenerThatAgrees_IsAccepted(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		ev("REASONING_START", "messageId", "r1", "subagentRunId", "s1"),
		ev("REASONING_MESSAGE_START", "messageId", "r1", "role", "reasoning", "subagentRunId", "s1"),
		ev("REASONING_MESSAGE_END", "messageId", "r1", "subagentRunId", "s1"),
		ev("REASONING_END", "messageId", "r1", "subagentRunId", "s1"),
		runFinished("r1"),
	)
	o.accepted(t)
	if len(o.events) == 0 {
		t.Error("nothing delivered")
	}
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_SecondUntaggedReasoningOpener_IsAccepted
//
// ADAPTED TO SPEC (attribution half). Acceptance is the spec rule
// (subagents.mdx: "A continuation carrying no tag is not a disagreement and
// MUST be accepted") and is asserted as in C#. C# also asserts the reasoning
// content is owned by s1, inherited from the REASONING_START span; that
// follows the TypeScript/.NET choice of one owner per id across spans and
// messages. reasoning.mdx says "Spans and reasoning messages are separate
// namespaces: a REASONING_START opens a span and no message", so the reasoning
// message r1 is opened by the untagged REASONING_MESSAGE_START and — "An
// absent subagentRunId means the parent agent" — belongs to the parent. The
// Go test asserts that, and that the delivered events keep the tags they were
// sent with.
func TestCsProtocolRuleTest_Subagent_SecondUntaggedReasoningOpener_IsAccepted(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		ev("REASONING_START", "messageId", "r1", "subagentRunId", "s1"),
		ev("REASONING_MESSAGE_START", "messageId", "r1", "role", "reasoning"),
		ev("REASONING_MESSAGE_CONTENT", "messageId", "r1", "delta", "thinking"),
		ev("REASONING_MESSAGE_END", "messageId", "r1"),
		ev("REASONING_END", "messageId", "r1"),
		runFinished("r1"),
	)
	o.accepted(t)
	contents := o.of(agui.EventTypeReasoningMessageContent)
	if len(contents) != 1 {
		t.Fatalf("REASONING_MESSAGE_CONTENT delivered %d times", len(contents))
	}
	eq(t, "content tag as sent", ownerOf(contents[0]), "<parent>")
	rm, ok := o.message(t, "r1").(*agui.ReasoningMessage)
	if !ok {
		t.Fatalf("r1 is %T", o.message(t, "r1"))
	}
	eq(t, "reasoning content", rm.Content, "thinking")
	eq(t, "r1 owner (opened by an untagged REASONING_MESSAGE_START)", own(rm.SubagentRunID), "<parent>")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_TextMessageReopenedUnderADifferentOwner_Throws
func TestCsProtocolRuleTest_Subagent_TextMessageReopenedUnderADifferentOwner_Throws(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		textStart("m1", "subagentRunId", "s1"),
		textContent("m1", "from s1", "subagentRunId", "s1"),
		textEnd("m1", "subagentRunId", "s1"),
		textStart("m1", "subagentRunId", "s2"),
	)
	o.protocolRejected(t, "does not match", "m1")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_ToolCallReopenedUnderADifferentOwner_Throws
func TestCsProtocolRuleTest_Subagent_ToolCallReopenedUnderADifferentOwner_Throws(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		toolStart("tc1", "search", "subagentRunId", "s1"),
		toolEnd("tc1", "subagentRunId", "s1"),
		toolStart("tc1", "search", "subagentRunId", "s2"),
	)
	o.protocolRejected(t, "does not match", "tc1")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_ReopeningAnIdUnderTheSameOwner_IsAccepted
func TestCsProtocolRuleTest_Subagent_ReopeningAnIdUnderTheSameOwner_IsAccepted(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		textStart("m1", "subagentRunId", "s1"),
		textEnd("m1", "subagentRunId", "s1"),
		textStart("m1", "subagentRunId", "s1"),
		textContent("m1", "again", "subagentRunId", "s1"),
		textEnd("m1", "subagentRunId", "s1"),
		toolStart("tc1", "search", "subagentRunId", "s1"),
		toolEnd("tc1", "subagentRunId", "s1"),
		toolStart("tc1", "search", "subagentRunId", "s1"),
		toolEnd("tc1", "subagentRunId", "s1"),
		runFinished("r1"),
	)
	o.accepted(t)
	eq(t, "RUN_FINISHED", o.count(agui.EventTypeRunFinished), 1)
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_UntaggedReopen_DoesNotClearTheRecordedOwner
func TestCsProtocolRuleTest_Subagent_UntaggedReopen_DoesNotClearTheRecordedOwner(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		textStart("m1", "subagentRunId", "s1"),
		textEnd("m1", "subagentRunId", "s1"),
		textStart("m1"),
		textContent("m1", "still s1", "subagentRunId", "s1"),
		textEnd("m1"),
		runFinished("r1"),
	)
	o.accepted(t)
	var text agui.Event
	for _, e := range o.of(agui.EventTypeTextMessageContent) {
		if e.(*agui.TextMessageContentEvent).Delta == "still s1" {
			text = e
		}
	}
	if text == nil {
		t.Fatal(`"still s1" not delivered`)
	}
	eq(t, "content owner", ownerOf(text), "s1")
	eq(t, "m1 owner", msgOwner(o.message(t, "m1")), "s1")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_TaggedContinuationOfAnUntaggedOpener_Throws
func TestCsProtocolRuleTest_Subagent_TaggedContinuationOfAnUntaggedOpener_Throws(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		textStart("m1"),
		textContent("m1", "x", "subagentRunId", "s1"),
	)
	o.protocolRejected(t, "does not match", "m1")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_EncryptedToolCallValueAfterTheCallClosed_IsStillChecked
func TestCsProtocolRuleTest_Subagent_EncryptedToolCallValueAfterTheCallClosed_IsStillChecked(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		toolStart("tc1", "search", "subagentRunId", "s1"),
		toolEnd("tc1", "subagentRunId", "s1"),
		encrypted("tool-call", "tc1", "opaque", "subagentRunId", "s2"),
	)
	o.protocolRejected(t, "does not match", "tc1")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_EncryptedMessageValueOnAReasoningMessage_IsStillChecked
func TestCsProtocolRuleTest_Subagent_EncryptedMessageValueOnAReasoningMessage_IsStillChecked(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		ev("REASONING_START", "messageId", "r1", "subagentRunId", "s1"),
		ev("REASONING_MESSAGE_START", "messageId", "r1", "role", "reasoning", "subagentRunId", "s1"),
		ev("REASONING_MESSAGE_END", "messageId", "r1", "subagentRunId", "s1"),
		encrypted("message", "r1", "opaque", "subagentRunId", "s2"),
	)
	o.protocolRejected(t, "does not match", "r1")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_EncryptedMessageValueOnAReasoningMessage_AcceptsTheOwner
func TestCsProtocolRuleTest_Subagent_EncryptedMessageValueOnAReasoningMessage_AcceptsTheOwner(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		ev("REASONING_START", "messageId", "r1", "subagentRunId", "s1"),
		ev("REASONING_MESSAGE_START", "messageId", "r1", "role", "reasoning", "subagentRunId", "s1"),
		ev("REASONING_MESSAGE_END", "messageId", "r1", "subagentRunId", "s1"),
		encrypted("message", "r1", "opaque", "subagentRunId", "s1"),
		ev("REASONING_END", "messageId", "r1", "subagentRunId", "s1"),
		runFinished("r1"),
	)
	o.accepted(t)
	rm := o.message(t, "r1").(*agui.ReasoningMessage)
	eq(t, "encryptedValue stored with r1", own(rm.EncryptedValue), "opaque")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_EncryptedMessageValue_ChecksMessageOwners
func TestCsProtocolRuleTest_Subagent_EncryptedMessageValue_ChecksMessageOwners(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		subStarted("s1", "a"),
		subStarted("s2", "b"),
		textStart("m", "subagentRunId", "s1"),
		encrypted("message", "m", "v", "subagentRunId", "s2"),
	)
	o.protocolRejected(t, "does not match", "'m'")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_EncryptedToolCallValue_StillChecksToolCallOwners
func TestCsProtocolRuleTest_Subagent_EncryptedToolCallValue_StillChecksToolCallOwners(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		subStarted("s1", "a"),
		subStarted("s2", "b"),
		toolStart("c", "t", "subagentRunId", "s1"),
		encrypted("tool-call", "c", "v", "subagentRunId", "s2"),
	)
	o.protocolRejected(t, "does not match", "'c'")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_ToolCallEncryptedValue_ResolvesViaTheCallNamespace
func TestCsProtocolRuleTest_Subagent_ToolCallEncryptedValue_ResolvesViaTheCallNamespace(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		toolStart("tc1", "search", "subagentRunId", "s1"),
		toolEnd("tc1", "subagentRunId", "s1"),
		toolResult("tc1", "tc1", "done", "subagentRunId", "s1"),
		encrypted("tool-call", "tc1", "opaque", "subagentRunId", "s1"),
		runFinished("r1"),
	)
	o.accepted(t)
	evs := o.of(agui.EventTypeReasoningEncryptedValue)
	if len(evs) != 1 {
		t.Fatalf("REASONING_ENCRYPTED_VALUE delivered %d times", len(evs))
	}
	eq(t, "encrypted owner", ownerOf(evs[0]), "s1")
	// subtype "tool-call" routes the value to the call, not to the tool
	// message that happens to share the id.
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
	eq(t, "value on the call", own(tc.EncryptedValue), "opaque")
	eq(t, "value not on the tool message", own(o.toolMessage(t, "tc1").EncryptedValue), "<parent>")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_CompactReasoningChunkOwnerChange_Throws
func TestCsProtocolRuleTest_Subagent_CompactReasoningChunkOwnerChange_Throws(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		reasoningChunk("messageId", "r1", "delta", "a", "subagentRunId", "s1"),
		reasoningChunk("messageId", "r1", "delta", "b", "subagentRunId", "s2"),
	)
	o.protocolRejected(t, "does not match", "r1")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_CompactReasoningChunkWithEmptyId_StillChecksOwner
func TestCsProtocolRuleTest_Subagent_CompactReasoningChunkWithEmptyId_StillChecksOwner(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		reasoningChunk("messageId", "", "delta", "a", "subagentRunId", "s1"),
		reasoningChunk("messageId", "", "delta", "b", "subagentRunId", "s2"),
	)
	o.protocolRejected(t, "does not match")
}
