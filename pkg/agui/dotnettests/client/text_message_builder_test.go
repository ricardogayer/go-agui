package client_test

// Port of sdks/dotnet/tests/AGUI.Client.UnitTests/TextMessageBuilderTest.cs.
//
// The .NET TextMessageBuilder turns TEXT_MESSAGE_* events into
// ChatResponseUpdates (role, text, message id, author name, conversation and
// response ids). The Go analogue is the consumer pipeline: the delivered
// TEXT_MESSAGE_CONTENT carries the delta, and client.Store assembles the
// message (id, role, content, name) under the run RUN_STARTED opened
// (thread/run ids).

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

func textStart(id string, role agui.TextMessageRole) *agui.TextMessageStartEvent {
	return &agui.TextMessageStartEvent{MessageID: id, Role: agui.Ptr(role)}
}

func textContent(id, delta string) *agui.TextMessageContentEvent {
	return &agui.TextMessageContentEvent{MessageID: id, Delta: delta}
}

func textEnd(id string) *agui.TextMessageEndEvent { return &agui.TextMessageEndEvent{MessageID: id} }

// lastContent returns the last delivered TEXT_MESSAGE_CONTENT.
func (f *feed) lastContent(t *testing.T) *agui.TextMessageContentEvent {
	t.Helper()
	for i := len(f.delivered) - 1; i >= 0; i-- {
		if c, ok := f.delivered[i].(*agui.TextMessageContentEvent); ok {
			return c
		}
	}
	t.Fatal("no TEXT_MESSAGE_CONTENT delivered")
	return nil
}

// C#: tests/AGUI.Client.UnitTests/TextMessageBuilderTest.cs::TextMessageBuilderTest.EmitTextUpdate_ReturnsUpdateWithTextContent
func TestCsTextMessageBuilderTest_EmitTextUpdate_ReturnsUpdateWithTextContent(t *testing.T) {
	f := started(t, "thread-1", "run-1")
	f.mustPush(t, textStart("msg-1", agui.TextMessageRoleAssistant), textContent("msg-1", "Hello"))
	c := f.lastContent(t)
	if c.Delta != "Hello" || c.MessageID != "msg-1" {
		t.Errorf("delivered %+v", c)
	}
	m := f.message(t, "msg-1")
	if roleOf(m) != agui.RoleAssistant || textOf(m) != "Hello" {
		t.Errorf("message %s %q", roleOf(m), textOf(m))
	}
	rs, ok := f.delivered[0].(*agui.RunStartedEvent)
	if !ok || rs.ThreadID != "thread-1" || rs.RunID != "run-1" {
		t.Errorf("the run's ids: %+v", f.delivered[0])
	}
}

// C#: tests/AGUI.Client.UnitTests/TextMessageBuilderTest.cs::TextMessageBuilderTest.EmitTextUpdate_MapsUserRole
func TestCsTextMessageBuilderTest_EmitTextUpdate_MapsUserRole(t *testing.T) {
	f := started(t, "t", "r")
	f.mustPush(t, textStart("msg-1", agui.TextMessageRoleUser), textContent("msg-1", "test"))
	if r := roleOf(f.message(t, "msg-1")); r != agui.RoleUser {
		t.Fatalf("role %s", r)
	}
}

// C#: tests/AGUI.Client.UnitTests/TextMessageBuilderTest.cs::TextMessageBuilderTest.EmitTextUpdate_MapsSystemRole
func TestCsTextMessageBuilderTest_EmitTextUpdate_MapsSystemRole(t *testing.T) {
	f := started(t, "t", "r")
	f.mustPush(t, textStart("msg-1", agui.TextMessageRoleSystem), textContent("msg-1", "test"))
	if r := roleOf(f.message(t, "msg-1")); r != agui.RoleSystem {
		t.Fatalf("role %s", r)
	}
}

