package server_test

// Port of sdks/dotnet/tests/AGUI.Hosting.AspNetCore.IntegrationTests/CustomAndRawEventIntegrationTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
	"github.com/ricardogayer/go-agui/pkg/stream"
)

func passthroughClient(t *testing.T, format string, emit func(s *stream.SSE, must func(error))) *testClient {
	c := newClientFormat(t, agent(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		emit(s, must)
		must(s.RunFinished(in.ThreadID, in.RunID))
	}), format)
	c.say("u1", "Hi")
	return c
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/CustomAndRawEventIntegrationTest.cs::CustomAndRawEventIntegrationTest.PostRun_CustomEvent_MapsToUpdateWithRawRepresentation
func TestCsCustomAndRawEventIntegrationTest_PostRun_CustomEvent_MapsToUpdateWithRawRepresentation(t *testing.T) {
	forFormats(t, func(t *testing.T, format string) *testClient {
		c := passthroughClient(t, format, func(s *stream.SSE, must func(error)) {
			must(s.CustomEvent("user_preference_updated", map[string]any{"theme": "dark", "notifications": true}))
		})
		res := c.runOK(t, client.RunInput{})
		if len(res.Events) != 3 {
			t.Fatalf("events %v", types(res.Events))
		}
		custom := res.Events[1].(*agui.CustomEvent)
		if custom.Name != "user_preference_updated" || custom.Value.(map[string]any)["theme"] != "dark" {
			t.Fatalf("custom %+v", custom)
		}
		return c
	})
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/CustomAndRawEventIntegrationTest.cs::CustomAndRawEventIntegrationTest.PostRun_RawEvent_MapsToUpdateWithRawRepresentation
func TestCsCustomAndRawEventIntegrationTest_PostRun_RawEvent_MapsToUpdateWithRawRepresentation(t *testing.T) {
	forFormats(t, func(t *testing.T, format string) *testClient {
		c := passthroughClient(t, format, func(s *stream.SSE, must func(error)) {
			must(s.RawEvent(map[string]any{"action": "button_click", "elementId": "submit-btn"}, agui.Ptr("frontend")))
		})
		res := c.runOK(t, client.RunInput{})
		if len(res.Events) != 3 {
			t.Fatalf("events %v", types(res.Events))
		}
		raw := res.Events[1].(*agui.RawEvent)
		if agui.Deref(raw.Source) != "frontend" || raw.Event.(map[string]any)["action"] != "button_click" {
			t.Fatalf("raw %+v", raw)
		}
		return c
	})
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/CustomAndRawEventIntegrationTest.cs::CustomAndRawEventIntegrationTest.PostRun_CustomAndRawEvents_ShareResponseId
func TestCsCustomAndRawEventIntegrationTest_PostRun_CustomAndRawEvents_ShareResponseId(t *testing.T) {
	forFormats(t, func(t *testing.T, format string) *testClient {
		c := passthroughClient(t, format, func(s *stream.SSE, must func(error)) {
			must(s.CustomEvent("test_custom", map[string]any{"key": "value"}))
			must(s.RawEvent(map[string]any{"data": "test"}, agui.Ptr("external")))
		})
		res := c.runOK(t, client.RunInput{})
		if len(res.Events) != 4 {
			t.Fatalf("events %v", types(res.Events))
		}
		assertOneRun(t, res)
		return c
	})
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/CustomAndRawEventIntegrationTest.cs::CustomAndRawEventIntegrationTest.PostRun_RawEventWithoutSource_SourceIsNull
func TestCsCustomAndRawEventIntegrationTest_PostRun_RawEventWithoutSource_SourceIsNull(t *testing.T) {
	forFormats(t, func(t *testing.T, format string) *testClient {
		var w *wireRun
		w = run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
			must(s.RunStarted(in.ThreadID, in.RunID))
			must(s.RawEvent(map[string]any{"data": "no source"}, nil))
			must(s.RunFinished(in.ThreadID, in.RunID))
		})
		if _, ok := w.Docs[1]["source"]; ok {
			t.Fatalf("absent source on the wire: %v", w.Docs[1])
		}
		c := passthroughClient(t, format, func(s *stream.SSE, must func(error)) {
			must(s.RawEvent(map[string]any{"data": "no source"}, nil))
		})
		res := c.runOK(t, client.RunInput{})
		if len(res.Events) != 3 || res.Events[1].(*agui.RawEvent).Source != nil {
			t.Fatalf("events %v", types(res.Events))
		}
		return c
	})
}
