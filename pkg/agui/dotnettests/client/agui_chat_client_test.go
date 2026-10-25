package client_test

// Port of sdks/dotnet/tests/AGUI.Client.UnitTests/AGUIChatClientTest.cs.
//
// AGUIChatClient is an IChatClient (Microsoft.Extensions.AI) emitting
// ChatResponseUpdate. Its protocol-relevant behaviour — the RunAgentInput it
// sends, and what a consumer makes of the events — maps onto client.Agent:
// ChatOptions.ConversationId / the pinned thread id → Agent.ThreadID, the
// RawRepresentationFactory's RunAgentInput → client.RunInput, the chat
// history → Agent.Store.Messages, updates → RunResult.Events + Store.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
)

func helloEvents() []agui.Event {
	return []agui.Event{
		&agui.RunStartedEvent{ThreadID: "t1", RunID: "r1"},
		&agui.TextMessageStartEvent{MessageID: "m1", Role: agui.Ptr(agui.TextMessageRoleAssistant)},
		&agui.TextMessageContentEvent{MessageID: "m1", Delta: "hi"},
		&agui.TextMessageEndEvent{MessageID: "m1"},
		&agui.RunFinishedEvent{ThreadID: "t1", RunID: "r1"},
	}
}

// C#: tests/AGUI.Client.UnitTests/AGUIChatClientTest.cs::AGUIChatClientTest.GetStreamingResponse_DoesNotSurfaceConversationId
func TestCsAGUIChatClientTest_GetStreamingResponse_DoesNotSurfaceConversationId(t *testing.T) {
	t.Skip("N/A in Go: ChatResponseUpdate.ConversationId is a Microsoft.Extensions.AI concept (a service-managed session that makes MEAI agent wrappers send only deltas); client.Agent has no such field. The protocol rule behind it — a stateless client sends the full history every turn — is covered by TestCsAGUIChatClientTest_GetStreamingResponse_SendsFullHistoryEveryTurn")
}

