package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/server"
	"github.com/ricardogayer/go-agui/pkg/stream"
)

// A producer exercising most of the protocol, served by package server and
// consumed by this package: the two halves must agree, quietly.
func featureAgent(ctx context.Context, in *agui.RunAgentInput, s *stream.SSE) error {
	must := func(err error) {
		if err != nil {
			panic(err)
		}
	}
	must(s.RunStarted(in.ThreadID, in.RunID))

	// Second run: the application answered the interrupt.
	if len(in.Resume) > 0 {
		answer := in.Resume[0]
		_, err := s.TextMessage("approved: " + string(answer.Status))
		must(err)
		return s.RunFinished(in.ThreadID, in.RunID)
	}
	// Third run: the frontend tool was answered in messages.
	for _, m := range in.Messages {
		if tm, ok := m.(*agui.ToolMessage); ok && tm.ToolCallID == "front-1" {
			_, err := s.TextMessage("tool said " + tm.Content.String())
			must(err)
			return s.RunFinished(in.ThreadID, in.RunID)
		}
	}

	must(s.StepStarted("plan"))
	_, err := s.Reasoning("thinking it over")
	must(err)
	must(s.StateDelta(agui.Add("/count", 1)))
	must(s.ActivitySnapshot("act-1", "search", map[string]any{"found": 0}))
	must(s.ActivityDelta("act-1", "search", agui.Replace("/found", 2)))
	must(s.SubagentStarted("sub-1", "researcher"))
	must(s.Send(&agui.TextMessageStartEvent{Attribution: agui.Attribution{SubagentRunID: agui.Ptr("sub-1")}, MessageID: "sub-msg"}))
	must(s.TextContent("sub-msg", "from the subagent"))
	must(s.TextEnd("sub-msg"))
	must(s.SubagentFinished("sub-1"))
	must(s.TextChunk("chunked", "Hello, "))
	must(s.TextChunk("", "world."))
	must(s.ToolCall("backend-1", "lookup", `{"q":"x"}`))
	must(s.ToolCallResultParts("result-1", "backend-1", agui.NewTextPart("found"), agui.NewDocumentPart(&agui.FileSource{Value: "file-abc", Provider: agui.Ptr("openai")})))
	must(s.StepFinished("plan"))

	if len(in.Tools) > 0 {
		// Call the frontend tool and leave it for the application.
		must(s.ToolCall("front-1", in.Tools[0].Name, `{}`))
		return s.RunFinishedPending(in.ThreadID, in.RunID, "front-1")
	}
	return s.RunInterrupted(in.ThreadID, in.RunID, agui.Interrupt{ID: "int-1", Reason: "tool_approval", Message: agui.Ptr("May I?")})
}

