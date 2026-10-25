package client_rules_test

import (
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// ────────────────────────────────────────────────
// Ownership seeded from MESSAGES_SNAPSHOT and RUN_STARTED.input
// ────────────────────────────────────────────────

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Snapshot_ReopenUnderDifferentSubagent_Throws
func TestCsProtocolRuleTest_Snapshot_ReopenUnderDifferentSubagent_Throws(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		ev("MESSAGES_SNAPSHOT", "messages", raw(`[{"id":"m","role":"assistant","content":"old","subagentRunId":"s1"}]`)),
		textStart("m", "subagentRunId", "s2"),
	)
	o.protocolRejected(t, "does not match", "'m'")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Snapshot_ReplayedToolCallUnderDifferentSubagent_Throws
func TestCsProtocolRuleTest_Snapshot_ReplayedToolCallUnderDifferentSubagent_Throws(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		ev("MESSAGES_SNAPSHOT", "messages", raw(`[{"id":"m","role":"assistant","subagentRunId":"s1","toolCalls":[{"id":"tc","type":"function","function":{"name":"search","arguments":"{}"}}]}]`)),
		toolStart("tc", "search", "subagentRunId", "s2"),
	)
	o.protocolRejected(t, "does not match", "'tc'")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Snapshot_ReopenMatchingOwner_Succeeds
func TestCsProtocolRuleTest_Snapshot_ReopenMatchingOwner_Succeeds(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		ev("MESSAGES_SNAPSHOT", "messages", raw(`[{"id":"m","role":"assistant","content":"old","subagentRunId":"s1"}]`)),
		textStart("m", "subagentRunId", "s1"),
		textContent("m", "new", "subagentRunId", "s1"),
		textEnd("m", "subagentRunId", "s1"),
		runFinished("r1"),
	)
	o.accepted(t)
	if _, ok := o.events[len(o.events)-1].(*agui.RunFinishedEvent); !ok {
		t.Errorf("last event %T, want RUN_FINISHED", o.events[len(o.events)-1])
	}
	// Re-streaming a message the consumer already has appends to it
	// (lifecycle.mdx, "Several runs on one stream").
	eq(t, "m content", agui.Deref(o.message(t, "m").(*agui.AssistantMessage).Content), "oldnew")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.RunStartedInputEcho_ReopenUnderDifferentSubagent_Throws
func TestCsProtocolRuleTest_RunStartedInputEcho_ReopenUnderDifferentSubagent_Throws(t *testing.T) {
	o := feed(t,
		runStarted("r1", "input", raw(`{"threadId":"t1","runId":"r1","messages":[{"id":"m","role":"assistant","content":"old","subagentRunId":"s1"}]}`)),
		textStart("m", "subagentRunId", "s2"),
	)
	o.protocolRejected(t, "does not match", "'m'")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Snapshot_ReasoningMessageSeedsTheReasoningOwnerMap_Throws
func TestCsProtocolRuleTest_Snapshot_ReasoningMessageSeedsTheReasoningOwnerMap_Throws(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		ev("MESSAGES_SNAPSHOT", "messages", raw(`[{"id":"r1","role":"reasoning","content":"old","subagentRunId":"s1"}]`)),
		ev("REASONING_MESSAGE_START", "messageId", "r1", "role", "reasoning", "subagentRunId", "s2"),
	)
	o.protocolRejected(t, "does not match", "'r1'")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Snapshot_AuthoritativelyReplacesARecordedOwner
func TestCsProtocolRuleTest_Snapshot_AuthoritativelyReplacesARecordedOwner(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		textStart("m", "subagentRunId", "s1"),
		textEnd("m", "subagentRunId", "s1"),
		ev("MESSAGES_SNAPSHOT", "messages", raw(`[{"id":"m","role":"assistant","content":"snapshot","subagentRunId":"s2"}]`)),
		textStart("m", "subagentRunId", "s1"),
	)
	o.protocolRejected(t, "does not match", "s2")
}

// ────────────────────────────────────────────────
// Tool call vs parent message ownership
// ────────────────────────────────────────────────

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.ToolCall_OwnerConflictsWithParentMessage_Throws
func TestCsProtocolRuleTest_ToolCall_OwnerConflictsWithParentMessage_Throws(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		textStart("m", "subagentRunId", "s1"),
		textEnd("m", "subagentRunId", "s1"),
		toolStart("tc", "search", "parentMessageId", "m", "subagentRunId", "s2"),
	)
	o.protocolRejected(t, "parent message 'm'")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.ToolCall_UntaggedInheritsParentMessageOwner_Succeeds
func TestCsProtocolRuleTest_ToolCall_UntaggedInheritsParentMessageOwner_Succeeds(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		textStart("m", "subagentRunId", "s1"),
		textEnd("m", "subagentRunId", "s1"),
		toolStart("tc", "search", "parentMessageId", "m"),
		toolArgs("tc", "{}", "subagentRunId", "s1"),
		toolEnd("tc"),
		runFinished("r1"),
	)
	o.accepted(t)
	if _, ok := o.events[len(o.events)-1].(*agui.RunFinishedEvent); !ok {
		t.Errorf("last event %T, want RUN_FINISHED", o.events[len(o.events)-1])
	}
	h := o.holder(t, "tc")
	eq(t, "holder", h.ID, "m")
	eq(t, "holder owner", msgOwner(h), "s1")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.ToolCall_ContinuationDisagreesWithInheritedOwner_Throws
func TestCsProtocolRuleTest_ToolCall_ContinuationDisagreesWithInheritedOwner_Throws(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		textStart("m", "subagentRunId", "s1"),
		textEnd("m", "subagentRunId", "s1"),
		toolStart("tc", "search", "parentMessageId", "m"),
		toolArgs("tc", "{}", "subagentRunId", "s2"),
	)
	o.protocolRejected(t, "does not match", "'tc'")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.ToolCall_UntaggedReopenUnderDifferentParent_Throws
func TestCsProtocolRuleTest_ToolCall_UntaggedReopenUnderDifferentParent_Throws(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		textStart("m1", "subagentRunId", "s1"),
		textEnd("m1", "subagentRunId", "s1"),
		toolStart("tc", "search", "parentMessageId", "m1"),
		toolEnd("tc"),
		textStart("m2", "subagentRunId", "s2"),
		textEnd("m2", "subagentRunId", "s2"),
		toolStart("tc", "search", "parentMessageId", "m2"),
	)
	o.protocolRejected(t, "parent message 'm2'")
}

