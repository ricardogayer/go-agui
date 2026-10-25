package agui_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	agui "github.com/ricardogayer/go-agui"
	"github.com/ricardogayer/go-agui/aguitest"
	wire "github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
	"github.com/ricardogayer/go-agui/pkg/protobuf"
)

// The behaviours AG-UI 1.0 requires of a producer that the agent loop has to
// get right on its own. Every run below also goes through aguitest.Run, which
// fails on any event the protocol refuses and replays the stream through a
// conformant consumer.
//
// PT: Os comportamentos que o AG-UI 1.0 exige de um produtor e que o laço do
// agente precisa acertar sozinho. Todo run abaixo também passa por
// aguitest.Run, que reprova qualquer evento recusado pelo protocolo e repassa
// o stream por um consumidor conforme.

func TestRunStartedDeclaresTheProtocolVersion(t *testing.T) {
	evs := aguitest.Run(t, newAgent(t, aguitest.NewFakeModel()), aguitest.Input("hi"))

	if got := evs[0].String("protocolVersion"); got != agui.ProtocolVersion {
		t.Fatalf("RUN_STARTED protocolVersion = %q, want %q", got, agui.ProtocolVersion)
	}
}

func TestFrontendToolEndsTheRunWithPendingToolCallIDs(t *testing.T) {
	m := aguitest.NewFakeModel(aguitest.Turn{ToolCalls: []aguitest.FakeToolCall{
		{ID: "call_server", Name: "add", Args: `{"a":1,"b":2}`},
		{ID: "call_client", Name: "showCard", Args: `{}`},
	}})
	ag := newAgent(t, m, agui.WithTools(addTool()))
	in := aguitest.Input("show a card")
	in.Tools = []agui.ClientTool{{Name: "showCard", Description: "Renders a card."}}

	evs := aguitest.Run(t, ag, in)

	final := evs[len(evs)-1]
	outcome, _ := final["outcome"].(map[string]any)
	if outcome["type"] != "success" {
		t.Fatalf("outcome = %v, want the success outcome", final["outcome"])
	}
	pending, _ := outcome["pendingToolCallIds"].([]any)
	if len(pending) != 1 || pending[0] != "call_client" {
		t.Fatalf("pendingToolCallIds = %v, want exactly [call_client]", pending)
	}
	// The server call of the same turn is not executed, but it is answered:
	// otherwise it would count as pending too.
	//
	// PT: A chamada de servidor do mesmo turno não é executada, mas é
	// respondida: senão também contaria como pendente.
	res, ok := aguitest.First(evs, agui.EventToolCallResult)
	if !ok || res.String("toolCallId") != "call_server" {
		t.Fatalf("the skipped server call needs a TOOL_CALL_RESULT, got %v", res)
	}
}

func TestMalformedToolCallIsAnsweredOnTheWire(t *testing.T) {
	m := aguitest.NewFakeModel(
		aguitest.Turn{ToolCalls: []aguitest.FakeToolCall{
			{ID: "call_bad", Name: "add", Args: `{not json`},
		}},
		aguitest.Turn{Text: "sorry"},
	)
	evs := aguitest.Run(t, newAgent(t, m, agui.WithTools(addTool())), aguitest.Input("add"))

	res, ok := aguitest.First(evs, agui.EventToolCallResult)
	if !ok || res.String("toolCallId") != "call_bad" {
		t.Fatalf("the started malformed call needs a TOOL_CALL_RESULT, got %v", res)
	}
	if outcome := evs[len(evs)-1]["outcome"]; outcome != nil {
		t.Fatalf("nothing is pending, the run must end with a plain success: %v", outcome)
	}
}

func TestRepeatedToolCallIDsAreRenamedOnTheWire(t *testing.T) {
	// Some providers number the calls again on every turn.
	// PT: Alguns provedores renumeram as chamadas a cada turno.
	m := aguitest.NewFakeModel(
		aguitest.Turn{ToolCalls: []aguitest.FakeToolCall{{ID: "call_0", Name: "add", Args: `{"a":1,"b":1}`}}},
		aguitest.Turn{ToolCalls: []aguitest.FakeToolCall{{ID: "call_0", Name: "add", Args: `{"a":2,"b":2}`}}},
		aguitest.Turn{Text: "done"},
	)
	evs := aguitest.Run(t, newAgent(t, m, agui.WithTools(addTool())), aguitest.Input("add twice"))

	starts := aguitest.All(evs, agui.EventToolCallStart)
	if len(starts) != 2 {
		t.Fatalf("got %d TOOL_CALL_START, want 2", len(starts))
	}
	first, second := starts[0].String("toolCallId"), starts[1].String("toolCallId")
	if first != "call_0" || second == "call_0" || second == "" {
		t.Fatalf("tool call ids = %q, %q: the repeated one must be renamed", first, second)
	}
	// The model sees the renamed id too, paired with its answer.
	// PT: O modelo também vê o id renomeado, pareado com a resposta.
	var paired bool
	for _, msg := range m.LastCall() {
		if msg.Role == schema.Tool && msg.ToolCallID == second {
			paired = true
		}
	}
	if !paired {
		t.Fatalf("the renamed call %q has no answer in the model input", second)
	}
}