func TestClientServerRoundTrip(t *testing.T) {
	srv := httptest.NewServer(server.NewAgentHandler(server.RunnerFunc(featureAgent), server.Options{Logger: t.Logf}))
	defer srv.Close()

	agent := New(srv.URL)
	var warnings []string
	agent.Warn = func(w agui.Warning) { warnings = append(warnings, w.String()) }
	agent.AddMessage(&agui.UserMessage{ID: "u1", Content: agui.TextContent("hello")})
	agent.AddMessage(&agui.ActivityMessage{ID: "local-act", ActivityType: "ui", Content: map[string]any{}})

	// Run 1: ends interrupted.
	res, err := agent.Run(context.Background(), RunInput{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusInterrupted || len(agent.PendingInterrupts) != 1 {
		t.Fatalf("status %s, pending %v", res.Status, agent.PendingInterrupts)
	}
	for _, m := range res.Request.Messages {
		if m.MessageRole() == agui.RoleActivity {
			t.Fatal("activity messages must not travel to the producer")
		}
	}
	if agui.Deref(res.Request.ProtocolVersion) != agui.ProtocolVersion {
		t.Fatal("the consumer must declare its protocol version")
	}
	state, _ := agent.Store.State.(map[string]any)
	if state == nil || state["count"] == nil {
		t.Fatalf("state delta not applied: %v", agent.Store.State)
	}
	var sawSub, sawChunk, sawResult, sawActivity, sawReasoning bool
	for _, m := range agent.Store.Messages {
		switch tm := m.(type) {
		case *agui.AssistantMessage:
			if tm.ID == "sub-msg" && agui.Deref(tm.SubagentRunID) == "sub-1" {
				sawSub = true
			}
			if tm.ID == "chunked" && agui.Deref(tm.Content) == "Hello, world." {
				sawChunk = true
			}
		case *agui.ToolMessage:
			sawResult = len(tm.Content.Parts()) == 2
		case *agui.ActivityMessage:
			if tm.ID == "act-1" && tm.Content["found"] != nil {
				sawActivity = true
			}
		case *agui.ReasoningMessage:
			sawReasoning = tm.Content == "thinking it over"
		}
	}
	if !(sawSub && sawChunk && sawResult && sawActivity && sawReasoning) {
		t.Fatalf("store incomplete: sub=%v chunk=%v result=%v activity=%v reasoning=%v", sawSub, sawChunk, sawResult, sawActivity, sawReasoning)
	}

	// A run that does not answer the interrupt is refused before sending.
	if _, err := agent.Run(context.Background(), RunInput{}, nil); err == nil || !strings.Contains(err.Error(), "int-1") {
		t.Fatalf("uncovered interrupt must be refused: %v", err)
	}
	if _, err := agent.Run(context.Background(), RunInput{Resume: []agui.ResumeEntry{{InterruptID: "other", Status: agui.ResumeStatusResolved}}}, nil); err == nil {
		t.Fatal("unknown interrupt id must be refused")
	}

	// Run 2: resume.
	res, err = agent.Run(context.Background(), RunInput{Resume: []agui.ResumeEntry{{InterruptID: "int-1", Status: agui.ResumeStatusResolved, Payload: map[string]any{"ok": true}}}}, nil)
	if err != nil || res.Status != StatusSucceeded || len(agent.PendingInterrupts) != 0 {
		t.Fatalf("resume: %v %v", err, res)
	}

	// Run 3 on a new thread: a frontend tool left pending.
	agent2 := New(srv.URL)
	agent2.Warn = agent.Warn
	agent2.AddMessage(&agui.UserMessage{ID: "u1", Content: agui.TextContent("use the tool")})
	res, err = agent2.Run(context.Background(), RunInput{Tools: []agui.Tool{{Name: "confirm", Description: "asks the user"}}}, nil)
	if err != nil || res.Status != StatusSucceeded || strings.Join(res.PendingToolCallIDs, ",") != "front-1" {
		t.Fatalf("frontend tool: %v %+v", err, res)
	}
	agent2.AddMessage(&agui.ToolMessage{ID: "t1", ToolCallID: "front-1", Content: agui.TextContent("yes")})
	res, err = agent2.Run(context.Background(), RunInput{}, nil)
	if err != nil || !res.Succeeded() {
		t.Fatalf("answer: %v %+v", err, res)
	}
	last := agent2.Store.Messages[len(agent2.Store.Messages)-1].(*agui.AssistantMessage)
	if agui.Deref(last.Content) != "tool said yes" {
		t.Fatalf("got %q", agui.Deref(last.Content))
	}

	if len(warnings) > 0 {
		t.Fatalf("a conformant exchange must be quiet:\n%s", strings.Join(warnings, "\n"))
	}
}

func TestRejectedInput(t *testing.T) {
	srv := httptest.NewServer(server.NewAgentHandler(&server.EchoRunner{}, server.Options{Logger: t.Logf}))
	defer srv.Close()
	agent := New(srv.URL)
	agent.AddMessage(&agui.UserMessage{ID: "u", Content: agui.TextContent("hi")})
	res, err := agent.Run(context.Background(), RunInput{}, nil)
	if err != nil || !res.Succeeded() {
		t.Fatalf("%v %+v", err, res)
	}
	if got := agui.Deref(agent.Store.Messages[1].(*agui.AssistantMessage).Content); got != "Echo: hi" {
		t.Fatalf("got %q", got)
	}
}

func TestTruncatedStream(t *testing.T) {
	agent := New("unused")
	res, err := agent.Consume(strings.NewReader("data: {\"type\":\"RUN_STARTED\",\"threadId\":\"t\",\"runId\":\"r\"}\n\n"), nil)
	if err != nil || res.Status != StatusTruncated {
		t.Fatalf("%v %v", err, res.Status)
	}
}

// TestBothBindings runs the same exchange over each binding: the client
// advertises protobuf by default and the server negotiates it; with
// DisableProtobuf (either side) the exchange is SSE. The outcome is identical.
func TestBothBindings(t *testing.T) {
	for _, tc := range []struct {
		name          string
		clientNoProto bool
		serverNoProto bool
		wantType      string
	}{
		{"protobuf", false, false, "application/vnd.ag-ui.event+proto"},
		{"client asks for SSE", true, false, "text/event-stream"},
		{"server serves SSE", false, true, "text/event-stream"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var served string
			h := server.NewAgentHandler(server.RunnerFunc(featureAgent), server.Options{Logger: t.Logf, DisableProtobuf: tc.serverNoProto})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				h.ServeHTTP(w, r)
				served = w.Header().Get("Content-Type")
			}))
			defer srv.Close()
			agent := New(srv.URL)
			agent.DisableProtobuf = tc.clientNoProto
			var warnings []string
			agent.Warn = func(w agui.Warning) { warnings = append(warnings, w.String()) }
			agent.AddMessage(&agui.UserMessage{ID: "u1", Content: agui.TextContent("hello")})
			res, err := agent.Run(context.Background(), RunInput{}, nil)
			if err != nil || res.Status != StatusInterrupted {
				t.Fatalf("%v %+v", err, res)
			}
			if served != tc.wantType {
				t.Fatalf("served %q, want %q", served, tc.wantType)
			}
			if len(agent.Store.Messages) != 7 || len(warnings) > 0 {
				t.Fatalf("%d messages, warnings %v", len(agent.Store.Messages), warnings)
			}
		})
	}
}
