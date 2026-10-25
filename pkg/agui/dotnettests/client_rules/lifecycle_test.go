package client_rules_test

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
)

// ────────────────────────────────────────────────
// Lifecycle rules
// ────────────────────────────────────────────────

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.ValidCompleteSequence_ProducesAllEvents
//
// C# collects only the updates that surface (START/END are builder-only in
// .NET); Go delivers every event, so the full sequence is asserted.
func TestCsProtocolRuleTest_ValidCompleteSequence_ProducesAllEvents(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		textStart("m1"),
		textContent("m1", "Hello"),
		textEnd("m1"),
		runFinished("r1"),
	)
	o.accepted(t)
	var types []string
	for _, e := range o.events {
		types = append(types, string(e.Type()))
	}
	eqStrings(t, "delivered", types, []string{"RUN_STARTED", "TEXT_MESSAGE_START", "TEXT_MESSAGE_CONTENT", "TEXT_MESSAGE_END", "RUN_FINISHED"})
	eq(t, "status", o.res.Status, client.StatusSucceeded)
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.RunError_ProducesErrorContentUpdate
func TestCsProtocolRuleTest_RunError_ProducesErrorContentUpdate(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		runError("Something failed", "code", "ERR01"),
	)
	o.accepted(t) // RUN_ERROR is a well-formed report, not a rejection
	eq(t, "status", o.res.Status, client.StatusFailed)
	if o.res.Error == nil {
		t.Fatal("no RUN_ERROR surfaced")
	}
	eq(t, "message", o.res.Error.Message, "Something failed")
	eq(t, "code", own(o.res.Error.Code), "ERR01")
	// ConversationId / ResponseId: the failure belongs to run r1 of thread t1.
	rs, ok := o.res.Events[0].(*agui.RunStartedEvent)
	if !ok {
		t.Fatalf("first event %T", o.res.Events[0])
	}
	eq(t, "threadId", rs.ThreadID, "t1")
	eq(t, "runId", rs.RunID, "r1")
	if _, ok := o.res.Events[1].(*agui.RunErrorEvent); !ok {
		t.Errorf("second event %T, want RUN_ERROR", o.res.Events[1])
	}
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.RunErrorAsFirstEvent_ProducesErrorContentUpdate
func TestCsProtocolRuleTest_RunErrorAsFirstEvent_ProducesErrorContentUpdate(t *testing.T) {
	o := feed(t, runError("Immediate failure"))
	o.accepted(t)
	eq(t, "delivered", len(o.events), 1)
	eq(t, "status", o.res.Status, client.StatusFailed)
	if o.res.Error == nil {
		t.Fatal("no RUN_ERROR surfaced")
	}
	eq(t, "message", o.res.Error.Message, "Immediate failure")
	if o.res.Error.Code != nil {
		t.Errorf("code: got %q, want absent", *o.res.Error.Code)
	}
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.RunFinished_IsLastEvent
func TestCsProtocolRuleTest_RunFinished_IsLastEvent(t *testing.T) {
	o := feed(t, runStarted("r1"), runFinished("r1"))
	o.accepted(t)
	if _, ok := o.events[len(o.events)-1].(*agui.RunFinishedEvent); !ok {
		t.Errorf("last event %T, want RUN_FINISHED", o.events[len(o.events)-1])
	}
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.RunStarted_SetsThreadAndRunId
func TestCsProtocolRuleTest_RunStarted_SetsThreadAndRunId(t *testing.T) {
	o := feed(t,
		ev("RUN_STARTED", "threadId", "thread-42", "runId", "run-7"),
		ev("RUN_FINISHED", "threadId", "thread-42", "runId", "run-7"),
	)
	o.accepted(t)
	rs, ok := o.events[0].(*agui.RunStartedEvent)
	if !ok {
		t.Fatalf("first event %T", o.events[0])
	}
	eq(t, "threadId", rs.ThreadID, "thread-42")
	eq(t, "runId", rs.RunID, "run-7")
}

// ────────────────────────────────────────────────
// Step events
// ────────────────────────────────────────────────

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Steps_DuringTextMessage_DoNotInterfere
func TestCsProtocolRuleTest_Steps_DuringTextMessage_DoNotInterfere(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		stepStarted("planning"),
		textStart("m1"),
		textContent("m1", "Hello"),
		stepFinished("planning"),
		textEnd("m1"),
		runFinished("r1"),
	)
	o.accepted(t)
	eqStrings(t, "texts", o.texts(), []string{"Hello"})
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Steps_DuringToolCall_DoNotInterfere
func TestCsProtocolRuleTest_Steps_DuringToolCall_DoNotInterfere(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		toolStart("tc1", "calc"),
		stepStarted("compute"),
		toolArgs("tc1", `{"x":1}`),
		stepFinished("compute"),
		toolEnd("tc1"),
		runFinished("r1"),
	)
	o.accepted(t)
	calls := o.calls()
	if len(calls) != 1 {
		t.Fatalf("tool calls: %d, want 1", len(calls))
	}
	eq(t, "name", calls[0].Function.Name, "calc")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Steps_FinishMismatchedName_Throws
func TestCsProtocolRuleTest_Steps_FinishMismatchedName_Throws(t *testing.T) {
	o := feed(t, runStarted("r1"), stepStarted("step1"), stepFinished("wrong_name"))
	o.protocolRejected(t, "wrong_name", "not started")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Steps_FinishWithoutStart_Throws
func TestCsProtocolRuleTest_Steps_FinishWithoutStart_Throws(t *testing.T) {
	o := feed(t, runStarted("r1"), stepFinished("never_started"))
	o.protocolRejected(t, "never_started", "not started")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Steps_DuplicateName_Throws
func TestCsProtocolRuleTest_Steps_DuplicateName_Throws(t *testing.T) {
	o := feed(t, runStarted("r1"), stepStarted("step1"), stepStarted("step1"))
	o.protocolRejected(t, "step1", "already active")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Steps_ConcurrentDifferentNames_Succeeds
func TestCsProtocolRuleTest_Steps_ConcurrentDifferentNames_Succeeds(t *testing.T) {
	o := feed(t,
		runStarted("r1"),
		stepStarted("step1"),
		stepStarted("step2"),
		stepFinished("step2"),
		stepFinished("step1"),
		runFinished("r1"),
	)
	o.accepted(t)
	var names []string
	for _, e := range o.of(agui.EventTypeStepStarted) {
		names = append(names, e.(*agui.StepStartedEvent).StepName)
	}
	eqStrings(t, "STEP_STARTED names", names, []string{"step1", "step2"})
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Steps_ActiveAtRunFinished_Throws
func TestCsProtocolRuleTest_Steps_ActiveAtRunFinished_Throws(t *testing.T) {
	o := feed(t, runStarted("r1"), stepStarted("unfinished"), runFinished("r1"))
	o.protocolRejected(t, "unfinished", "steps are still active")
}

// ────────────────────────────────────────────────
// Lifecycle validation
// ────────────────────────────────────────────────

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Lifecycle_FirstEventMustBeRunStarted
func TestCsProtocolRuleTest_Lifecycle_FirstEventMustBeRunStarted(t *testing.T) {
	o := feed(t, textStart("m1"))
	o.protocolRejected(t, "First event must be 'RUN_STARTED'")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Lifecycle_RunErrorAsFirstEvent_IsAllowed
func TestCsProtocolRuleTest_Lifecycle_RunErrorAsFirstEvent_IsAllowed(t *testing.T) {
	o := feed(t, runError("Immediate failure"))
	o.accepted(t)
	eq(t, "delivered", len(o.events), 1)
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Lifecycle_NoEventsAfterRunFinished
func TestCsProtocolRuleTest_Lifecycle_NoEventsAfterRunFinished(t *testing.T) {
	o := feed(t, runStarted("r1"), runFinished("r1"), textStart("m1"))
	o.protocolRejected(t, "already finished")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Lifecycle_NoEventsAfterRunError
func TestCsProtocolRuleTest_Lifecycle_NoEventsAfterRunError(t *testing.T) {
	o := feed(t, runStarted("r1"), runError("boom"), textStart("m1"))
	o.protocolRejected(t, "already errored")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Lifecycle_CannotStartNewRunWhileActive
func TestCsProtocolRuleTest_Lifecycle_CannotStartNewRunWhileActive(t *testing.T) {
	o := feed(t, runStarted("r1"), runStarted("r2"))
	o.protocolRejected(t, "run is still active")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Lifecycle_RunFinishedWithActiveMessages_Throws
func TestCsProtocolRuleTest_Lifecycle_RunFinishedWithActiveMessages_Throws(t *testing.T) {
	o := feed(t, runStarted("r1"), textStart("m1"), textContent("m1", "Hello"), runFinished("r1"))
	o.protocolRejected(t, "text messages are still active", "m1")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Lifecycle_RunFinishedWithActiveToolCalls_Throws
func TestCsProtocolRuleTest_Lifecycle_RunFinishedWithActiveToolCalls_Throws(t *testing.T) {
	o := feed(t, runStarted("r1"), toolStart("tc1", "fetch"), runFinished("r1"))
	o.protocolRejected(t, "tool calls are still active", "tc1")
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.Lifecycle_RunErrorAfterRunFinished_StillTerminal
func TestCsProtocolRuleTest_Lifecycle_RunErrorAfterRunFinished_StillTerminal(t *testing.T) {
	o := feed(t, runStarted("r1"), runFinished("r1"), runError("late error"), textStart("m1"))
	o.protocolRejected(t, "already errored")
	// The late RUN_ERROR itself was accepted and fails the run (lifecycle.mdx:
	// "A consumer MUST treat the run as failed in that case").
	if o.res == nil || len(o.res.Errors) != 1 || o.res.Errors[0].Message != "late error" {
		t.Errorf("late RUN_ERROR not surfaced: %+v", o.res)
	}
	eq(t, "status", o.res.Status, client.StatusFailed)
}

// ────────────────────────────────────────────────
// Multi-run support
// ────────────────────────────────────────────────

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.MultiRun_SequentialRuns_Succeeds
func TestCsProtocolRuleTest_MultiRun_SequentialRuns_Succeeds(t *testing.T) {
	o := feed(t,
		runStarted("r1"), textStart("m1"), textContent("m1", "Run 1"), textEnd("m1"), runFinished("r1"),
		runStarted("r2"), textStart("m2"), textContent("m2", "Run 2"), textEnd("m2"), runFinished("r2"),
	)
	o.accepted(t)
	eqStrings(t, "texts", o.texts(), []string{"Run 1", "Run 2"})
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.MultiRun_ReuseMessageIdsAcrossRuns_Succeeds
func TestCsProtocolRuleTest_MultiRun_ReuseMessageIdsAcrossRuns_Succeeds(t *testing.T) {
	o := feed(t,
		runStarted("r1"), textStart("m1"), textContent("m1", "First"), textEnd("m1"), runFinished("r1"),
		runStarted("r2"), textStart("m1"), textContent("m1", "Second"), textEnd("m1"), runFinished("r2"),
	)
	o.accepted(t)
	eqStrings(t, "texts", o.texts(), []string{"First", "Second"})
	// Go has a message store, .NET does not. lifecycle.mdx: "Re-streaming a
	// message the consumer already has appends to it rather than restating it."
	m, ok := o.message(t, "m1").(*agui.AssistantMessage)
	if !ok {
		t.Fatalf("m1 is %T", o.message(t, "m1"))
	}
	eq(t, "m1 content", agui.Deref(m.Content), "FirstSecond")
	eq(t, "message count", len(o.agent.Store.Messages), 1)
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.MultiRun_WithToolCalls_Succeeds
func TestCsProtocolRuleTest_MultiRun_WithToolCalls_Succeeds(t *testing.T) {
	o := feed(t,
		runStarted("r1"), toolStart("tc1", "search"), toolArgs("tc1", "{}"), toolEnd("tc1"), runFinished("r1"),
		runStarted("r2"), toolStart("tc2", "fetch"), toolArgs("tc2", "{}"), toolEnd("tc2"), runFinished("r2"),
	)
	o.accepted(t)
	var names []string
	for _, c := range o.calls() {
		names = append(names, c.Function.Name)
	}
	eqStrings(t, "tool calls", names, []string{"search", "fetch"})
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.MultiRun_WithSteps_Succeeds
func TestCsProtocolRuleTest_MultiRun_WithSteps_Succeeds(t *testing.T) {
	o := feed(t,
		runStarted("r1"), stepStarted("plan"), stepFinished("plan"), runFinished("r1"),
		runStarted("r2"), stepStarted("plan"), stepFinished("plan"), runFinished("r2"),
	)
	o.accepted(t)
	eq(t, "RUN_FINISHED", o.count(agui.EventTypeRunFinished), 2)
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.MultiRun_ThreeSequentialRuns_Succeeds
func TestCsProtocolRuleTest_MultiRun_ThreeSequentialRuns_Succeeds(t *testing.T) {
	o := feed(t,
		runStarted("r1"), runFinished("r1"),
		runStarted("r2"), runFinished("r2"),
		runStarted("r3"), runFinished("r3"),
	)
	o.accepted(t)
	eq(t, "RUN_STARTED", o.count(agui.EventTypeRunStarted), 3)
	eq(t, "RUN_FINISHED", o.count(agui.EventTypeRunFinished), 3)
}

// C#: tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs::ProtocolRuleTest.MultiRun_RunErrorBlocksAnotherRunInSameStream
//
// ADAPTED TO SPEC. The C# test pins a KNOWN .NET GAP: its converter's
// runError latch rejects every later event, RUN_STARTED included (corpus
// run-error-then-new-run-continues.json, expectOverrides.dotnet: "The .NET
// converter forbids any event after RUN_ERROR ... with no RUN_STARTED
// exemption"). lifecycle.mdx: "A producer MUST NOT emit anything after
// RUN_ERROR except RUN_STARTED" and the state diagram has
// "Failed --> Active: RUN_STARTED (a new run)". So the Go test asserts the
// spec rule: the new run starts. The stream here then ends without closing r2,
// so that run is truncated — but nothing is rejected.
func TestCsProtocolRuleTest_MultiRun_RunErrorBlocksAnotherRunInSameStream(t *testing.T) {
	o := feed(t, runStarted("r1"), runError("boom"), runStarted("r2"))
	o.accepted(t)
	eq(t, "RUN_STARTED delivered", o.count(agui.EventTypeRunStarted), 2)
	eq(t, "RUN_ERROR delivered", len(o.res.Errors), 1)
	eq(t, "status of the open run r2", o.res.Status, client.StatusTruncated)

	// And the spec's positive case: the second run completes normally.
	o = feed(t, runStarted("r1"), runError("boom"), runStarted("r2"), runFinished("r2"))
	o.accepted(t)
	eq(t, "status after r2 finished", o.res.Status, client.StatusSucceeded)
	eq(t, "earlier RUN_ERROR still reported", len(o.res.Errors), 1)
}
