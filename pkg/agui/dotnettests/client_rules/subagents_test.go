package client_rules_test

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// ────────────────────────────────────────────────
// Subagent lifecycle
// ────────────────────────────────────────────────

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_WellFormedLifecycle_PassesThrough
func TestCsProtocolRuleTest_Subagent_WellFormedLifecycle_PassesThrough(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		subStarted("s1", "researcher"),
		textStart("m1", "subagentRunId", "s1"),
		textContent("m1", "found it", "subagentRunId", "s1"),
		textEnd("m1", "subagentRunId", "s1"),
		subFinished("s1"),
		runFinished("r1"),
	)
	o.accepted(t)
	started := o.of(agui.EventTypeSubagentStarted)
	if len(started) != 1 {
		t.Fatalf("SUBAGENT_STARTED delivered %d times", len(started))
	}
	eq(t, "name", started[0].(*agui.SubagentStartedEvent).Name, "researcher")
	eq(t, "SUBAGENT_FINISHED", o.count(agui.EventTypeSubagentFinished), 1)
	found := false
	for _, e := range o.of(agui.EventTypeTextMessageContent) {
		found = found || ownerOf(e) == "s1"
	}
	if !found {
		t.Error("no TEXT_MESSAGE_CONTENT carries subagentRunId s1 after the SSE round trip")
	}
	eq(t, "m1 owner", msgOwner(o.message(t, "m1")), "s1")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_DuplicateStarted_Throws
