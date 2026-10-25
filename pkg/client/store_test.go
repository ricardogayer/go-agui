package client

import (
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

func sseOf(events ...string) string {
	var b strings.Builder
	for _, e := range events {
		b.WriteString("data: " + e + "\n\n")
	}
	return b.String()
}

func messageIDs(ms agui.Messages) []string {
	var ids []string
	for _, m := range ms {
		ids = append(ids, string(m.MessageRole())+":"+m.MessageID())
	}
	return ids
}

// A tool call that names no parent message travels in an assistant message
// the store invents. The producer never named that message, so its id must
// yield to any message the producer does name — the tool result, typically,
// whose messageId may well be the tool call's id.
func TestInventedMessageIDsNeverCollide(t *testing.T) {
	for _, tc := range []struct {
		name   string
		prior  agui.Messages
		events []string
		want   string
	}{{
		name: "a tool result whose messageId is the tool call's id",
		events: []string{
			`{"type":"TOOL_CALL_START","toolCallId":"c1","toolCallName":"lookup"}`,
			`{"type":"TOOL_CALL_END","toolCallId":"c1"}`,
			`{"type":"TOOL_CALL_RESULT","messageId":"c1","toolCallId":"c1","content":"ok"}`,
		},
		want: "user:u1 assistant:c1-tool-calls tool:c1",
	}, {
		name: "a text message under the tool call's id",
		events: []string{
			`{"type":"TOOL_CALL_START","toolCallId":"c1","toolCallName":"lookup"}`,
			`{"type":"TOOL_CALL_END","toolCallId":"c1"}`,
			`{"type":"TEXT_MESSAGE_START","messageId":"c1","role":"assistant"}`,
			`{"type":"TEXT_MESSAGE_CONTENT","messageId":"c1","delta":"done"}`,
			`{"type":"TEXT_MESSAGE_END","messageId":"c1"}`,
		},
		want: "user:u1 assistant:c1-tool-calls assistant:c1",
	}, {
		name:  "a tool call id that an earlier message already holds",
		prior: agui.Messages{&agui.UserMessage{ID: "c1", Content: agui.TextContent("hi")}},
		events: []string{
			`{"type":"TOOL_CALL_START","toolCallId":"c1","toolCallName":"lookup"}`,
			`{"type":"TOOL_CALL_END","toolCallId":"c1"}`,
			`{"type":"TOOL_CALL_RESULT","messageId":"r1","toolCallId":"c1","content":"ok"}`,
		},
		want: "user:c1 assistant:c1-2 tool:r1",
	}, {
		name: "a parent-less call with an unrelated result id keeps the tool call's id",
		events: []string{
			`{"type":"TOOL_CALL_START","toolCallId":"c1","toolCallName":"lookup"}`,
			`{"type":"TOOL_CALL_END","toolCallId":"c1"}`,
			`{"type":"TOOL_CALL_RESULT","messageId":"r1","toolCallId":"c1","content":"ok"}`,
		},
		want: "user:u1 assistant:c1 tool:r1",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			agent := New("unused")
			var warnings []string
			agent.Warn = func(w agui.Warning) { warnings = append(warnings, w.String()) }
			if tc.prior != nil {
				agent.Store.Messages = tc.prior
			} else {
				agent.AddMessage(&agui.UserMessage{ID: "u1", Content: agui.TextContent("hello")})
			}
			events := append([]string{`{"type":"RUN_STARTED","threadId":"t","runId":"r"}`}, tc.events...)
			events = append(events, `{"type":"RUN_FINISHED","threadId":"t","runId":"r"}`)
			res, err := agent.Consume(strings.NewReader(sseOf(events...)), nil)
			if err != nil || !res.Succeeded() {
				t.Fatalf("%v %+v", err, res)
			}
			if got := strings.Join(messageIDs(agent.Store.Messages), " "); got != tc.want {
				t.Fatalf("got  %s\nwant %s", got, tc.want)
			}
			if a, call := agent.Store.findToolCall("c1"); a == nil || call.Function.Name != "lookup" {
				t.Fatal("the tool call was lost")
			}
			if len(warnings) > 0 {
				t.Fatalf("a conformant stream must be quiet: %v", warnings)
			}
		})
	}
}