// C#: tests/AGUI.Client.UnitTests/TextMessageBuilderTest.cs::TextMessageBuilderTest.EmitTextUpdate_MapsCustomRole
//
// "developer" is a custom ChatRole in MEAI; in AG-UI it is one of the four
// text message roles.
func TestCsTextMessageBuilderTest_EmitTextUpdate_MapsCustomRole(t *testing.T) {
	f := started(t, "t", "r")
	f.mustPush(t, textStart("msg-1", agui.TextMessageRoleDeveloper), textContent("msg-1", "test"))
	m := f.message(t, "msg-1")
	if roleOf(m) != agui.RoleDeveloper || textOf(m) != "test" {
		t.Fatalf("message %s %q", roleOf(m), textOf(m))
	}
}

// C#: tests/AGUI.Client.UnitTests/TextMessageBuilderTest.cs::TextMessageBuilderTest.EndCurrentMessage_ResetsState
func TestCsTextMessageBuilderTest_EndCurrentMessage_ResetsState(t *testing.T) {
	f := started(t, "t", "r")
	f.mustPush(t,
		textStart("msg-1", agui.TextMessageRoleAssistant),
		textEnd("msg-1"),
		textStart("msg-2", agui.TextMessageRoleUser)) // must not be refused
}

// C#: tests/AGUI.Client.UnitTests/TextMessageBuilderTest.cs::TextMessageBuilderTest.AddTextStart_SupportsConcurrentMessages
func TestCsTextMessageBuilderTest_AddTextStart_SupportsConcurrentMessages(t *testing.T) {
	f := started(t, "t", "r")
	f.mustPush(t,
		textStart("msg-1", agui.TextMessageRoleAssistant),
		textStart("msg-2", agui.TextMessageRoleUser),
		textContent("msg-1", "Hello"),
		textContent("msg-2", "World"))
	m1, m2 := f.message(t, "msg-1"), f.message(t, "msg-2")
	if roleOf(m1) != agui.RoleAssistant || textOf(m1) != "Hello" {
		t.Errorf("msg-1: %s %q", roleOf(m1), textOf(m1))
	}
	if roleOf(m2) != agui.RoleUser || textOf(m2) != "World" {
		t.Errorf("msg-2: %s %q", roleOf(m2), textOf(m2))
	}
}

// C#: tests/AGUI.Client.UnitTests/TextMessageBuilderTest.cs::TextMessageBuilderTest.EndCurrentMessage_RemovesMessageById
func TestCsTextMessageBuilderTest_EndCurrentMessage_RemovesMessageById(t *testing.T) {
	f := started(t, "t", "r")
	f.mustPush(t,
		textStart("msg-1", agui.TextMessageRoleAssistant),
		textStart("msg-2", agui.TextMessageRoleUser),
		textEnd("msg-1"),
		textContent("msg-2", "test"))
	if m := f.message(t, "msg-2"); roleOf(m) != agui.RoleUser || textOf(m) != "test" {
		t.Errorf("msg-2: %s %q", roleOf(m), textOf(m))
	}
	// msg-1 is closed: more content for it violates the open/content/close discipline.
	if err := f.push(textContent("msg-1", "late")); err == nil {
		t.Error("content for an ended message was accepted")
	}
}

// C#: tests/AGUI.Client.UnitTests/TextMessageBuilderTest.cs::TextMessageBuilderTest.EmitTextUpdate_WithName_SetsAuthorName
func TestCsTextMessageBuilderTest_EmitTextUpdate_WithName_SetsAuthorName(t *testing.T) {
	f := started(t, "thread-1", "run-1")
	start := textStart("msg-1", agui.TextMessageRoleAssistant)
	start.Name = agui.Ptr("TestAgent")
	f.mustPush(t, start, textContent("msg-1", "Hello"))
	if n := nameOf(f.message(t, "msg-1")); agui.Deref(n) != "TestAgent" {
		t.Fatalf("name = %v", n)
	}
}

// C#: tests/AGUI.Client.UnitTests/TextMessageBuilderTest.cs::TextMessageBuilderTest.EmitTextUpdate_WithoutName_AuthorNameIsNull
func TestCsTextMessageBuilderTest_EmitTextUpdate_WithoutName_AuthorNameIsNull(t *testing.T) {
	f := started(t, "t", "r")
	f.mustPush(t, textStart("msg-1", agui.TextMessageRoleAssistant), textContent("msg-1", "Hello"))
	if n := nameOf(f.message(t, "msg-1")); n != nil {
		t.Fatalf("name = %q, want absent", *n)
	}
}
