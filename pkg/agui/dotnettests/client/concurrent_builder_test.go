package client_test

// Port of sdks/dotnet/tests/AGUI.Client.UnitTests/ConcurrentBuilderTest.cs.
//
// Interleaved text messages and tool calls, through the consumer pipeline and
// client.Store (the Go analogue of the .NET TextMessageBuilder and
// ToolCallBuilder; see text_message_builder_test.go and
// tool_call_builder_test.go).

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// C#: tests/AGUI.Client.UnitTests/ConcurrentBuilderTest.cs::ConcurrentBuilderTest.InterleavedToolCallArgs_TrackedIndependently
//
// .NET lists the calls in END order (its flush buffer); the specification
// assigns no order to that, so the Go test looks each call up by id.
func TestCsConcurrentBuilderTest_InterleavedToolCallArgs_TrackedIndependently(t *testing.T) {
	f := started(t, "thread1", "run1")
	f.mustPush(t,
		tcStart("c1", "weather"), tcStart("c2", "search"),
		tcArgs("c1", `{"lo`), tcArgs("c2", `{"qu`),
		tcArgs("c1", `c":"NYC"}`), tcArgs("c2", `ery":"test"}`),
		tcEnd("c2"), tcEnd("c1"))
	if n := len(f.toolCalls()); n != 2 {
		t.Fatalf("%d calls", n)
	}
	_, c1 := f.toolCall(t, "c1")
	if c1.Function.Name != "weather" || parseArgs(t, c1.Function.Arguments)["loc"] != "NYC" {
		t.Errorf("c1 = %+v", c1)
	}
	_, c2 := f.toolCall(t, "c2")
	if c2.Function.Name != "search" || parseArgs(t, c2.Function.Arguments)["query"] != "test" {
		t.Errorf("c2 = %+v", c2)
	}
}

// C#: tests/AGUI.Client.UnitTests/ConcurrentBuilderTest.cs::ConcurrentBuilderTest.ManySimultaneousToolCalls_AllTracked
func TestCsConcurrentBuilderTest_ManySimultaneousToolCalls_AllTracked(t *testing.T) {
	f := started(t, "t", "r")
	for i := 0; i < 10; i++ {
		f.mustPush(t, tcStart(fmt.Sprintf("call-%d", i), fmt.Sprintf("tool_%d", i)))
	}
	for i := 0; i < 10; i++ {
		f.mustPush(t, tcArgs(fmt.Sprintf("call-%d", i), fmt.Sprintf(`{"index":%d}`, i)))
	}
	for i := 9; i >= 0; i-- {
		f.mustPush(t, tcEnd(fmt.Sprintf("call-%d", i)))
	}
	if n := len(f.toolCalls()); n != 10 {
		t.Fatalf("%d calls", n)
	}
	for i := 0; i < 10; i++ {
		_, tc := f.toolCall(t, fmt.Sprintf("call-%d", i))
		if tc.Function.Name != fmt.Sprintf("tool_%d", i) || jsonString(parseArgs(t, tc.Function.Arguments)["index"]) != fmt.Sprint(i) {
			t.Errorf("call-%d = %+v", i, tc)
		}
	}
}

// C#: tests/AGUI.Client.UnitTests/ConcurrentBuilderTest.cs::ConcurrentBuilderTest.ConcurrentToolCalls_OneEmptyOneComplex
func TestCsConcurrentBuilderTest_ConcurrentToolCalls_OneEmptyOneComplex(t *testing.T) {
	f := started(t, "t", "r")
	f.mustPush(t,
		tcStart("c1", "no_args_tool"), tcStart("c2", "complex_tool"),
		tcArgs("c2", `{"nested":{"a":1,"b":[1,2,3]},"flag":true}`),
		tcEnd("c1"), tcEnd("c2"))
	calls := f.toolCalls()
	if len(calls) != 2 {
		t.Fatalf("calls = %+v", calls)
	}
	// .NET: null Arguments; AG-UI: the empty argument text (arguments is a required string).
	if _, c1 := f.toolCall(t, "c1"); c1.Function.Arguments != "" {
		t.Errorf("c1 arguments = %q", c1.Function.Arguments)
	}
	_, c2 := f.toolCall(t, "c2")
	args := parseArgs(t, c2.Function.Arguments)
	if _, ok := args["nested"]; !ok {
		t.Error("c2 lacks nested")
	}
	if _, ok := args["flag"]; !ok {
		t.Error("c2 lacks flag")
	}
}

