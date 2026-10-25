package server_test

// Port of sdks/dotnet/tests/AGUI.Hosting.AspNetCore.IntegrationTests/TransportEquivalenceIntegrationTest.cs.
//
// The same scenario over HTTP + SSE and HTTP + Protobuf must decode to the
// same events (ids that are random per request excluded).

import (
	"reflect"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
	"github.com/ricardogayer/go-agui/pkg/stream"
)

// describe projects events to a transport- and run-independent shape: the
// event as JSON without timestamps and run identity.
func describe(t *testing.T, events []agui.Event) []string {
	var out []string
	for _, e := range events {
		g := toGeneric(t, e).(map[string]any)
		delete(g, "timestamp")
		delete(g, "threadId")
		delete(g, "runId")
		delete(g, "messageId")
		out = append(out, canon(g))
	}
	return out
}

func equivalentOverBindings(t *testing.T, emit func(s *stream.SSE, must func(error))) {
	t.Helper()
	var got [][]string
	for _, f := range []string{formatJSON, formatProtobuf} {
		c := newClientFormat(t, agent(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
			must(s.RunStarted(in.ThreadID, in.RunID))
			emit(s, must)
			must(s.RunFinished(in.ThreadID, in.RunID))
		}), f)
		c.say("u1", "Hi")
		res := c.runOK(t, client.RunInput{})
		if want := map[string]string{formatJSON: "text/event-stream", formatProtobuf: protoMediaType}[f]; c.contentTypes[0] != want {
			t.Fatalf("%s: Content-Type %q", f, c.contentTypes[0])
		}
		got = append(got, describe(t, res.Events))
	}
	if !reflect.DeepEqual(got[0], got[1]) {
		t.Fatalf("bindings disagree\n  sse: %v\nproto: %v", got[0], got[1])
	}
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/TransportEquivalenceIntegrationTest.cs::TransportEquivalenceIntegrationTest.TextScenario_JsonAndProtobuf_ProduceIdenticalDecodedUpdates
func TestCsTransportEquivalenceIntegrationTest_TextScenario_JsonAndProtobuf_ProduceIdenticalDecodedUpdates(t *testing.T) {
	equivalentOverBindings(t, func(s *stream.SSE, must func(error)) {
		_, err := s.TextMessage("Identical across transports!")
		must(err)
	})
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/TransportEquivalenceIntegrationTest.cs::TransportEquivalenceIntegrationTest.StateScenario_JsonAndProtobuf_ProduceIdenticalDecodedUpdates
func TestCsTransportEquivalenceIntegrationTest_StateScenario_JsonAndProtobuf_ProduceIdenticalDecodedUpdates(t *testing.T) {
	equivalentOverBindings(t, func(s *stream.SSE, must func(error)) {
		must(s.StateSnapshot(map[string]any{"counter": 0}))
		must(s.StateDelta(agui.Replace("/counter", 1)))
	})
}