func TestResumeThatDoesNotCoverAnInterruptInterruptsAgain(t *testing.T) {
	var executed int
	m := aguitest.NewFakeModel(aguitest.Turn{Text: "should not be reached"})
	ag := newAgent(t, m, agui.WithTools(purchaseTool(t, &executed)))

	in := aguitest.Input("buy two")
	in.Messages = append(in.Messages, &agui.AssistantMessage{
		ID: "msg_2",
		ToolCalls: []agui.ToolCall{
			{ID: "call_1", Function: agui.FunctionCall{Name: "purchase", Arguments: `{"amount":1}`}},
			{ID: "call_2", Function: agui.FunctionCall{Name: "purchase", Arguments: `{"amount":2}`}},
		},
	})
	in.Resume = []agui.ResumeEntry{{
		InterruptID: "call_1", Status: agui.ResumeStatusResolved, Payload: "ok",
	}}

	evs := aguitest.Run(t, ag, in)

	if executed != 1 {
		t.Fatalf("executed %d purchases, want only the covered one", executed)
	}
	if n := len(m.Calls()); n != 0 {
		t.Fatalf("the model was called %d times past an open interrupt", n)
	}
	raw := mustJSON(t, evs[len(evs)-1]["outcome"])
	if !strings.Contains(raw, `"interrupt"`) || !strings.Contains(raw, "call_2") || strings.Contains(raw, `"call_1"`) {
		t.Fatalf("outcome = %s, want the interrupt outcome for call_2 only", raw)
	}
}

func TestTokenUsageIsReportedOnTheTerminalEvent(t *testing.T) {
	m := aguitest.NewFakeModel(
		aguitest.Turn{
			ToolCalls: []aguitest.FakeToolCall{{ID: "call_1", Name: "add", Args: `{"a":1,"b":1}`}},
			Usage:     &schema.TokenUsage{PromptTokens: 10, CompletionTokens: 2, TotalTokens: 12},
		},
		aguitest.Turn{
			Text:  "2",
			Usage: &schema.TokenUsage{PromptTokens: 20, CompletionTokens: 1, TotalTokens: 21},
		},
	)
	evs := aguitest.Run(t, newAgent(t, m, agui.WithTools(addTool())), aguitest.Input("1+1"))

	usage, _ := evs[len(evs)-1]["usage"].([]any)
	if len(usage) != 1 {
		t.Fatalf("usage = %v, want one entry", evs[len(evs)-1]["usage"])
	}
	u := usage[0].(map[string]any)
	if u["inputTokens"] != 30.0 || u["outputTokens"] != 3.0 || u["totalTokens"] != 33.0 {
		t.Fatalf("usage = %v, want the sum of both turns (30/3/33)", u)
	}
}

func TestSnapshotKeepsRequestMessagesVerbatim(t *testing.T) {
	m := aguitest.NewFakeModel(aguitest.Turn{Text: "a cat"})
	in := aguitest.Input("ignored")
	in.Messages = agui.Messages{
		&agui.ReasoningMessage{ID: "r1", Content: "earlier reasoning"},
		&agui.UserMessage{ID: "u1", Content: agui.PartsContent(
			wire.NewTextPart("what is this?"),
			wire.NewImagePart(&wire.URLSource{Value: "https://example.com/cat.png"}),
		)},
	}

	evs := aguitest.Run(t, newAgent(t, m), in)

	snap, ok := aguitest.First(evs, agui.EventMessagesSnapshot)
	if !ok {
		t.Fatal("no MESSAGES_SNAPSHOT")
	}
	raw := mustJSON(t, snap["messages"])
	for _, want := range []string{`"earlier reasoning"`, `cat.png`, `"what is this?"`, `"a cat"`} {
		if !strings.Contains(raw, want) {
			t.Errorf("the snapshot lost %s:\n%s", want, raw)
		}
	}
	// The model only got the text of the user turn.
	// PT: O modelo só recebeu o texto do turno do usuário.
	if got := m.LastCall()[len(m.LastCall())-1].Content; got != "what is this?" {
		t.Errorf("model input = %q", got)
	}
}

