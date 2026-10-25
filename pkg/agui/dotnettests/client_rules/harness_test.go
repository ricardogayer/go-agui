package client_rules_test

// Port of the official AG-UI .NET SDK tests
// tests/AGUI.Client.UnitTests/ProtocolRuleTest.cs.
//
// The C# helper ProcessEventsAsync serialises each BaseEvent as one SSE frame,
// serves the stream through a mock HttpClient and runs it through
// EventStreamConverter.AsChatResponseUpdates, collecting the
// Microsoft.Extensions.AI ChatResponseUpdate objects; a protocol violation
// surfaces as InvalidOperationException.
//
// The Go analogue (feed below) frames the same JSON events as SSE and runs
// them through client.Agent.Consume — the very pipeline Agent.Run uses on an
// HTTP response body: compatibility boundary → middleware → enforcement →
// chunk expansion → verification → Store. A violation surfaces as a non-nil
// error (*agui.ProtocolError for sequencing, *agui.ValidationError for a
// malformed known value). Assertions on ChatResponseUpdate objects are
// translated as follows:
//
//   - u.RawRepresentation is XEvent      → an XEvent among the delivered events
//   - u.Text (text updates)              → TEXT_MESSAGE_CONTENT deltas, plus the
//     assembled message in Store.Messages
//   - FunctionCallContent                → the ToolCall on the assistant message
//     in Store.Messages (name, id, parsed arguments)
//   - FunctionResultContent              → the tool message in Store.Messages
//   - ErrorContent                       → RunResult.Status == failed and
//     RunResult.Error (message, code)
//   - Owner(u) ("agui.subagentRunId")    → subagentRunId on the delivered event
//     and/or on the message the Store assembled
//   - ToChatResponse().AsAGUIMessages()  → the messages Agent.BuildInput sends on
//     the next turn (the Go round trip)

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
)

// outcome is what one stream did to a fresh client.
type outcome struct {
	agent    *client.Agent
	res      *client.RunResult
	err      error
	events   []agui.Event
	warnings []agui.Warning
}

// feed runs the JSON events, one SSE frame each, through a fresh client.
func feed(t *testing.T, evs ...string) *outcome {
	t.Helper()
	var sb strings.Builder
	for _, e := range evs {
		sb.WriteString("data: ")
		sb.WriteString(e)
		sb.WriteString("\n\n")
	}
	o := &outcome{agent: client.New("http://localhost/agent")}
	o.agent.ThreadID = "t1"
	o.agent.Warn = func(w agui.Warning) { o.warnings = append(o.warnings, w) }
	o.res, o.err = o.agent.Consume(strings.NewReader(sb.String()), func(e agui.Event) {
		o.events = append(o.events, e)
	})
	return o
}

// raw marks a JSON literal to embed as is.
type raw = json.RawMessage