// ────────────────────────────────────────────────
// Compact reasoning chunks: lanes and resolution
// ────────────────────────────────────────────────
//
// .NET models REASONING_MESSAGE_CHUNK as an event of its own and attributes
// the update; Go expands chunks (streaming.mdx: "A consumer MUST expand them
// into the start/content/end form before verification and before application
// code"), so the id-less chunk reaches the application as the
// REASONING_MESSAGE_CONTENT it stands for, attributed to the lane it resolved
// to.

// contentFor returns the delivered REASONING_MESSAGE_CONTENT carrying delta.
func contentFor(t *testing.T, o *outcome, delta string) *agui.ReasoningMessageContentEvent {
	t.Helper()
	for _, e := range o.of(agui.EventTypeReasoningMessageContent) {
		if c := e.(*agui.ReasoningMessageContentEvent); c.Delta == delta {
			return c
		}
	}
	t.Fatalf("no REASONING_MESSAGE_CONTENT with delta %q delivered", delta)
	return nil
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_ConcurrentCompactReasoningStreams_AreTrackedPerLane
func TestCsProtocolRuleTest_Subagent_ConcurrentCompactReasoningStreams_AreTrackedPerLane(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		reasoningChunk("messageId", "ra", "delta", "a", "subagentRunId", "s1"),
		reasoningChunk("messageId", "rb", "delta", "b", "subagentRunId", "s2"),
		reasoningChunk("delta", "c", "subagentRunId", "s1"),
		runFinished("r1"),
	)
	o.accepted(t)
	eq(t, "RUN_FINISHED", o.count(agui.EventTypeRunFinished), 1)
	eq(t, "c continues", contentFor(t, o, "c").MessageID, "ra")
	eq(t, "ra", o.message(t, "ra").(*agui.ReasoningMessage).Content, "ac")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_IdlessCompactChunk_IsCheckedAgainstItsOwnLaneOnly
func TestCsProtocolRuleTest_Subagent_IdlessCompactChunk_IsCheckedAgainstItsOwnLaneOnly(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		reasoningChunk("messageId", "ra", "delta", "a", "subagentRunId", "s1"),
		reasoningChunk("messageId", "rb", "delta", "b", "subagentRunId", "s2"),
		reasoningChunk("messageId", "rc", "delta", "c", "subagentRunId", "s1"),
		reasoningChunk("delta", "d", "subagentRunId", "s2"),
		runFinished("r1"),
	)
	o.accepted(t)
	eq(t, "RUN_FINISHED", o.count(agui.EventTypeRunFinished), 1)
	eq(t, "d continues", contentFor(t, o, "d").MessageID, "rb")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_UntaggedIdlessCompactChunk_IsNeverAMismatch
func TestCsProtocolRuleTest_Subagent_UntaggedIdlessCompactChunk_IsNeverAMismatch(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		reasoningChunk("messageId", "ra", "delta", "a", "subagentRunId", "s1"),
		reasoningChunk("delta", "b"),
		runFinished("r1"),
	)
	o.accepted(t)
	eq(t, "RUN_FINISHED", o.count(agui.EventTypeRunFinished), 1)
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_CompactReasoningCursorsAreClearedBetweenRuns
//
// ADAPTED TO SPEC. C# accepts run 2's id-less chunk because .NET only checks
// owners for compact reasoning and never requires a first chunk's id (the
// corpus records .NET's missing chunk stage as a GAP, e.g.
// first-chunk-missing-id-fatal.json, expectOverrides.dotnet). The cursors are
// indeed cleared by RUN_FINISHED — which is exactly why, in run 2, the chunk
// is a FIRST chunk with no messageId: streaming.mdx, "The first
// REASONING_MESSAGE_CHUNK for a message MUST carry messageId. A consumer MUST
// treat a first chunk missing a required field as a protocol violation rather
// than inventing an identifier." The Go test asserts that rejection — and that
// it is the first-chunk rule, not an owner mismatch against run 1's stream.
func TestCsProtocolRuleTest_Subagent_CompactReasoningCursorsAreClearedBetweenRuns(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		reasoningChunk("messageId", "r1", "delta", "a", "subagentRunId", "s1"),
		runFinished("r1"),
		runStarted("r2"),
		reasoningChunk("delta", "b", "subagentRunId", "s2"),
		runFinished("r2"),
	)
	o.protocolRejected(t, "must have a messageId")
	if o.err != nil && strings.Contains(o.err.Error(), "does not match") {
		t.Errorf("run 2's chunk was checked against run 1's stream: %v", o.err)
	}
	eq(t, "run 1 finished cleanly", o.count(agui.EventTypeRunFinished), 1)
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.ReasoningChunk_IdlessTaglessWithTwoOpenLanes_Throws
func TestCsProtocolRuleTest_ReasoningChunk_IdlessTaglessWithTwoOpenLanes_Throws(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		reasoningChunk("messageId", "ra", "delta", "a", "subagentRunId", "s1"),
		reasoningChunk("messageId", "rb", "delta", "b", "subagentRunId", "s2"),
		reasoningChunk("delta", "c"),
	)
	o.protocolRejected(t, "Ambiguous")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.ReasoningChunk_IdlessTaglessSoleOpenLane_ContinuesAndAttributes
func TestCsProtocolRuleTest_ReasoningChunk_IdlessTaglessSoleOpenLane_ContinuesAndAttributes(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		reasoningChunk("messageId", "ra", "delta", "a", "subagentRunId", "s1"),
		reasoningChunk("delta", "c"),
		runFinished("r1"),
	)
	o.accepted(t)
	c := contentFor(t, o, "c")
	eq(t, "continues", c.MessageID, "ra")
	eq(t, "inferred owner", own(c.SubagentRunID), "s1")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.ReasoningChunk_IdlessTaglessPrefersParentOpenStream
func TestCsProtocolRuleTest_ReasoningChunk_IdlessTaglessPrefersParentOpenStream(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		reasoningChunk("messageId", "rp", "delta", "p"),
		reasoningChunk("messageId", "ra", "delta", "a", "subagentRunId", "s1"),
		reasoningChunk("delta", "c"),
		runFinished("r1"),
	)
	o.accepted(t)
	c := contentFor(t, o, "c")
	eq(t, "continues the parent's stream", c.MessageID, "rp")
	if c.SubagentRunID != nil {
		t.Errorf("an id-less chunk continuing the parent's stream must not be attributed to a subagent: %q", *c.SubagentRunID)
	}
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.ReasoningChunk_SnapshotClosesTheParentLane_SoleLaneWinsAfterIt
func TestCsProtocolRuleTest_ReasoningChunk_SnapshotClosesTheParentLane_SoleLaneWinsAfterIt(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		reasoningChunk("messageId", "parent", "delta", "p"),
		ev("MESSAGES_SNAPSHOT", "messages", raw(`[]`)),
		reasoningChunk("messageId", "sub", "delta", "a", "subagentRunId", "s1"),
		reasoningChunk("delta", "c"),
		runFinished("r1"),
	)
	o.accepted(t)
	c := contentFor(t, o, "c")
	eq(t, "continues", c.MessageID, "sub")
	eq(t, "owner", own(c.SubagentRunID), "s1")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.ReasoningChunk_SubagentTerminalClosesItsLane
func TestCsProtocolRuleTest_ReasoningChunk_SubagentTerminalClosesItsLane(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		subStarted("s1", "a"),
		subStarted("s2", "b"),
		reasoningChunk("messageId", "ra", "delta", "a", "subagentRunId", "s1"),
		subFinished("s1"),
		reasoningChunk("messageId", "rb", "delta", "b", "subagentRunId", "s2"),
		reasoningChunk("delta", "c"),
		subFinished("s2"),
		runFinished("r1"),
	)
	o.accepted(t)
	c := contentFor(t, o, "c")
	eq(t, "continues", c.MessageID, "rb")
	eq(t, "owner", own(c.SubagentRunID), "s2")
}