func TestMalformedKnownValueIsRefusedBeforeTheStream(t *testing.T) {
	h := agui.Handler(agui.RunnerFunc(
		func(context.Context, *agui.Emitter, *agui.RunAgentInput) error {
			t.Fatal("the runner should not be called")
			return nil
		}))
	// role "user" with a numeric content is a malformed known value.
	// PT: role "user" com content numérico é um valor conhecido malformado.
	body := `{"threadId":"t","runId":"r","messages":[{"id":"u1","role":"user","content":42}]}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestUnknownFieldsAreDroppedNotFatal(t *testing.T) {
	h := agui.Handler(newAgent(t, aguitest.NewFakeModel()))
	body := `{"threadId":"t","runId":"r","futureField":1,"messages":[{"id":"u1","role":"user","content":"hi","extra":true}]}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	aguitest.RequireConformant(t, aguitest.ParseSSE(t, rec.Body.Bytes()))
}

func TestOtherMajorVersionCanBeRejected(t *testing.T) {
	body := `{"threadId":"t","runId":"r","protocolVersion":"2.0","messages":[]}`
	for _, tc := range []struct {
		opts []agui.HandlerOption
		want int
	}{
		{nil, http.StatusOK},
		{[]agui.HandlerOption{agui.WithRejectOtherMajorVersions()}, http.StatusBadRequest},
	} {
		h := agui.Handler(newAgent(t, aguitest.NewFakeModel()), tc.opts...)
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("status = %d, want %d", rec.Code, tc.want)
		}
	}
}

func TestProtobufBindingIsNegotiated(t *testing.T) {
	body := `{"threadId":"t","runId":"r","messages":[{"id":"u1","role":"user","content":"hi"}]}`
	for _, tc := range []struct {
		accept string
		opts   []agui.HandlerOption
		want   string
	}{
		{"text/event-stream", nil, "text/event-stream"},
		{protobuf.ContentType, nil, protobuf.ContentType},
		{protobuf.ContentType, []agui.HandlerOption{agui.WithoutProtobuf()}, "text/event-stream"},
	} {
		h := agui.Handler(newAgent(t, aguitest.NewFakeModel()), tc.opts...)
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req.Header.Set("Accept", tc.accept)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if got := rec.Header().Get("Content-Type"); got != tc.want {
			t.Errorf("Accept %q: Content-Type = %q, want %q", tc.accept, got, tc.want)
		}
	}
}

// The reference consumer (package client) talks to an agui.Agent over real
// HTTP, on both bindings, through a frontend tool round trip.
//
// PT: O consumidor de referência (pacote client) conversa com um agui.Agent
// por HTTP de verdade, nos dois bindings, num ciclo com tool de frontend.
func TestReferenceClientInteroperatesOnBothBindings(t *testing.T) {
	for _, onlySSE := range []bool{false, true} {
		m := aguitest.NewFakeModel(
			aguitest.Turn{ToolCalls: []aguitest.FakeToolCall{{ID: "call_1", Name: "confirm", Args: `{}`}}},
			aguitest.Turn{Text: "confirmed, thanks"},
		)
		srv := httptest.NewServer(agui.Handler(newAgent(t, m)))

		c := client.New(srv.URL)
		c.DisableProtobuf = onlySSE
		var warnings []string
		c.Warn = func(w wire.Warning) { warnings = append(warnings, w.String()) }
		c.AddMessage(&agui.UserMessage{ID: "u1", Content: agui.TextContent("go")})
		tools := []agui.ClientTool{{Name: "confirm", Description: "asks the user"}}

		res, err := c.Run(context.Background(), client.RunInput{Tools: tools}, nil)
		if err != nil {
			t.Fatalf("protobuf=%v: first run: %v", !onlySSE, err)
		}
		if res.Status != client.StatusSucceeded || len(res.PendingToolCallIDs) != 1 {
			t.Fatalf("protobuf=%v: status %s, pending %v", !onlySSE, res.Status, res.PendingToolCallIDs)
		}
		c.AddMessage(&agui.ToolMessage{ID: "t1", ToolCallID: res.PendingToolCallIDs[0], Content: agui.TextContent("yes")})

		res, err = c.Run(context.Background(), client.RunInput{Tools: tools}, nil)
		if err != nil || !res.Succeeded() {
			t.Fatalf("protobuf=%v: second run: %v %v", !onlySSE, err, res)
		}
		last, ok := c.Store.Messages[len(c.Store.Messages)-1].(*agui.AssistantMessage)
		if !ok || wire.Deref(last.Content) != "confirmed, thanks" {
			t.Fatalf("protobuf=%v: last message = %#v", !onlySSE, c.Store.Messages[len(c.Store.Messages)-1])
		}
		if len(warnings) > 0 {
			t.Errorf("protobuf=%v: the client warned: %v", !onlySSE, warnings)
		}
		srv.Close()
	}
}