// C#: tests/AGUI.Client.UnitTests/AGUIChatClientTest.cs::AGUIChatClientTest.GetStreamingResponse_SurfacesThreadIdInAdditionalProperties
//
// .NET surfaces the AG-UI thread id on updates (AdditionalProperties
// "agui_thread_id") and honours a caller-supplied ConversationId as the thread
// id. Go: the caller's Agent.ThreadID is the thread id sent, and the thread id
// is observable on the delivered RUN_STARTED and on RunResult.Request.
func TestCsAGUIChatClientTest_GetStreamingResponse_SurfacesThreadIdInAdditionalProperties(t *testing.T) {
	stub := newStub(t, static(helloEvents()...))
	agent, _ := newAgent(stub.URL)
	agent.ThreadID = "t1"
	agent.AddMessage(user("u1", "hi"))
	res, err := agent.Run(t.Context(), client.RunInput{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := stub.lastInput(t).ThreadID; got != "t1" {
		t.Errorf("threadId sent = %q, want t1", got)
	}
	if res.Request.ThreadID != "t1" {
		t.Errorf("RunResult.Request.ThreadID = %q", res.Request.ThreadID)
	}
	found := false
	for _, e := range res.Events {
		if rs, ok := e.(*agui.RunStartedEvent); ok && rs.ThreadID == "t1" {
			found = true
		}
	}
	if !found {
		t.Error("no delivered event carries thread id t1")
	}
}

// C#: tests/AGUI.Client.UnitTests/AGUIChatClientTest.cs::AGUIChatClientTest.GetStreamingResponse_ReusedOptions_KeepsStableThreadIdWithoutConversationId
//
// Reusing the same Agent across turns keeps one generated thread id.
func TestCsAGUIChatClientTest_GetStreamingResponse_ReusedOptions_KeepsStableThreadIdWithoutConversationId(t *testing.T) {
	stub := newStub(t, echo(
		&agui.TextMessageStartEvent{MessageID: "m1", Role: agui.Ptr(agui.TextMessageRoleAssistant)},
		&agui.TextMessageContentEvent{MessageID: "m1", Delta: "hi"},
		&agui.TextMessageEndEvent{MessageID: "m1"},
	))
	agent, _ := newAgent(stub.URL) // no thread id supplied: one is generated
	agent.AddMessage(user("u1", "turn one"))
	if _, err := agent.Run(t.Context(), client.RunInput{}, nil); err != nil {
		t.Fatal(err)
	}
	first := stub.lastInput(t).ThreadID
	if first == "" {
		t.Fatal("no thread id was sent")
	}
	agent.AddMessage(user("u2", "turn two"))
	if _, err := agent.Run(t.Context(), client.RunInput{}, nil); err != nil {
		t.Fatal(err)
	}
	if got := stub.lastInput(t).ThreadID; got != first {
		t.Fatalf("thread id changed across turns: %q then %q", first, got)
	}
	if agent.ThreadID != first {
		t.Fatalf("Agent.ThreadID = %q, want the pinned %q", agent.ThreadID, first)
	}
}

// C#: tests/AGUI.Client.UnitTests/AGUIChatClientTest.cs::AGUIChatClientTest.GetStreamingResponse_FreshOptionsPerTurn_GeneratesNewThreadId
//
// A fresh client.New per turn (no continuity) yields a different thread id.
func TestCsAGUIChatClientTest_GetStreamingResponse_FreshOptionsPerTurn_GeneratesNewThreadId(t *testing.T) {
	stub := newStub(t, echo())
	a1, _ := newAgent(stub.URL)
	a1.AddMessage(user("u1", "turn one"))
	if _, err := a1.Run(t.Context(), client.RunInput{}, nil); err != nil {
		t.Fatal(err)
	}
	first := stub.lastInput(t).ThreadID
	a2, _ := newAgent(stub.URL)
	a2.AddMessage(user("u1", "turn two"))
	if _, err := a2.Run(t.Context(), client.RunInput{}, nil); err != nil {
		t.Fatal(err)
	}
	if second := stub.lastInput(t).ThreadID; second == first {
		t.Fatalf("both turns used thread id %q", first)
	}
}

// C#: tests/AGUI.Client.UnitTests/AGUIChatClientTest.cs::AGUIChatClientTest.GetStreamingResponse_SendsFullHistoryEveryTurn
func TestCsAGUIChatClientTest_GetStreamingResponse_SendsFullHistoryEveryTurn(t *testing.T) {
	stub := newStub(t, echo())
	agent, _ := newAgent(stub.URL)
	agent.AddMessage(user("u1", "first"))
	agent.AddMessage(&agui.AssistantMessage{ID: "a1", Content: agui.Ptr("reply")})
	agent.AddMessage(user("u2", "second"))
	if _, err := agent.Run(t.Context(), client.RunInput{}, nil); err != nil {
		t.Fatal(err)
	}
	if n := len(stub.lastInput(t).Messages); n != 3 {
		t.Fatalf("sent %d messages, want 3", n)
	}
}

// imageMessage is a user message holding one inline PNG image part.
func imageMessage(metadata any) *agui.UserMessage {
	img := agui.NewImagePart(&agui.DataSource{Value: "AQIDBA==", MimeType: "image/png"}) // bytes 1,2,3,4
	img.Metadata = metadata
	return &agui.UserMessage{ID: "u1", Content: agui.PartsContent(img)}
}

// sentImage runs once and returns the single image part of the single message sent.
func sentImage(t *testing.T, m *agui.UserMessage) map[string]any {
	t.Helper()
	stub := newStub(t, echo())
	agent, _ := newAgent(stub.URL)
	agent.AddMessage(m)
	if _, err := agent.Run(t.Context(), client.RunInput{}, nil); err != nil {
		t.Fatal(err)
	}
	body := stub.lastBody(t)
	msgs := mustAt(t, body, "messages").([]any)
	if len(msgs) != 1 {
		t.Fatalf("sent %d messages", len(msgs))
	}
	if r := mustAt(t, msgs[0], "role"); r != "user" {
		t.Fatalf("role %v", r)
	}
	parts, ok := mustAt(t, msgs[0], "content").([]any)
	if !ok || len(parts) != 1 {
		t.Fatalf("content = %v", mustAt(t, msgs[0], "content"))
	}
	part := parts[0].(map[string]any)
	if part["type"] != "image" {
		t.Fatalf("part type %v, want image", part["type"])
	}
	return part
}

// C#: tests/AGUI.Client.UnitTests/AGUIChatClientTest.cs::AGUIChatClientTest.GetStreamingResponse_MultimodalContent_MapsAdditionalPropertiesToMetadata
//
// Go builds the 1.0 image part directly; its metadata must reach the wire.
func TestCsAGUIChatClientTest_GetStreamingResponse_MultimodalContent_MapsAdditionalPropertiesToMetadata(t *testing.T) {
	part := sentImage(t, imageMessage(map[string]any{"detail": "high"}))
	if got := mustAt(t, part, "metadata.detail"); got != "high" {
		t.Fatalf("metadata.detail = %v", got)
	}
}

// C#: tests/AGUI.Client.UnitTests/AGUIChatClientTest.cs::AGUIChatClientTest.GetStreamingResponse_MultimodalContent_UsesConfiguredSerializerOptionsForMetadata
//
// .NET serializes a caller's metadata model through the configured
// JsonSerializerOptions (snake_case property name). In Go a caller's model is
// serialized through its own json tags; the request must carry them.
func TestCsAGUIChatClientTest_GetStreamingResponse_MultimodalContent_UsesConfiguredSerializerOptionsForMetadata(t *testing.T) {
	type customMetadata struct {
		QualityLevel string `json:"quality_level"`
	}
	part := sentImage(t, imageMessage(map[string]any{"provider": customMetadata{QualityLevel: "high"}}))
	if got := mustAt(t, part, "metadata.provider.quality_level"); got != "high" {
		t.Fatalf("metadata.provider.quality_level = %v", got)
	}
}

// C#: tests/AGUI.Client.UnitTests/AGUIChatClientTest.cs::AGUIChatClientTest.GetStreamingResponse_MultimodalContent_PreservesSerializedMetadataKeysWhenAddingFilename
//
// The metadata object is opaque (schema: "unconstrained"): its keys travel as
// written — no naming policy rewrites providerHint — next to a filename.
func TestCsAGUIChatClientTest_GetStreamingResponse_MultimodalContent_PreservesSerializedMetadataKeysWhenAddingFilename(t *testing.T) {
	part := sentImage(t, imageMessage(map[string]any{"providerHint": "high", "filename": "pixel.png"}))
	if got := mustAt(t, part, "metadata.providerHint"); got != "high" {
		t.Errorf("metadata.providerHint = %v", got)
	}
	if _, ok := at(part, "metadata.provider_hint"); ok {
		t.Error("metadata key was rewritten to provider_hint")
	}
	if got := mustAt(t, part, "metadata.filename"); got != "pixel.png" {
		t.Errorf("metadata.filename = %v", got)
	}
}

// C#: tests/AGUI.Client.UnitTests/AGUIChatClientTest.cs::AGUIChatClientTest.GetStreamingResponse_RawRepresentationFactory_ForwardsContextAndForwardedProperties
func TestCsAGUIChatClientTest_GetStreamingResponse_RawRepresentationFactory_ForwardsContextAndForwardedProperties(t *testing.T) {
	stub := newStub(t, echo())
	agent, _ := newAgent(stub.URL)
	agent.AddMessage(user("u1", "Hello"))
	_, err := agent.Run(t.Context(), client.RunInput{
		Context:        []agui.Context{{Description: "userId", Value: "u-123"}},
		ForwardedProps: map[string]any{"tenant": "acme"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	sent := stub.lastInput(t)
	if len(sent.Context) != 1 || sent.Context[0].Description != "userId" || sent.Context[0].Value != "u-123" {
		t.Errorf("context = %+v", sent.Context)
	}
	fp, ok := sent.ForwardedProps.(map[string]any)
	if !ok {
		t.Fatalf("forwardedProps is %T, want an object", sent.ForwardedProps)
	}
	if fp["tenant"] != "acme" {
		t.Errorf("forwardedProps.tenant = %v", fp["tenant"])
	}
}

// interruptThenEcho interrupts the first run with interrupt id, and echoes
// every later run.
func interruptThenEcho(id string) func(*agui.RunAgentInput) []agui.Event {
	calls := 0
	return func(in *agui.RunAgentInput) []agui.Event {
		calls++
		if calls == 1 {
			return []agui.Event{
				&agui.RunStartedEvent{ThreadID: in.ThreadID, RunID: in.RunID},
				&agui.RunFinishedEvent{ThreadID: in.ThreadID, RunID: in.RunID, Outcome: &agui.InterruptOutcome{Interrupts: []agui.Interrupt{{ID: id, Reason: "tool_approval"}}}},
			}
		}
		return echo()(in)
	}
}

// C#: tests/AGUI.Client.UnitTests/AGUIChatClientTest.cs::AGUIChatClientTest.GetStreamingResponse_RawRepresentationFactory_ForwardsResume
//
// The Go client enforces the consumer's coverage rule
// (interrupt-resume.mdx), so a resume entry only travels for an interrupt the
// last run raised; the first run here raises "caller-interrupt".
func TestCsAGUIChatClientTest_GetStreamingResponse_RawRepresentationFactory_ForwardsResume(t *testing.T) {
	stub := newStub(t, interruptThenEcho("caller-interrupt"))
	agent, _ := newAgent(stub.URL)
	agent.AddMessage(user("u1", "Hello"))
	if res, err := agent.Run(t.Context(), client.RunInput{}, nil); err != nil || res.Status != client.StatusInterrupted {
		t.Fatalf("first run: %v %+v", err, res)
	}
	_, err := agent.Run(t.Context(), client.RunInput{Resume: []agui.ResumeEntry{
		{InterruptID: "caller-interrupt", Status: agui.ResumeStatusResolved},
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	resume := stub.lastInput(t).Resume
	if len(resume) != 1 || resume[0].InterruptID != "caller-interrupt" {
		t.Fatalf("resume = %+v", resume)
	}
}

// C#: tests/AGUI.Client.UnitTests/AGUIChatClientTest.cs::AGUIChatClientTest.GetStreamingResponse_CallerResume_TakesPrecedenceOverApprovalResponses
func TestCsAGUIChatClientTest_GetStreamingResponse_CallerResume_TakesPrecedenceOverApprovalResponses(t *testing.T) {
	t.Skip("N/A in Go: tests precedence between a caller-supplied Resume and the MEAI ToolApprovalResponseContent → resume translation; client.Agent has no approval-content translation — RunInput.Resume is the only source of resume entries")
}

// C#: tests/AGUI.Client.UnitTests/AGUIChatClientTest.cs::AGUIChatClientTest.GetStreamingResponse_CallerResume_TakesPrecedenceOverInterruptResponses
func TestCsAGUIChatClientTest_GetStreamingResponse_CallerResume_TakesPrecedenceOverInterruptResponses(t *testing.T) {
	t.Skip("N/A in Go: tests precedence between a caller-supplied Resume and the MEAI InterruptResponseContent → resume translation; client.Agent has no message-content translation — RunInput.Resume is the only source of resume entries")
}

// C#: tests/AGUI.Client.UnitTests/AGUIChatClientTest.cs::AGUIChatClientTest.GetStreamingResponse_InterruptResponseMetadata_ReachesTheResumeEntry
func TestCsAGUIChatClientTest_GetStreamingResponse_InterruptResponseMetadata_ReachesTheResumeEntry(t *testing.T) {
	stub := newStub(t, interruptThenEcho("req-interrupt"))
	agent, _ := newAgent(stub.URL)
	agent.AddMessage(user("u1", "Hello"))
	if _, err := agent.Run(t.Context(), client.RunInput{}, nil); err != nil {
		t.Fatal(err)
	}
	_, err := agent.Run(t.Context(), client.RunInput{Resume: []agui.ResumeEntry{{
		InterruptID: "req-interrupt",
		Status:      agui.ResumeStatusResolved,
		Payload:     map[string]any{"approved": true},
		Metadata:    agui.Metadata{"definitionId": "review-plan", "key": "afterModel-review"},
	}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	body := stub.lastBody(t)
	entries := mustAt(t, body, "resume").([]any)
	if len(entries) != 1 {
		t.Fatalf("resume = %v", entries)
	}
	if got := mustAt(t, entries[0], "interruptId"); got != "req-interrupt" {
		t.Errorf("interruptId = %v", got)
	}
	if got := mustAt(t, entries[0], "payload.approved"); got != true {
		t.Errorf("payload.approved = %v", got)
	}
	if got := mustAt(t, entries[0], "metadata.definitionId"); got != "review-plan" {
		t.Errorf("metadata.definitionId = %v", got)
	}
	if got := mustAt(t, entries[0], "metadata.key"); got != "afterModel-review" {
		t.Errorf("metadata.key = %v", got)
	}
}

// C#: tests/AGUI.Client.UnitTests/AGUIChatClientTest.cs::AGUIChatClientTest.GetStreamingResponse_InterruptResponseWithoutMetadata_OmitsIt
func TestCsAGUIChatClientTest_GetStreamingResponse_InterruptResponseWithoutMetadata_OmitsIt(t *testing.T) {
	stub := newStub(t, interruptThenEcho("req-interrupt"))
	agent, _ := newAgent(stub.URL)
	agent.AddMessage(user("u1", "Hello"))
	if _, err := agent.Run(t.Context(), client.RunInput{}, nil); err != nil {
		t.Fatal(err)
	}
	_, err := agent.Run(t.Context(), client.RunInput{Resume: []agui.ResumeEntry{
		{InterruptID: "req-interrupt", Status: agui.ResumeStatusResolved},
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	entries := mustAt(t, stub.lastBody(t), "resume").([]any)
	if len(entries) != 1 {
		t.Fatalf("resume = %v", entries)
	}
	if v, ok := at(entries[0], "metadata"); ok {
		t.Fatalf("metadata must be absent, got %v", v)
	}
}

// toolResultRun runs a stream carrying one plain-text TOOL_CALL_RESULT.
func toolResultRun(t *testing.T, content string) *client.Agent {
	t.Helper()
	stub := newStub(t, static(
		&agui.RunStartedEvent{ThreadID: "thread-1", RunID: "run-1"},
		&agui.ToolCallResultEvent{MessageID: "msg-1", ToolCallID: "call-1", Content: agui.TextContent(content), Role: agui.Ptr(agui.RoleTool)},
		&agui.RunFinishedEvent{ThreadID: "thread-1", RunID: "run-1"},
	))
	agent, _ := newAgent(stub.URL)
	agent.ThreadID = "thread-1"
	agent.AddMessage(user("u1", "start"))
	res, err := agent.Run(t.Context(), client.RunInput{RunID: "run-1"}, nil)
	if err != nil {
		t.Fatalf("plain-text tool result rejected: %v", err)
	}
	if !res.Succeeded() {
		t.Fatalf("status %s", res.Status)
	}
	return agent
}

func singleToolMessage(t *testing.T, a *client.Agent) *agui.ToolMessage {
	t.Helper()
	var found []*agui.ToolMessage
	for _, m := range a.Store.Messages {
		if tm, ok := m.(*agui.ToolMessage); ok {
			found = append(found, tm)
		}
	}
	if len(found) != 1 {
		t.Fatalf("found %d tool messages", len(found))
	}
	return found[0]
}

// C#: tests/AGUI.Client.UnitTests/AGUIChatClientTest.cs::AGUIChatClientTest.AGUIChatClient_ToolCallResultWithPlainTextContent_DoesNotParseAsJson
func TestCsAGUIChatClientTest_AGUIChatClient_ToolCallResultWithPlainTextContent_DoesNotParseAsJson(t *testing.T) {
	tm := singleToolMessage(t, toolResultRun(t, "Transferred."))
	if tm.ToolCallID != "call-1" {
		t.Errorf("toolCallId = %q", tm.ToolCallID)
	}
	if tm.Content.IsParts() || tm.Content.String() != "Transferred." {
		t.Errorf("content = %#v", tm.Content)
	}
}

// C#: tests/AGUI.Client.UnitTests/AGUIChatClientTest.cs::AGUIChatClientTest.AGUIChatClient_WorkflowToolCallResultWithPlainTextContent_DoesNotThrowJsonException
func TestCsAGUIChatClientTest_AGUIChatClient_WorkflowToolCallResultWithPlainTextContent_DoesNotThrowJsonException(t *testing.T) {
	tm := singleToolMessage(t, toolResultRun(t, "Expense report ER-1 approved"))
	if tm.Content.String() != "Expense report ER-1 approved" {
		t.Errorf("content = %q", tm.Content.String())
	}
}

// C#: tests/AGUI.Client.UnitTests/AGUIChatClientTest.cs::AGUIChatClientTest.ClientToolExecution_EmitsExecuteToolSpan_OnAGUIClientSource
func TestCsAGUIChatClientTest_ClientToolExecution_EmitsExecuteToolSpan_OnAGUIClientSource(t *testing.T) {
	t.Skip("N/A in Go: asserts a System.Diagnostics ActivitySource span (\"execute_tool probe_location\") emitted when the MEAI function-invocation layer runs an AIFunction client tool; client.Agent neither executes tools (it reports them in RunResult.PendingToolCallIDs) nor has tracing instrumentation")
}

func i64(v int64) *int64 { return &v }

// C#: tests/AGUI.Client.UnitTests/AGUIChatClientTest.cs::AGUIChatClientTest.GetStreamingResponse_SurfacesRunFinishedUsageAsUsageContent
//
// .NET maps RUN_FINISHED.usage onto MEAI UsageContent; Go surfaces the
// decoded usage on RunResult.Finished. Every count must survive.
func TestCsAGUIChatClientTest_GetStreamingResponse_SurfacesRunFinishedUsageAsUsageContent(t *testing.T) {
	events := helloEvents()
	events[4] = &agui.RunFinishedEvent{ThreadID: "t1", RunID: "r1", Usage: []agui.TokenUsage{{
		Provider: agui.Ptr("openai"), Model: agui.Ptr("gpt-4o"),
		InputTokens: i64(11), OutputTokens: i64(22), TotalTokens: i64(33),
		ReasoningTokens: i64(44), CachedInputTokens: i64(55), CacheWriteInputTokens: i64(66),
	}}}
	stub := newStub(t, static(events...))
	agent, _ := newAgent(stub.URL)
	agent.ThreadID = "t1"
	agent.AddMessage(user("u1", "hi"))
	res, err := agent.Run(t.Context(), client.RunInput{RunID: "r1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Finished == nil || len(res.Finished.Usage) != 1 {
		t.Fatalf("usage not surfaced: %+v", res.Finished)
	}
	u := res.Finished.Usage[0]
	for name, pair := range map[string][2]any{
		"provider":              {agui.Deref(u.Provider), "openai"},
		"model":                 {agui.Deref(u.Model), "gpt-4o"},
		"inputTokens":           {agui.Deref(u.InputTokens), int64(11)},
		"outputTokens":          {agui.Deref(u.OutputTokens), int64(22)},
		"totalTokens":           {agui.Deref(u.TotalTokens), int64(33)},
		"reasoningTokens":       {agui.Deref(u.ReasoningTokens), int64(44)},
		"cachedInputTokens":     {agui.Deref(u.CachedInputTokens), int64(55)},
		"cacheWriteInputTokens": {agui.Deref(u.CacheWriteInputTokens), int64(66)},
	} {
		if pair[0] != pair[1] {
			t.Errorf("%s = %v, want %v", name, pair[0], pair[1])
		}
	}
}

// C#: tests/AGUI.Client.UnitTests/AGUIChatClientTest.cs::AGUIChatClientTest.GetStreamingResponse_SurfacesRunErrorUsageAsUsageContent
func TestCsAGUIChatClientTest_GetStreamingResponse_SurfacesRunErrorUsageAsUsageContent(t *testing.T) {
	stub := newStub(t, static(
		&agui.RunStartedEvent{ThreadID: "t1", RunID: "r1"},
		&agui.RunErrorEvent{Message: "failed", Code: agui.Ptr("ERR"), Usage: []agui.TokenUsage{{
			Provider: agui.Ptr("openai"), Model: agui.Ptr("gpt-4o"),
			InputTokens: i64(11), OutputTokens: i64(22), TotalTokens: i64(33),
		}}},
	))
	agent, _ := newAgent(stub.URL)
	agent.ThreadID = "t1"
	agent.AddMessage(user("u1", "hi"))
	res, err := agent.Run(t.Context(), client.RunInput{RunID: "r1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := types(res.Events); !eqStrings(got, []string{"RUN_STARTED", "RUN_ERROR"}) {
		t.Fatalf("delivered %v", got)
	}
	if res.Status != client.StatusFailed || res.Error == nil {
		t.Fatalf("status %s error %+v", res.Status, res.Error)
	}
	if res.Error.Message != "failed" || agui.Deref(res.Error.Code) != "ERR" {
		t.Errorf("error = %+v", res.Error)
	}
	if res.Error != res.Events[1] {
		t.Error("RunResult.Error is not the delivered RUN_ERROR")
	}
	if len(res.Error.Usage) != 1 {
		t.Fatalf("usage = %+v", res.Error.Usage)
	}
	u := res.Error.Usage[0]
	if agui.Deref(u.Model) != "gpt-4o" || agui.Deref(u.InputTokens) != 11 || agui.Deref(u.OutputTokens) != 22 || agui.Deref(u.TotalTokens) != 33 {
		t.Errorf("usage = %+v", u)
	}
}

// C#: tests/AGUI.Client.UnitTests/AGUIChatClientTest.cs::AGUIChatClientTest.GetStreamingResponse_UsageContentCarriesModelId
//
// Per-model attribution survives: one entry per model, in order.
func TestCsAGUIChatClientTest_GetStreamingResponse_UsageContentCarriesModelId(t *testing.T) {
	stub := newStub(t, static(
		&agui.RunStartedEvent{ThreadID: "t1", RunID: "r1"},
		&agui.RunFinishedEvent{ThreadID: "t1", RunID: "r1", Usage: []agui.TokenUsage{
			{Model: agui.Ptr("gpt-4o"), InputTokens: i64(10)},
			{Model: agui.Ptr("gpt-4o-mini"), InputTokens: i64(5)},
		}},
	))
	agent, _ := newAgent(stub.URL)
	agent.ThreadID = "t1"
	res, err := agent.Run(t.Context(), client.RunInput{RunID: "r1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	u := res.Finished.Usage
	if len(u) != 2 {
		t.Fatalf("usage = %+v", u)
	}
	if agui.Deref(u[0].Model) != "gpt-4o" || agui.Deref(u[0].InputTokens) != 10 {
		t.Errorf("usage[0] = %+v", u[0])
	}
	if agui.Deref(u[1].Model) != "gpt-4o-mini" || agui.Deref(u[1].InputTokens) != 5 {
		t.Errorf("usage[1] = %+v", u[1])
	}
}

// C#: tests/AGUI.Client.UnitTests/AGUIChatClientTest.cs::AGUIChatClientTest.GetStreamingResponse_NoUsage_EmitsNoUsageContent
func TestCsAGUIChatClientTest_GetStreamingResponse_NoUsage_EmitsNoUsageContent(t *testing.T) {
	stub := newStub(t, static(
		&agui.RunStartedEvent{ThreadID: "t1", RunID: "r1"},
		&agui.RunFinishedEvent{ThreadID: "t1", RunID: "r1"},
	))
	agent, _ := newAgent(stub.URL)
	agent.ThreadID = "t1"
	res, err := agent.Run(t.Context(), client.RunInput{RunID: "r1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Finished == nil {
		t.Fatal("no RUN_FINISHED")
	}
	if res.Finished.Usage != nil {
		t.Errorf("usage invented: %+v", res.Finished.Usage)
	}
	for _, e := range res.Events {
		g, _ := agui.ToGeneric(e)
		if _, ok := at(g, "usage"); ok {
			t.Errorf("%s carries usage: %s", e.Type(), jsonString(g))
		}
	}
}
