package agui

import (
	"encoding/json"
	"strings"
	"testing"
)

func mustGeneric(t *testing.T, s string) any {
	t.Helper()
	v, err := decodeGeneric([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestApplyPatch(t *testing.T) {
	cases := []struct {
		doc, patch, want string
		fails            bool
	}{
		{`{"a":1}`, `[{"op":"add","path":"/b","value":2}]`, `{"a":1,"b":2}`, false},
		{`{"a":[1,3]}`, `[{"op":"add","path":"/a/1","value":2}]`, `{"a":[1,2,3]}`, false},
		{`{"a":[1]}`, `[{"op":"add","path":"/a/-","value":2}]`, `{"a":[1,2]}`, false},
		{`{"a":1,"b":2}`, `[{"op":"remove","path":"/a"}]`, `{"b":2}`, false},
		{`{"a":1}`, `[{"op":"replace","path":"/a","value":null}]`, `{"a":null}`, false},
		{`{"a":{"x":1}}`, `[{"op":"move","from":"/a/x","path":"/y"}]`, `{"a":{},"y":1}`, false},
		{`{"a":[1]}`, `[{"op":"copy","from":"/a","path":"/b"}]`, `{"a":[1],"b":[1]}`, false},
		{`{"a~b":{"c/d":1}}`, `[{"op":"replace","path":"/a~0b/c~1d","value":2}]`, `{"a~b":{"c/d":2}}`, false},
		{`{"a":1}`, `[{"op":"replace","path":"","value":[1]}]`, `[1]`, false},
		{`{"a":1}`, `[{"op":"test","path":"/a","value":1},{"op":"add","path":"/t","value":true}]`, `{"a":1,"t":true}`, false},
		// Atomicity: the first op applies, the second fails, nothing survives.
		{`{"a":1}`, `[{"op":"add","path":"/b","value":2},{"op":"remove","path":"/missing"}]`, ``, true},
		{`{"a":1}`, `[{"op":"test","path":"/a","value":2}]`, ``, true},
		{`{"a":[1]}`, `[{"op":"add","path":"/a/5","value":2}]`, ``, true},
		{`{"a":{}}`, `[{"op":"move","from":"/a","path":"/a/b"}]`, ``, true},
	}
	for _, c := range cases {
		doc := mustGeneric(t, c.doc)
		before, _ := json.Marshal(doc)
		var patch JSONPatch
		if err := json.Unmarshal([]byte(c.patch), &patch); err != nil {
			t.Fatal(err)
		}
		got, err := ApplyPatch(doc, patch)
		after, _ := json.Marshal(doc)
		if string(before) != string(after) {
			t.Errorf("%s: input document was mutated", c.patch)
		}
		if c.fails {
			if err == nil {
				t.Errorf("%s: expected failure, got %v", c.patch, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", c.patch, err)
			continue
		}
		if !jsonEqual(normalizeGeneric(got), mustGeneric(t, c.want)) {
			b, _ := json.Marshal(got)
			t.Errorf("%s: got %s want %s", c.patch, b, c.want)
		}
	}
}

func TestPatchOperationKeepsExtensions(t *testing.T) {
	var op PatchOperation
	if err := json.Unmarshal([]byte(`{"op":"remove","path":"/a","value":"leftover","x-trace":1}`), &op); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(op)
	if !jsonEqual(mustGeneric(t, string(b)), mustGeneric(t, `{"op":"remove","path":"/a","value":"leftover","x-trace":1}`)) {
		t.Fatalf("extensions lost: %s", b)
	}
}

func runChecker(events ...Event) error {
	var c StreamChecker
	for _, e := range events {
		if _, err := c.Check(e); err != nil {
			return err
		}
	}
	return nil
}

func TestVerifierRules(t *testing.T) {
	sub := Ptr("s1")
	cases := []struct {
		name    string
		events  []Event
		wantErr string
	}{
		{"conformant", []Event{
			NewRunStartedEvent("t", "r"),
			NewStepStartedEvent("plan"),
			NewTextMessageStartEvent("m"), NewTextMessageContentEvent("m", "hi"), NewTextMessageEndEvent("m"),
			NewToolCallStartEvent("c", "search"), NewToolCallArgsEvent("c", "{}"), NewToolCallEndEvent("c"),
			NewToolCallResultEvent("m2", "c", "ok"),
			NewReasoningStartEvent("r1"), NewReasoningMessageStartEvent("r1"), NewReasoningMessageEndEvent("r1"), NewReasoningEndEvent("r1"),
			NewStepFinishedEvent("plan"),
			NewRunFinishedEvent("t", "r"),
			NewRunStartedEvent("t", "r2"),
			NewRunFinishedEvent("t", "r2"),
			NewRunErrorEvent("late failure"),
		}, ""},
		{"first event", []Event{NewTextMessageStartEvent("m")}, "First event must be 'RUN_STARTED'"},
		{"run error first", []Event{NewRunErrorEvent("x"), NewRunStartedEvent("t", "r"), NewRunFinishedEvent("t", "r")}, ""},
		{"nested run", []Event{NewRunStartedEvent("t", "r"), NewRunStartedEvent("t", "r2")}, "while a run is still active"},
		{"after finish", []Event{NewRunStartedEvent("t", "r"), NewRunFinishedEvent("t", "r"), NewStepStartedEvent("x")}, "already finished"},
		{"after error", []Event{NewRunStartedEvent("t", "r"), NewRunErrorEvent("x"), NewRunErrorEvent("y")}, "already errored"},
		{"open message", []Event{NewRunStartedEvent("t", "r"), NewTextMessageStartEvent("m"), NewRunFinishedEvent("t", "r")}, "text messages are still active"},
		{"orphan content", []Event{NewRunStartedEvent("t", "r"), NewTextMessageContentEvent("m", "x")}, "No active text message found with ID 'm'"},
		{"reopen open", []Event{NewRunStartedEvent("t", "r"), NewTextMessageStartEvent("m"), NewTextMessageStartEvent("m")}, "already in progress"},
		{"reopen closed ok", []Event{NewRunStartedEvent("t", "r"), NewTextMessageStartEvent("m"), NewTextMessageEndEvent("m"), NewTextMessageStartEvent("m"), NewTextMessageEndEvent("m"), NewRunFinishedEvent("t", "r")}, ""},
		{"step unbalanced", []Event{NewRunStartedEvent("t", "r"), NewStepFinishedEvent("x")}, "that was not started"},
		{"open step", []Event{NewRunStartedEvent("t", "r"), NewStepStartedEvent("x"), NewRunFinishedEvent("t", "r")}, "steps are still active"},
		{"open tool", []Event{NewRunStartedEvent("t", "r"), NewToolCallStartEvent("c", "f"), NewRunFinishedEvent("t", "r")}, "tool calls are still active"},
		{"open span", []Event{NewRunStartedEvent("t", "r"), NewReasoningStartEvent("s"), NewRunFinishedEvent("t", "r")}, "reasoning spans are still active"},
		{"span is not message", []Event{NewRunStartedEvent("t", "r"), NewReasoningStartEvent("s"), NewReasoningMessageContentEvent("s", "x")}, "No active reasoning message found with ID 's'"},
		{"active subagent", []Event{NewRunStartedEvent("t", "r"), NewSubagentStartedEvent("s1", "a"), NewRunFinishedEvent("t", "r")}, "subagents are still active: s1"},
		{"subagent reuse", []Event{NewRunStartedEvent("t", "r"), NewSubagentStartedEvent("s1", "a"), NewSubagentFinishedEvent("s1"), NewSubagentStartedEvent("s1", "a")}, "cannot be reused"},
		{"subagent unstarted", []Event{NewRunStartedEvent("t", "r"), NewSubagentFinishedEvent("s9")}, "no active subagent found with ID 's9'"},
		{"attribution mismatch", []Event{NewRunStartedEvent("t", "r"), NewTextMessageStartEvent("m"),
			&TextMessageContentEvent{Attribution: Attribution{SubagentRunID: sub}, MessageID: "m", Delta: "x"}}, "does not match"},
		{"untagged continuation ok", []Event{NewRunStartedEvent("t", "r"),
			&TextMessageStartEvent{Attribution: Attribution{SubagentRunID: sub}, MessageID: "m"},
			NewTextMessageContentEvent("m", "x"), NewTextMessageEndEvent("m"), NewRunFinishedEvent("t", "r")}, ""},
		{"chunks expand and close", []Event{NewRunStartedEvent("t", "r"), NewTextMessageChunkEvent("m", "a"), NewTextMessageChunkEvent("", "b"),
			NewToolCallChunkEvent("c", "f", "{"), NewToolCallChunkEvent("", "", "}"), NewRunFinishedEvent("t", "r")}, ""},
		{"first chunk without id", []Event{NewRunStartedEvent("t", "r"), NewTextMessageChunkEvent("", "a")}, "First TEXT_MESSAGE_CHUNK must have a messageId"},
		{"tool chunk without name", []Event{NewRunStartedEvent("t", "r"), NewToolCallChunkEvent("c", "", "a")}, "First TOOL_CALL_CHUNK must have a toolCallName"},
		{"custom closes chunk", []Event{NewRunStartedEvent("t", "r"), NewTextMessageChunkEvent("m", "a"), NewCustomEvent("x.y", 1), NewTextMessageChunkEvent("", "b")}, "First TEXT_MESSAGE_CHUNK must have a messageId"},
		{"raw does not close chunk", []Event{NewRunStartedEvent("t", "r"), NewTextMessageChunkEvent("m", "a"), NewRawEvent(1), NewTextMessageChunkEvent("", "b"), NewRunFinishedEvent("t", "r")}, ""},
		{"conflicting role", []Event{NewRunStartedEvent("t", "r"), NewTextMessageChunkEvent("m", "a"),
			&TextMessageChunkEvent{Role: Ptr(TextMessageRoleUser), Delta: Ptr("b")}}, "does not match the open stream's role"},
	}
	for _, c := range cases {
		err := runChecker(c.events...)
		switch {
		case c.wantErr == "" && err != nil:
			t.Errorf("%s: unexpected error %v", c.name, err)
		case c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)):
			t.Errorf("%s: got %v, want error containing %q", c.name, err, c.wantErr)
		}
	}
}

func TestParseRunAgentInput(t *testing.T) {
	body := `{
		"threadId":"t","runId":"r","protocolVersion":"1.0","futureField":true,"forwardedProps":null,
		"messages":[
			{"id":"u","role":"user","content":[
				{"type":"text","text":"look"},
				{"type":"binary","mimeType":"image/png","url":"https://x/y.png","filename":"y.png"},
				{"type":"hologram","uri":"x"}]},
			{"id":"z","role":"narrator","content":"x"}
		]}`
	in, warnings, err := ParseRunAgentInput([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(in.Messages) != 1 {
		t.Fatalf("unrecognised message role must be dropped, have %d messages", len(in.Messages))
	}
	parts := in.Messages[0].(*UserMessage).Content.Parts()
	if len(parts) != 2 {
		t.Fatalf("want text + upgraded image, got %d parts", len(parts))
	}
	img, ok := parts[1].(*ImagePart)
	if !ok || img.Source.(*URLSource).Value != "https://x/y.png" {
		t.Fatalf("binary part not upgraded: %#v", parts[1])
	}
	joined := ""
	for _, w := range warnings {
		joined += w.String() + "\n"
	}
	for _, want := range []string{"/futureField", "/forwardedProps", "/messages/0/content/1", "/messages/1"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing warning for %s in:\n%s", want, joined)
		}
	}

	if _, _, err := ParseRunAgentInput([]byte(`{"threadId":"t","runId":"r","messages":[{"id":"1","role":"user","content":5}]}`)); err == nil {
		t.Error("malformed known value must be rejected")
	}
	if _, _, err := ParseRunAgentInput([]byte(`{"threadId":"t","messages":[]}`)); err == nil {
		t.Error("missing runId must be rejected")
	}
}

func TestCheckAtomicRollsBack(t *testing.T) {
	sub := Ptr("s1")
	// A refused event must not have recorded an owner.
	var c2 StreamChecker
	_, _ = c2.CheckAtomic(NewRunStartedEvent("t", "r"))
	_, _ = c2.CheckAtomic(&TextMessageStartEvent{Attribution: Attribution{SubagentRunID: sub}, MessageID: "p"})
	if _, err := c2.CheckAtomic(&ToolCallStartEvent{Attribution: Attribution{SubagentRunID: Ptr("s2")}, ToolCallID: "c", ToolCallName: "f", ParentMessageID: Ptr("p")}); err == nil {
		t.Fatal("tool call attributed against its parent message must be refused")
	}
	if _, known := c2.Verifier.ownToolCall["c"]; known {
		t.Fatal("refused TOOL_CALL_START left an owner behind")
	}
	if _, err := c2.CheckAtomic(&ToolCallStartEvent{Attribution: Attribution{SubagentRunID: sub}, ToolCallID: "c", ToolCallName: "f", ParentMessageID: Ptr("p")}); err != nil {
		t.Fatalf("a correct retry must be accepted: %v", err)
	}
}

func TestVerifierCloneRollback(t *testing.T) {
	var v Verifier
	if err := v.Check(NewRunStartedEvent("t", "r")); err != nil {
		t.Fatal(err)
	}
	trial := v.trial()
	if err := trial.Check(&TextMessageStartEvent{Attribution: Attribution{SubagentRunID: Ptr("s1")}, MessageID: "m"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := v.ownMessage["m"]; !ok {
		t.Fatal("ownership maps are shared with the clone")
	}
	trial.rollback()
	if _, ok := v.ownMessage["m"]; ok {
		t.Fatal("Rollback left the owner behind")
	}
	if v.messages["m"] {
		t.Fatal("the original's open set must not see the trial's message")
	}
}

func TestPatchRejectsSignedIndexes(t *testing.T) {
	doc := mustGeneric(t, `{"a":["x","y"]}`)
	for _, op := range []PatchOperation{Replace("/a/+1", "z"), Remove("/a/-0"), Add("/a/01", "z")} {
		if _, err := ApplyPatch(doc, JSONPatch{op}); err == nil {
			t.Errorf("%s %s must not apply", op.Op, op.Path)
		}
	}
}
