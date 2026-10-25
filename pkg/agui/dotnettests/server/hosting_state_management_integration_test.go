package server_test

// Port of sdks/dotnet/tests/AGUI.Hosting.AspNetCore.IntegrationTests/StateManagementIntegrationTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
	"github.com/ricardogayer/go-agui/pkg/stream"
)

func stateClient(t *testing.T, format string, withDelta bool) *testClient {
	c := newClientFormat(t, agent(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.StateSnapshot(map[string]any{"counter": 0, "items": []any{}}))
		if withDelta {
			must(s.StateDelta(agui.Replace("/counter", 1), agui.Add("/items/-", "item1")))
		}
		must(s.RunFinished(in.ThreadID, in.RunID))
	}), format)
	c.say("u1", "Hi")
	return c
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/StateManagementIntegrationTest.cs::StateManagementIntegrationTest.PostRun_StateSnapshot_MapsToUpdateWithRawRepresentation
func TestCsStateManagementIntegrationTest_PostRun_StateSnapshot_MapsToUpdateWithRawRepresentation(t *testing.T) {
	forFormats(t, func(t *testing.T, format string) *testClient {
		c := stateClient(t, format, false)
		res := c.runOK(t, client.RunInput{})
		if len(res.Events) != 3 {
			t.Fatalf("events %v", types(res.Events))
		}
		if snap := res.Events[1].(*agui.StateSnapshotEvent).Snapshot.(map[string]any); snap["counter"] != float64(0) {
			t.Fatalf("snapshot %v", snap)
		}
		if canon(c.Store.State) != `{"counter":0,"items":[]}` {
			t.Fatalf("store state %s", canon(c.Store.State))
		}
		return c
	})
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/StateManagementIntegrationTest.cs::StateManagementIntegrationTest.PostRun_StateDelta_MapsToUpdateWithRawRepresentation
func TestCsStateManagementIntegrationTest_PostRun_StateDelta_MapsToUpdateWithRawRepresentation(t *testing.T) {
	forFormats(t, func(t *testing.T, format string) *testClient {
		c := stateClient(t, format, true)
		res := c.runOK(t, client.RunInput{})
		if len(res.Events) != 4 {
			t.Fatalf("events %v", types(res.Events))
		}
		if _, ok := res.Events[1].(*agui.StateSnapshotEvent); !ok {
			t.Fatal("events[1] is not STATE_SNAPSHOT")
		}
		delta := res.Events[2].(*agui.StateDeltaEvent)
		if len(delta.Delta) != 2 || delta.Delta[0].Op != "replace" || delta.Delta[1].Op != "add" {
			t.Fatalf("delta %+v", delta.Delta)
		}
		if canon(c.Store.State) != `{"counter":1,"items":["item1"]}` {
			t.Fatalf("store state %s", canon(c.Store.State))
		}
		return c
	})
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/StateManagementIntegrationTest.cs::StateManagementIntegrationTest.PostRun_StateEvents_ShareResponseId
func TestCsStateManagementIntegrationTest_PostRun_StateEvents_ShareResponseId(t *testing.T) {
	forFormats(t, func(t *testing.T, format string) *testClient {
		c := stateClient(t, format, true)
		assertOneRun(t, c.runOK(t, client.RunInput{}))
		return c
	})
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/StateManagementIntegrationTest.cs::StateManagementIntegrationTest.PostRun_WithStateInRawRepresentationFactory_StateArrivesOnServer
func TestCsStateManagementIntegrationTest_PostRun_WithStateInRawRepresentationFactory_StateArrivesOnServer(t *testing.T) {
	forFormats(t, func(t *testing.T, format string) *testClient {
		var seen []*agui.RunAgentInput
		c := emptyRun(t, format, &seen)
		c.ThreadID = "state-thread"
		c.Store.State = map[string]any{"counter": 5, "name": "test"}
		c.say("u1", "Hi")
		c.runOK(t, client.RunInput{RunID: "state-run"})
		st, ok := seen[0].State.(map[string]any)
		if !ok || st["counter"] != float64(5) || st["name"] != "test" {
			t.Fatalf("server state %#v", seen[0].State)
		}
		return c
	})
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/StateManagementIntegrationTest.cs::StateManagementIntegrationTest.PostRun_WithNullState_ServerReceivesNullState
func TestCsStateManagementIntegrationTest_PostRun_WithNullState_ServerReceivesNullState(t *testing.T) {
	forFormats(t, func(t *testing.T, format string) *testClient {
		var seen []*agui.RunAgentInput
		c := emptyRun(t, format, &seen)
		c.ThreadID = "no-state-thread"
		c.say("u1", "Hi")
		res := c.runOK(t, client.RunInput{RunID: "no-state-run"})
		if seen[0].State != nil {
			t.Fatalf("server state %#v", seen[0].State)
		}
		// Absent on the wire, not null.
		if _, ok := toGeneric(t, res.Request).(map[string]any)["state"]; ok {
			t.Fatal("the request carried a state member")
		}
		return c
	})
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/StateManagementIntegrationTest.cs::StateManagementIntegrationTest.PostRun_WithStateRoundTrip_ServerReceivesAndEchoesState
func TestCsStateManagementIntegrationTest_PostRun_WithStateRoundTrip_ServerReceivesAndEchoesState(t *testing.T) {
	forFormats(t, func(t *testing.T, format string) *testClient {
		c := newClientFormat(t, agent(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
			must(s.RunStarted(in.ThreadID, in.RunID))
			if in.State != nil {
				must(s.StateSnapshot(in.State))
			}
			must(s.RunFinished(in.ThreadID, in.RunID))
		}), format)
		c.ThreadID = "roundtrip-thread"
		c.Store.State = map[string]any{"counter": 42}
		c.say("u1", "Hi")
		res := c.runOK(t, client.RunInput{RunID: "roundtrip-run"})
		if len(res.Events) != 3 {
			t.Fatalf("events %v", types(res.Events))
		}
		if snap := res.Events[1].(*agui.StateSnapshotEvent).Snapshot.(map[string]any); snap["counter"] != float64(42) {
			t.Fatalf("snapshot %v", snap)
		}
		return c
	})
}
