package server_test

// Port of sdks/dotnet/tests/AGUI.Hosting.AspNetCore.IntegrationTests/ActivityIntegrationTest.cs.
//
// The C# tests read events back as MEAI ChatResponseUpdates (RawRepresentation,
// Role, ResponseId); the Go tests read them from pkg/client (the delivered
// events and the Store the consumer builds).

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
	"github.com/ricardogayer/go-agui/pkg/stream"
)

func activityAgent(t *testing.T, replace *bool, withDelta bool) *testClient {
	c := newClient(t, agent(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		snap := agui.NewActivitySnapshotEvent("msg_activity", "PLAN", map[string]any{"tasks": []any{"search"}})
		snap.Replace = replace
		must(s.Send(snap))
		if withDelta {
			must(s.ActivityDelta("msg_activity", "PLAN", agui.Replace("/tasks/0", "✓ search")))
		}
		must(s.RunFinished(in.ThreadID, in.RunID))
	}))
	c.say("u1", "Hi")
	return c
}

func activityMessage(t *testing.T, c *testClient) *agui.ActivityMessage {
	t.Helper()
	for _, m := range c.Store.Messages {
		if am, ok := m.(*agui.ActivityMessage); ok && am.ID == "msg_activity" {
			return am
		}
	}
	t.Fatalf("no activity message in %+v", c.Store.Messages)
	return nil
}

// assertOneRun is the Go counterpart of the C# "share ResponseId" checks: the
// stateless C# client correlates updates by response id; on the wire, every
// event belongs to the one run the request named.
func assertOneRun(t *testing.T, res *client.RunResult) {
	t.Helper()
	started := single[*agui.RunStartedEvent](t, res.Events)
	finished := single[*agui.RunFinishedEvent](t, res.Events)
	if started.RunID != res.Request.RunID || finished.RunID != res.Request.RunID ||
		started.ThreadID != res.Request.ThreadID || finished.ThreadID != res.Request.ThreadID {
		t.Fatalf("run identity: request %s/%s, started %s/%s, finished %s/%s", res.Request.ThreadID, res.Request.RunID,
			started.ThreadID, started.RunID, finished.ThreadID, finished.RunID)
	}
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/ActivityIntegrationTest.cs::ActivityIntegrationTest.PostRun_ActivitySnapshot_MapsToUpdateWithRawRepresentation
func TestCsActivityIntegrationTest_PostRun_ActivitySnapshot_MapsToUpdateWithRawRepresentation(t *testing.T) {
	c := activityAgent(t, agui.Ptr(true), false)
	res := c.runOK(t, client.RunInput{})
	if len(res.Events) != 3 {
		t.Fatalf("events %v", types(res.Events))
	}
	snap := res.Events[1].(*agui.ActivitySnapshotEvent)
	if snap.MessageID != "msg_activity" || snap.ActivityType != "PLAN" || len(snap.Content["tasks"].([]any)) != 1 {
		t.Fatalf("snapshot %+v", snap)
	}
	if am := activityMessage(t, c); am.ActivityType != "PLAN" || len(am.Content["tasks"].([]any)) != 1 {
		t.Fatalf("activity message %+v", am)
	}
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/ActivityIntegrationTest.cs::ActivityIntegrationTest.PostRun_ActivityDelta_MapsToUpdateWithRawRepresentation
func TestCsActivityIntegrationTest_PostRun_ActivityDelta_MapsToUpdateWithRawRepresentation(t *testing.T) {
	c := activityAgent(t, nil, true)
	res := c.runOK(t, client.RunInput{})
	if len(res.Events) != 4 {
		t.Fatalf("events %v", types(res.Events))
	}
	delta := res.Events[2].(*agui.ActivityDeltaEvent)
	if delta.MessageID != "msg_activity" || delta.ActivityType != "PLAN" || len(delta.Patch) != 1 || delta.Patch[0].Op != "replace" {
		t.Fatalf("delta %+v", delta)
	}
	if am := activityMessage(t, c); am.Content["tasks"].([]any)[0] != "✓ search" {
		t.Fatalf("patched content %+v", am.Content)
	}
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/ActivityIntegrationTest.cs::ActivityIntegrationTest.PostRun_ActivityEvents_ShareResponseId
func TestCsActivityIntegrationTest_PostRun_ActivityEvents_ShareResponseId(t *testing.T) {
	c := activityAgent(t, nil, true)
	assertOneRun(t, c.runOK(t, client.RunInput{}))
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/ActivityIntegrationTest.cs::ActivityIntegrationTest.PostRun_ActivitySnapshotWithReplace_PreservesReplaceFlag
func TestCsActivityIntegrationTest_PostRun_ActivitySnapshotWithReplace_PreservesReplaceFlag(t *testing.T) {
	c := activityAgent(t, agui.Ptr(false), false)
	res := c.runOK(t, client.RunInput{})
	if len(res.Events) != 3 {
		t.Fatalf("events %v", types(res.Events))
	}
	if r := res.Events[1].(*agui.ActivitySnapshotEvent).Replace; r == nil || *r {
		t.Fatalf("replace %v", r)
	}
}