func TestCsProtocolRuleTest_Subagent_DuplicateStarted_Throws(t *testing.T) {
	o := feed(t, runStarted("r1"), subStarted("s1", "researcher"), subStarted("s1", "researcher"))
	o.protocolRejected(t, "s1", "already active")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_FinishedWithoutStarted_Throws
func TestCsProtocolRuleTest_Subagent_FinishedWithoutStarted_Throws(t *testing.T) {
	o := feed(t, runStarted("r1"), subFinished("ghost"))
	o.protocolRejected(t, "ghost", "no active subagent")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_ErrorWithoutStarted_Throws
func TestCsProtocolRuleTest_Subagent_ErrorWithoutStarted_Throws(t *testing.T) {
	o := feed(t, runStarted("r1"), ev("SUBAGENT_ERROR", "subagentRunId", "ghost", "message", "boom"))
	o.protocolRejected(t, "ghost", "no active subagent")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_UnknownParent_Throws
func TestCsProtocolRuleTest_Subagent_UnknownParent_Throws(t *testing.T) {
	o := feed(t, runStarted("r1"), subStarted("child", "inner", "parentSubagentRunId", "never-started"))
	o.protocolRejected(t, "never-started", "has not been started")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_RunFinishedWhileOpen_Throws
func TestCsProtocolRuleTest_Subagent_RunFinishedWhileOpen_Throws(t *testing.T) {
	o := feed(t, runStarted("r1"), subStarted("s1", "researcher"), runFinished("r1"))
	o.protocolRejected(t, "Cannot send 'RUN_FINISHED' while subagents are still active: s1")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_ErrorClosesTheSubagent_SoRunMayFinish
func TestCsProtocolRuleTest_Subagent_ErrorClosesTheSubagent_SoRunMayFinish(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		subStarted("s1", "researcher"),
		ev("SUBAGENT_ERROR", "subagentRunId", "s1", "message", "boom"),
		runFinished("r1"),
	)
	o.accepted(t)
	eq(t, "SUBAGENT_ERROR", o.count(agui.EventTypeSubagentError), 1)
	eq(t, "RUN_FINISHED", o.count(agui.EventTypeRunFinished), 1)
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_NestedAndParallel_LinkByIdentityNotArrivalOrder
func TestCsProtocolRuleTest_Subagent_NestedAndParallel_LinkByIdentityNotArrivalOrder(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		subStarted("a", "alpha"),
		subStarted("b", "beta"),
		subStarted("child", "inner", "parentSubagentRunId", "a"),
		textStart("mb", "subagentRunId", "b"),
		textContent("mb", "from beta", "subagentRunId", "b"),
		textEnd("mb", "subagentRunId", "b"),
		textStart("mc", "subagentRunId", "child"),
		textContent("mc", "from child", "subagentRunId", "child"),
		textEnd("mc", "subagentRunId", "child"),
		subFinished("child"),
		subFinished("b"),
		subFinished("a"),
		runFinished("r1"),
	)
	o.accepted(t)
	var child *agui.SubagentStartedEvent
	for _, e := range o.of(agui.EventTypeSubagentStarted) {
		if s := e.(*agui.SubagentStartedEvent); s.SubagentRunID == "child" {
			child = s
		}
	}
	if child == nil {
		t.Fatal("SUBAGENT_STARTED child not delivered")
	}
	eq(t, "child parent", own(child.ParentSubagentRunID), "a")
	lanes := map[string]string{}
	for _, e := range o.of(agui.EventTypeTextMessageContent) {
		lanes[e.(*agui.TextMessageContentEvent).MessageID] = ownerOf(e)
	}
	eq(t, "mb owner", lanes["mb"], "b")
	eq(t, "mc owner", lanes["mc"], "child")
	eq(t, "RUN_FINISHED", o.count(agui.EventTypeRunFinished), 1)
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_ClosingParentBeforeChild_IsAccepted_ChildStillBlocksRunFinish
func TestCsProtocolRuleTest_Subagent_ClosingParentBeforeChild_IsAccepted_ChildStillBlocksRunFinish(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		subStarted("a", "alpha"),
		subStarted("child", "inner", "parentSubagentRunId", "a"),
		subFinished("a"),
		runFinished("r1"),
	)
	o.protocolRejected(t, "while subagents are still active", "child")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_AttributedStateEvents_AreAccepted
func TestCsProtocolRuleTest_Subagent_AttributedStateEvents_AreAccepted(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		subStarted("s1", "researcher"),
		ev("STATE_SNAPSHOT", "snapshot", raw(`{"a":1}`), "subagentRunId", "s1"),
		ev("STATE_DELTA", "delta", raw(`[]`), "subagentRunId", "s1"),
		subFinished("s1"),
		runFinished("r1"),
	)
	o.accepted(t)
	// subagents.mdx: "a consumer MUST NOT withhold the update from the run's
	// state because a subagent sent it" — Go has a state store, so check it.
	st, _ := o.agent.Store.State.(map[string]any)
	if st == nil || st["a"] == nil {
		t.Errorf("subagent's snapshot not applied to the run state: %v", o.agent.Store.State)
	}
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_UnattributedStateFromParent_IsAccepted
func TestCsProtocolRuleTest_Subagent_UnattributedStateFromParent_IsAccepted(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		subStarted("s1", "researcher"),
		ev("STATE_SNAPSHOT", "snapshot", raw(`{"a":1}`)),
		subFinished("s1"),
		runFinished("r1"),
	)
	o.accepted(t)
	eq(t, "STATE_SNAPSHOT", o.count(agui.EventTypeStateSnapshot), 1)
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_StateIsClearedBetweenRuns
func TestCsProtocolRuleTest_Subagent_StateIsClearedBetweenRuns(t *testing.T) {
	o := feed(t,
		runStarted("r1"), subStarted("s1", "researcher"), subFinished("s1"), runFinished("r1"),
		runStarted("r2"), subStarted("s1", "researcher"), subFinished("s1"), runFinished("r2"),
	)
	o.accepted(t)
	eq(t, "SUBAGENT_STARTED", o.count(agui.EventTypeSubagentStarted), 2)
	eq(t, "RUN_FINISHED", o.count(agui.EventTypeRunFinished), 2)
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_NothingIsAcceptedAfterTheRunTerminal
func TestCsProtocolRuleTest_Subagent_NothingIsAcceptedAfterTheRunTerminal(t *testing.T) {
	o := feed(t, runStarted("r1"), runError("failed"), subStarted("s1", "researcher"))
	o.protocolRejected(t, "already errored")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_ToolCallAttribution_SurvivesTheWire
func TestCsProtocolRuleTest_Subagent_ToolCallAttribution_SurvivesTheWire(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		subStarted("s1", "researcher"),
		toolStart("tc1", "search", "subagentRunId", "s1"),
		toolArgs("tc1", `{"q":"x"}`, "subagentRunId", "s1"),
		toolEnd("tc1", "subagentRunId", "s1"),
		toolResult("m9", "tc1", "done", "subagentRunId", "s1"),
		subFinished("s1"),
		runFinished("r1"),
	)
	o.accepted(t)
	for _, et := range []agui.EventType{agui.EventTypeToolCallStart, agui.EventTypeToolCallArgs, agui.EventTypeToolCallEnd, agui.EventTypeToolCallResult} {
		evs := o.of(et)
		if len(evs) != 1 || ownerOf(evs[0]) != "s1" {
			t.Errorf("%s: attribution lost (%v)", et, evs)
		}
	}
	calls := o.calls()
	if len(calls) != 1 {
		t.Fatalf("tool calls: %d, want 1", len(calls))
	}
	eq(t, "name", calls[0].Function.Name, "search")
	eq(t, "call holder owner", msgOwner(o.holder(t, "tc1")), "s1")
	eq(t, "result owner", own(o.toolMessage(t, "tc1").SubagentRunID), "s1")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_RestartingAFinishedSubagent_Throws
func TestCsProtocolRuleTest_Subagent_RestartingAFinishedSubagent_Throws(t *testing.T) {
	o := feed(t, runStarted("r1"), subStarted("s1", "researcher"), subFinished("s1"), subStarted("s1", "researcher"))
	o.protocolRejected(t, "s1", "already finished in this run")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_EventTaggedWithAFinishedSubagent_IsAccepted
func TestCsProtocolRuleTest_Subagent_EventTaggedWithAFinishedSubagent_IsAccepted(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		subStarted("s1", "researcher"),
		subFinished("s1"),
		textStart("m1", "subagentRunId", "s1"),
		textContent("m1", "trailing"),
		textEnd("m1"),
		runFinished("r1"),
	)
	o.accepted(t)
	eq(t, "RUN_FINISHED", o.count(agui.EventTypeRunFinished), 1)
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_MissingRequiredIdentifier_Throws
//
// The C# event leaves SubagentRunId null, so the property is omitted on the
// wire; the schema requires it, so this is a malformed known value (fatal).
func TestCsProtocolRuleTest_Subagent_MissingRequiredIdentifier_Throws(t *testing.T) {
	o := feed(t, runStarted("r1"), ev("SUBAGENT_STARTED", "name", "worker"))
	o.validationRejected(t, "subagentRunId")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_MissingRequiredName_Throws
func TestCsProtocolRuleTest_Subagent_MissingRequiredName_Throws(t *testing.T) {
	o := feed(t, runStarted("r1"), ev("SUBAGENT_STARTED", "subagentRunId", "s1"))
	o.validationRejected(t, "name")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_MissingErrorMessage_Throws
func TestCsProtocolRuleTest_Subagent_MissingErrorMessage_Throws(t *testing.T) {
	o := feed(t, runStarted("r1"), subStarted("s1", "researcher"), ev("SUBAGENT_ERROR", "subagentRunId", "s1"))
	o.validationRejected(t, "message")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_ChildOfAFinishedParent_IsAccepted
func TestCsProtocolRuleTest_Subagent_ChildOfAFinishedParent_IsAccepted(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		subStarted("p", "parent"),
		subFinished("p"),
		subStarted("c", "child", "parentSubagentRunId", "p"),
		subFinished("c"),
		runFinished("r1"),
	)
	o.accepted(t)
	eq(t, "RUN_FINISHED", o.count(agui.EventTypeRunFinished), 1)
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_EmptyStringId_IsDistinctFromTheParent
func TestCsProtocolRuleTest_Subagent_EmptyStringId_IsDistinctFromTheParent(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		subStarted("", "a"),
		stepStarted("tools"),
		stepStarted("tools", "subagentRunId", ""),
		stepFinished("tools", "subagentRunId", ""),
		stepFinished("tools"),
		subFinished(""),
		runFinished("r1"),
	)
	o.accepted(t)
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_AttributionOnlyStream_IsAccepted
func TestCsProtocolRuleTest_Subagent_AttributionOnlyStream_IsAccepted(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		textStart("m", "subagentRunId", "never-started"),
		textContent("m", "x", "subagentRunId", "never-started"),
		textEnd("m", "subagentRunId", "never-started"),
		runFinished("r1"),
	)
	o.accepted(t)
	eq(t, "m owner", msgOwner(o.message(t, "m")), "never-started")
}

// ────────────────────────────────────────────────
// Steps are per owner
// ────────────────────────────────────────────────

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_ParentAndSubagentStepsOfSameName_AreBothAllowed
func TestCsProtocolRuleTest_Subagent_ParentAndSubagentStepsOfSameName_AreBothAllowed(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		stepStarted("tools"),
		subStarted("s1", "alpha"),
		stepStarted("tools", "subagentRunId", "s1"),
		stepFinished("tools", "subagentRunId", "s1"),
		subFinished("s1"),
		stepFinished("tools"),
		runFinished("r1"),
	)
	o.accepted(t)
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_StepFinishedClosingParentStepUnderSubagentTag_Throws
func TestCsProtocolRuleTest_Subagent_StepFinishedClosingParentStepUnderSubagentTag_Throws(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		stepStarted("tools"),
		subStarted("s1", "alpha"),
		stepFinished("tools", "subagentRunId", "s1"),
	)
	o.protocolRejected(t, "that step is open under the parent agent", "finished by whoever started it")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_StepFinishedClosingSubagentStepUntagged_Throws
func TestCsProtocolRuleTest_Subagent_StepFinishedClosingSubagentStepUntagged_Throws(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		subStarted("s1", "alpha"),
		stepStarted("inner", "subagentRunId", "s1"),
		stepFinished("inner"),
	)
	o.protocolRejected(t, "attributed to the parent agent", "open under subagent 's1'")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_UnfinishedSubagentStepAtRunFinished_NamesTheOwner
func TestCsProtocolRuleTest_Subagent_UnfinishedSubagentStepAtRunFinished_NamesTheOwner(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		subStarted("s1", "alpha"),
		stepStarted("inner", "subagentRunId", "s1"),
		subFinished("s1"),
		runFinished("r1"),
	)
	o.protocolRejected(t, "inner (subagent 's1')")
}

// ────────────────────────────────────────────────
// Per-run owner maps
// ────────────────────────────────────────────────

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_ReasoningOwnersAreClearedBetweenRuns
func TestCsProtocolRuleTest_Subagent_ReasoningOwnersAreClearedBetweenRuns(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		ev("REASONING_START", "messageId", "r1", "subagentRunId", "s1"),
		ev("REASONING_END", "messageId", "r1", "subagentRunId", "s1"),
		runFinished("r1"),
		runStarted("r2"),
		ev("REASONING_START", "messageId", "r1", "subagentRunId", "s2"),
		ev("REASONING_END", "messageId", "r1", "subagentRunId", "s2"),
		runFinished("r2"),
	)
	o.accepted(t)
	eq(t, "RUN_FINISHED", o.count(agui.EventTypeRunFinished), 2)
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_ToolCallOwnersAreClearedBetweenRuns
func TestCsProtocolRuleTest_Subagent_ToolCallOwnersAreClearedBetweenRuns(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		toolStart("tc1", "search", "subagentRunId", "s1"),
		toolEnd("tc1", "subagentRunId", "s1"),
		runFinished("r1"),
		runStarted("r2"),
		toolStart("tc1", "search", "subagentRunId", "s2"),
		toolEnd("tc1", "subagentRunId", "s2"),
		runFinished("r2"),
	)
	o.accepted(t)
	eq(t, "RUN_FINISHED", o.count(agui.EventTypeRunFinished), 2)
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Subagent_ActivityOwnersAreClearedBetweenRuns
func TestCsProtocolRuleTest_Subagent_ActivityOwnersAreClearedBetweenRuns(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		ev("ACTIVITY_SNAPSHOT", "messageId", "a1", "activityType", "search", "content", raw(`{}`), "subagentRunId", "s1"),
		runFinished("r1"),
		runStarted("r2"),
		ev("ACTIVITY_SNAPSHOT", "messageId", "a1", "activityType", "search", "content", raw(`{}`), "replace", false, "subagentRunId", "s2"),
		ev("ACTIVITY_DELTA", "messageId", "a1", "activityType", "search", "patch", raw(`[]`), "subagentRunId", "s2"),
		runFinished("r2"),
	)
	o.accepted(t)
	eq(t, "RUN_FINISHED", o.count(agui.EventTypeRunFinished), 2)
}