// ev builds one event as JSON: ev("TEXT_MESSAGE_START", "messageId", "m1").
func ev(typ string, kv ...any) string {
	m := map[string]any{"type": typ}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i].(string)] = kv[i+1]
	}
	b, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func runStarted(runID string, kv ...any) string {
	return ev("RUN_STARTED", append([]any{"threadId", "t1", "runId", runID}, kv...)...)
}
func runFinished(runID string, kv ...any) string {
	return ev("RUN_FINISHED", append([]any{"threadId", "t1", "runId", runID}, kv...)...)
}
func textStart(id string, kv ...any) string {
	return ev("TEXT_MESSAGE_START", append([]any{"messageId", id, "role", "assistant"}, kv...)...)
}
func textContent(id, delta string, kv ...any) string {
	return ev("TEXT_MESSAGE_CONTENT", append([]any{"messageId", id, "delta", delta}, kv...)...)
}
func textEnd(id string, kv ...any) string {
	return ev("TEXT_MESSAGE_END", append([]any{"messageId", id}, kv...)...)
}
func toolStart(id, name string, kv ...any) string {
	return ev("TOOL_CALL_START", append([]any{"toolCallId", id, "toolCallName", name}, kv...)...)
}
func toolArgs(id, delta string, kv ...any) string {
	return ev("TOOL_CALL_ARGS", append([]any{"toolCallId", id, "delta", delta}, kv...)...)
}
func toolEnd(id string, kv ...any) string {
	return ev("TOOL_CALL_END", append([]any{"toolCallId", id}, kv...)...)
}
func toolResult(msgID, callID, content string, kv ...any) string {
	return ev("TOOL_CALL_RESULT", append([]any{"messageId", msgID, "toolCallId", callID, "content", content}, kv...)...)
}
func stepStarted(name string, kv ...any) string {
	return ev("STEP_STARTED", append([]any{"stepName", name}, kv...)...)
}
func stepFinished(name string, kv ...any) string {
	return ev("STEP_FINISHED", append([]any{"stepName", name}, kv...)...)
}
func subStarted(id, name string, kv ...any) string {
	return ev("SUBAGENT_STARTED", append([]any{"subagentRunId", id, "name", name}, kv...)...)
}
func subFinished(id string, kv ...any) string {
	return ev("SUBAGENT_FINISHED", append([]any{"subagentRunId", id}, kv...)...)
}
func runError(msg string, kv ...any) string {
	return ev("RUN_ERROR", append([]any{"message", msg}, kv...)...)
}
func reasoningChunk(kv ...any) string { return ev("REASONING_MESSAGE_CHUNK", kv...) }
func encrypted(subtype, entity, value string, kv ...any) string {
	return ev("REASONING_ENCRYPTED_VALUE", append([]any{"subtype", subtype, "entityId", entity, "encryptedValue", value}, kv...)...)
}

// ---------------------------------------------------------------------------
// Assertions
// ---------------------------------------------------------------------------

func (o *outcome) accepted(t *testing.T) {
	t.Helper()
	if o.err != nil {
		t.Fatalf("stream rejected: %v", o.err)
	}
}

// rejected asserts a protocol rejection whose message names every fragment.
func (o *outcome) rejected(t *testing.T, fragments ...string) {
	t.Helper()
	if o.err == nil {
		t.Fatalf("stream accepted, expected a rejection naming %q", fragments)
	}
	var pe *agui.ProtocolError
	var ve *agui.ValidationError
	if !errors.As(o.err, &pe) && !errors.As(o.err, &ve) {
		t.Fatalf("rejection is neither a protocol nor a validation error: %T %v", o.err, o.err)
	}
	for _, f := range fragments {
		if !strings.Contains(o.err.Error(), f) {
			t.Errorf("rejection %q does not contain %q", o.err, f)
		}
	}
}

// protocolRejected asserts a sequencing rejection (*agui.ProtocolError).
func (o *outcome) protocolRejected(t *testing.T, fragments ...string) {
	t.Helper()
	o.rejected(t, fragments...)
	var pe *agui.ProtocolError
	if !errors.As(o.err, &pe) {
		t.Errorf("want a *agui.ProtocolError, got %T", o.err)
	}
}

// validationRejected asserts a malformed-known-value rejection at a field.
func (o *outcome) validationRejected(t *testing.T, field string) {
	t.Helper()
	o.rejected(t)
	var ve *agui.ValidationError
	if !errors.As(o.err, &ve) {
		t.Fatalf("want a *agui.ValidationError, got %T %v", o.err, o.err)
	}
	if !strings.Contains(ve.Error(), field) {
		t.Errorf("validation error %q does not name %q", ve, field)
	}
}

// texts returns the non-empty TEXT_MESSAGE_CONTENT deltas delivered, in order.
func (o *outcome) texts() []string {
	var out []string
	for _, e := range o.events {
		if c, ok := e.(*agui.TextMessageContentEvent); ok && c.Delta != "" {
			out = append(out, c.Delta)
		}
	}
	return out
}

