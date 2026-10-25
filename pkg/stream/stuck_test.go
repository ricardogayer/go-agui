package stream

import (
	"io"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

func TestMixedFormsCannotWedgeTheStream(t *testing.T) {
	s := NewWriter(io.Discard)
	_ = s.RunStarted("t", "r")
	_ = s.SubagentStarted("s", "worker")
	if err := s.Send(&agui.TextMessageChunkEvent{Attribution: agui.Attribution{SubagentRunID: agui.Ptr("s")}, MessageID: agui.Ptr("m"), Delta: agui.Ptr("x")}); err != nil {
		t.Fatal(err)
	}
	if err := s.TextEnd("m"); err == nil {
		t.Error("an explicit END for a chunk-opened message mixes the two spellings and must be refused")
	}
	if err := s.RunError("boom", nil); err != nil {
		t.Fatalf("RUN_ERROR must always be sendable in an open run: %v", err)
	}
}

func TestProducerRules(t *testing.T) {
	sub := agui.Attribution{SubagentRunID: agui.Ptr("sub")}
	cases := []struct {
		name  string
		setup func(s *SSE)
		bad   func(s *SSE) error
	}{
		{"finished run must match the started one", nil,
			func(s *SSE) error { return s.RunFinished("t2", "r2") }},
		{"runId must not be reused", func(s *SSE) { _ = s.RunFinished("t", "r") },
			func(s *SSE) error { return s.RunStarted("t", "r") }},
		{"subagent announced after use", func(s *SSE) {
			_ = s.Send(&agui.TextMessageStartEvent{Attribution: sub, MessageID: "m"})
		}, func(s *SSE) error { return s.SubagentStarted("sub", "w") }},
		{"subagent used after it finished", func(s *SSE) {
			_ = s.SubagentStarted("sub", "w")
			_ = s.SubagentFinished("sub")
		}, func(s *SSE) error { return s.Send(&agui.StepStartedEvent{Attribution: sub, StepName: "x"}) }},
		{"reopened message with another role", func(s *SSE) {
			_ = s.TextStart("m")
			_ = s.TextEnd("m")
		}, func(s *SSE) error {
			return s.Send(&agui.TextMessageStartEvent{MessageID: "m", Role: agui.Ptr(agui.TextMessageRoleUser)})
		}},
		{"reopened tool call with another name", func(s *SSE) { _ = s.ToolCall("c", "a", "{}") },
			func(s *SSE) error { return s.ToolCallStart("c", "b") }},
		{"duplicate interrupt ids", nil, func(s *SSE) error {
			return s.RunInterrupted("t", "r", agui.Interrupt{ID: "i", Reason: "x"}, agui.Interrupt{ID: "i", Reason: "y"})
		}},
		{"pending call never made", nil,
			func(s *SSE) error { return s.RunFinishedPending("t", "r", "nope") }},
		{"pending list missing a call", func(s *SSE) {
			_ = s.ToolCall("c1", "f", "{}")
			_ = s.ToolCall("c2", "f", "{}")
		}, func(s *SSE) error { return s.RunFinishedPending("t", "r", "c2") }},
		{"activity delta without snapshot", nil,
			func(s *SSE) error { return s.ActivityDelta("a", "x", agui.Replace("/n", 1)) }},
	}
	for _, c := range cases {
		s := NewWriter(io.Discard)
		_ = s.RunStarted("t", "r")
		if c.setup != nil {
			c.setup(s)
		}
		if err := c.bad(s); err == nil {
			t.Errorf("%s: accepted", c.name)
		}
	}

	// The conforming counterparts are accepted.
	s := NewWriter(io.Discard)
	steps := []func() error{
		func() error { return s.RunStarted("t", "r") },
		func() error { return s.ToolCall("c1", "f", "{}") },
		func() error { return s.ToolCall("c2", "f", "{}") },
		func() error { return s.ToolCallResult("res", "c1", "ok") },
		func() error { return s.TextStart("m") },
		func() error { return s.TextEnd("m") },
		func() error { return s.TextStart("m") }, // reopening with the same role
		func() error { return s.TextEnd("m") },
		func() error { return s.RunFinishedPending("t", "r", "c2") },
		func() error { return s.RunStarted("t", "r2") },
		func() error {
			return s.RunInterrupted("t", "r2", agui.Interrupt{ID: "i1", Reason: "x"}, agui.Interrupt{ID: "i2", Reason: "x"})
		},
	}
	for i, step := range steps {
		if err := step(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}
}