// C#: tests/AGUI.Client.UnitTests/ConcurrentBuilderTest.cs::ConcurrentBuilderTest.SequentialTextMessages_BuilderResetsCorrectly
func TestCsConcurrentBuilderTest_SequentialTextMessages_BuilderResetsCorrectly(t *testing.T) {
	f := started(t, "thread1", "run1")
	f.mustPush(t,
		textStart("m1", agui.TextMessageRoleAssistant), textContent("m1", "First"), textEnd("m1"),
		textStart("m2", agui.TextMessageRoleUser), textContent("m2", "Second"), textEnd("m2"),
		textStart("m3", agui.TextMessageRoleSystem), textContent("m3", "Third"), textEnd("m3"))
	for _, want := range []struct {
		id   string
		role agui.Role
		text string
	}{{"m1", agui.RoleAssistant, "First"}, {"m2", agui.RoleUser, "Second"}, {"m3", agui.RoleSystem, "Third"}} {
		m := f.message(t, want.id)
		if roleOf(m) != want.role || textOf(m) != want.text {
			t.Errorf("%s: %s %q, want %s %q", want.id, roleOf(m), textOf(m), want.role, want.text)
		}
	}
}

// C#: tests/AGUI.Client.UnitTests/ConcurrentBuilderTest.cs::ConcurrentBuilderTest.TextBuilder_MultipleContentDeltas_EmitsAll
func TestCsConcurrentBuilderTest_TextBuilder_MultipleContentDeltas_EmitsAll(t *testing.T) {
	f := started(t, "thread1", "run1")
	f.mustPush(t, textStart("m1", agui.TextMessageRoleAssistant))
	var want strings.Builder
	for i := 0; i < 20; i++ {
		d := fmt.Sprintf("chunk%d ", i)
		want.WriteString(d)
		f.mustPush(t, textContent("m1", d))
	}
	f.mustPush(t, textEnd("m1"))
	var deltas []string
	for _, e := range f.delivered {
		if c, ok := e.(*agui.TextMessageContentEvent); ok {
			deltas = append(deltas, c.Delta)
		}
	}
	if len(deltas) != 20 {
		t.Fatalf("%d deltas delivered", len(deltas))
	}
	for i, d := range deltas {
		if d != fmt.Sprintf("chunk%d ", i) {
			t.Errorf("delta %d = %q", i, d)
		}
	}
	if m := f.message(t, "m1"); roleOf(m) != agui.RoleAssistant || textOf(m) != want.String() {
		t.Errorf("m1: %s %q", roleOf(m), textOf(m))
	}
}

// C#: tests/AGUI.Client.UnitTests/ConcurrentBuilderTest.cs::ConcurrentBuilderTest.ToolCallBuilder_StartSameIdTwice_Throws
func TestCsConcurrentBuilderTest_ToolCallBuilder_StartSameIdTwice_Throws(t *testing.T) {
	f := started(t, "t", "r")
	f.mustPush(t, tcStart("c1", "first_tool"))
	err := f.push(tcStart("c1", "second_tool"))
	if err == nil {
		t.Fatal("a second TOOL_CALL_START for an open call was accepted")
	}
	if !strings.Contains(err.Error(), "c1") || !strings.Contains(err.Error(), "already in progress") {
		t.Fatalf("error = %v", err)
	}
}

// C#: tests/AGUI.Client.UnitTests/ConcurrentBuilderTest.cs::ConcurrentBuilderTest.TextBuilder_ConcurrentStart_SupportsDifferentIds
func TestCsConcurrentBuilderTest_TextBuilder_ConcurrentStart_SupportsDifferentIds(t *testing.T) {
	f := started(t, "t", "r")
	f.mustPush(t,
		textStart("m1", agui.TextMessageRoleAssistant),
		textStart("m2", agui.TextMessageRoleUser),
		textContent("m1", "Hello"),
		textContent("m2", "World"))
	if r := roleOf(f.message(t, "m1")); r != agui.RoleAssistant {
		t.Errorf("m1 role %s", r)
	}
	if r := roleOf(f.message(t, "m2")); r != agui.RoleUser {
		t.Errorf("m2 role %s", r)
	}
}

// C#: tests/AGUI.Client.UnitTests/ConcurrentBuilderTest.cs::ConcurrentBuilderTest.InterleavedTextAndToolCalls_IndependentBuilders
func TestCsConcurrentBuilderTest_InterleavedTextAndToolCalls_IndependentBuilders(t *testing.T) {
	f := started(t, "thread1", "run1")
	f.mustPush(t,
		textStart("m1", agui.TextMessageRoleAssistant),
		tcStart("tc1", "search"),
		textContent("m1", "Let me "),
		tcArgs("tc1", `{"q":`),
		textContent("m1", "search..."),
		tcArgs("tc1", `"test"}`),
		textEnd("m1"),
		tcEnd("tc1"))
	var deltas []string
	for _, e := range f.delivered {
		if c, ok := e.(*agui.TextMessageContentEvent); ok {
			deltas = append(deltas, c.Delta)
		}
	}
	if !eqStrings(deltas, []string{"Let me ", "search..."}) {
		t.Errorf("deltas = %q", deltas)
	}
	if textOf(f.message(t, "m1")) != "Let me search..." {
		t.Errorf("m1 = %q", textOf(f.message(t, "m1")))
	}
	calls := f.toolCalls()
	if len(calls) != 1 || calls[0].Function.Name != "search" || calls[0].Function.Arguments != `{"q":"test"}` {
		t.Errorf("calls = %+v", calls)
	}
}