// count returns how many delivered events have type et.
func (o *outcome) count(et agui.EventType) int {
	n := 0
	for _, e := range o.events {
		if e.Type() == et {
			n++
		}
	}
	return n
}

// of returns the delivered events of type et.
func (o *outcome) of(et agui.EventType) []agui.Event {
	var out []agui.Event
	for _, e := range o.events {
		if e.Type() == et {
			out = append(out, e)
		}
	}
	return out
}

// calls returns every tool call in the Store, in message order.
func (o *outcome) calls() []agui.ToolCall {
	var out []agui.ToolCall
	for _, m := range o.agent.Store.Messages {
		if a, ok := m.(*agui.AssistantMessage); ok {
			out = append(out, a.ToolCalls...)
		}
	}
	return out
}

// holder returns the assistant message holding a tool call.
func (o *outcome) holder(t *testing.T, callID string) *agui.AssistantMessage {
	t.Helper()
	for _, m := range o.agent.Store.Messages {
		if a, ok := m.(*agui.AssistantMessage); ok {
			for _, tc := range a.ToolCalls {
				if tc.ID == callID {
					return a
				}
			}
		}
	}
	t.Fatalf("no assistant message holds tool call %q", callID)
	return nil
}

// message returns the first Store message with id.
func (o *outcome) message(t *testing.T, id string) agui.Message {
	t.Helper()
	for _, m := range o.agent.Store.Messages {
		if m.MessageID() == id {
			return m
		}
	}
	t.Fatalf("no message %q in the store", id)
	return nil
}

// toolMessage returns the tool message answering a call.
func (o *outcome) toolMessage(t *testing.T, callID string) *agui.ToolMessage {
	t.Helper()
	for _, m := range o.agent.Store.Messages {
		if tm, ok := m.(*agui.ToolMessage); ok && tm.ToolCallID == callID {
			return tm
		}
	}
	t.Fatalf("no tool message answers %q", callID)
	return nil
}

// nextTurn returns the messages the next run's input would carry, as JSON
// objects — the Go equivalent of ToChatResponse().AsAGUIMessages().
func (o *outcome) nextTurn(t *testing.T, resume ...agui.ResumeEntry) []map[string]any {
	t.Helper()
	_, body, err := o.agent.BuildInput(client.RunInput{RunID: "next", Resume: resume})
	if err != nil {
		t.Fatalf("BuildInput: %v", err)
	}
	var in struct {
		Messages []map[string]any `json:"messages"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		t.Fatal(err)
	}
	return in.Messages
}

func args(t *testing.T, tc agui.ToolCall) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &m); err != nil {
		t.Fatalf("tool call %s arguments %q: %v", tc.ID, tc.Function.Arguments, err)
	}
	return m
}

func eqStrings(t *testing.T, what string, got, want []string) {
	t.Helper()
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") || len(got) != len(want) {
		t.Errorf("%s: got %q, want %q", what, got, want)
	}
}

func eq[T comparable](t *testing.T, what string, got, want T) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %v, want %v", what, got, want)
	}
}

// own renders an optional subagentRunId ("<parent>" when absent).
func own(p *string) string {
	if p == nil {
		return "<parent>"
	}
	return *p
}

// ownerOf returns the subagentRunId an event carries ("<parent>" when none).
func ownerOf(e agui.Event) string {
	b, _ := json.Marshal(e)
	var m struct {
		SubagentRunID *string `json:"subagentRunId"`
	}
	_ = json.Unmarshal(b, &m)
	return own(m.SubagentRunID)
}

func msgOwner(m agui.Message) string {
	b, _ := json.Marshal(m)
	var x struct {
		SubagentRunID *string `json:"subagentRunId"`
	}
	_ = json.Unmarshal(b, &x)
	return own(x.SubagentRunID)
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
