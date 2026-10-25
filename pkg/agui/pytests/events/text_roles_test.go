package events_test

// Port of the official AG-UI Python SDK tests/test_text_roles.py.

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// Every role a text message may take (tool excluded).
var textMessageRoles = []agui.TextMessageRole{
	agui.TextMessageRoleDeveloper, agui.TextMessageRoleSystem,
	agui.TextMessageRoleAssistant, agui.TextMessageRoleUser,
}

// Python: tests/test_text_roles.py::TestTextMessageRoles::test_text_message_start_with_all_roles
func TestPyTestTextMessageRoles_TextMessageStartWithAllRoles(t *testing.T) {
	for _, role := range textMessageRoles {
		t.Run(string(role), func(t *testing.T) {
			e := &agui.TextMessageStartEvent{MessageID: "test-msg", Role: agui.Ptr(role)}
			eq(t, "Type", e.Type(), agui.EventTypeTextMessageStart)
			eq(t, "MessageID", e.MessageID, "test-msg")
			eq(t, "Role", *e.Role, role)
			if err := agui.ValidateEvent(e); err != nil {
				t.Errorf("ValidateEvent: %v", err)
			}
		})
	}
}

// Python: tests/test_text_roles.py::TestTextMessageRoles::test_text_message_chunk_with_all_roles
func TestPyTestTextMessageRoles_TextMessageChunkWithAllRoles(t *testing.T) {
	for _, role := range textMessageRoles {
		t.Run(string(role), func(t *testing.T) {
			delta := "Hello from " + string(role)
			e := &agui.TextMessageChunkEvent{MessageID: agui.Ptr("test-msg"), Role: agui.Ptr(role), Delta: agui.Ptr(delta)}
			eq(t, "Type", e.Type(), agui.EventTypeTextMessageChunk)
			eq(t, "MessageID", *e.MessageID, "test-msg")
			eq(t, "Role", *e.Role, role)
			eq(t, "Delta", *e.Delta, delta)
			if err := agui.ValidateEvent(e); err != nil {
				t.Errorf("ValidateEvent: %v", err)
			}
		})
	}
}

// Python: tests/test_text_roles.py::TestTextMessageRoles::test_text_message_chunk_without_role
func TestPyTestTextMessageRoles_TextMessageChunkWithoutRole(t *testing.T) {
	e := &agui.TextMessageChunkEvent{MessageID: agui.Ptr("test-msg"), Delta: agui.Ptr("Hello without role")}
	eq(t, "Type", e.Type(), agui.EventTypeTextMessageChunk)
	eq(t, "MessageID", *e.MessageID, "test-msg")
	if e.Role != nil {
		t.Errorf("Role = %v, want nil", *e.Role)
	}
	eq(t, "Delta", *e.Delta, "Hello without role")
	absent(t, dump(t, e), "role")
	if err := agui.ValidateEvent(e); err != nil {
		t.Errorf("ValidateEvent: %v", err)
	}
}

// Python: tests/test_text_roles.py::TestTextMessageRoles::test_multiple_messages_different_roles
func TestPyTestTextMessageRoles_MultipleMessagesDifferentRoles(t *testing.T) {
	var events []agui.Event
	for _, role := range textMessageRoles {
		id := "msg-" + string(role)
		events = append(events,
			&agui.TextMessageStartEvent{MessageID: id, Role: agui.Ptr(role)},
			&agui.TextMessageContentEvent{MessageID: id, Delta: "Message from " + string(role)},
			&agui.TextMessageEndEvent{MessageID: id},
		)
	}
	eq(t, "len(events)", len(events), len(textMessageRoles)*3)
	for i, role := range textMessageRoles {
		start, ok := events[i*3].(*agui.TextMessageStartEvent)
		if !ok {
			t.Fatalf("events[%d] is %T", i*3, events[i*3])
		}
		eq(t, "Role", *start.Role, role)
		eq(t, "MessageID", start.MessageID, "msg-"+string(role))
	}
}

// Python: tests/test_text_roles.py::TestTextMessageRoles::test_text_message_serialization
//
// Python's model_dump() (snake_case keys) and TextMessageStartEvent(**dict)
// map to Go's single JSON form (camelCase) and json.Unmarshal.
func TestPyTestTextMessageRoles_TextMessageSerialization(t *testing.T) {
	for _, role := range textMessageRoles {
		t.Run(string(role), func(t *testing.T) {
			e := agui.TextMessageStartEvent{MessageID: "test-msg", Role: agui.Ptr(role)}
			d := dump(t, e)
			eq(t, "role", d["role"], string(role))
			eq(t, "type", d["type"], string(agui.EventTypeTextMessageStart))
			eq(t, "messageId", d["messageId"], "test-msg")

			var back agui.TextMessageStartEvent
			if err := json.Unmarshal(mustJSON(t, e), &back); err != nil {
				t.Fatal(err)
			}
			eq(t, "Role", *back.Role, role)
			if !reflect.DeepEqual(back, e) {
				t.Errorf("recreated event differs:\n got %#v\nwant %#v", back, e)
			}
		})
	}
}

// Python: tests/test_text_roles.py::TestTextMessageRoles::test_invalid_role_rejected
func TestPyTestTextMessageRoles_InvalidRoleRejected(t *testing.T) {
	cases := []struct{ def, doc string }{
		{"TextMessageStartEvent", `{"type":"TEXT_MESSAGE_START","messageId":"test-msg","role":"invalid_role"}`},
		{"TextMessageStartEvent", `{"type":"TEXT_MESSAGE_START","messageId":"test-msg","role":"tool"}`},
		{"TextMessageChunkEvent", `{"type":"TEXT_MESSAGE_CHUNK","messageId":"test-msg","role":"tool","delta":"Tool message"}`},
	}
	for _, c := range cases {
		if err := agui.Validate(c.def, []byte(c.doc)); err == nil {
			t.Errorf("Validate(%s) accepted %s", c.def, c.doc)
		}
		if err := agui.Validate("Event", []byte(c.doc)); err == nil {
			t.Errorf("Validate(Event) accepted %s", c.doc)
		}
	}
	// A Go value holding a role outside the set is refused before emission.
	bad := &agui.TextMessageStartEvent{MessageID: "test-msg", Role: agui.Ptr(agui.TextMessageRole("tool"))}
	if err := agui.ValidateEvent(bad); err == nil {
		t.Error("ValidateEvent accepted TEXT_MESSAGE_START with role tool")
	}
}

// Python: tests/test_text_roles.py::TestTextMessageRoles::test_text_message_start_name_preserved_with_roles
func TestPyTestTextMessageRoles_TextMessageStartNamePreservedWithRoles(t *testing.T) {
	for _, role := range textMessageRoles {
		t.Run(string(role), func(t *testing.T) {
			e := agui.TextMessageStartEvent{MessageID: "test-msg", Role: agui.Ptr(role), Name: agui.Ptr("test-agent")}
			eq(t, "Role", *e.Role, role)
			eq(t, "Name", *e.Name, "test-agent")
			var back agui.TextMessageStartEvent
			if err := json.Unmarshal(mustJSON(t, e), &back); err != nil {
				t.Fatal(err)
			}
			eq(t, "round-tripped Name", *back.Name, "test-agent")
			eq(t, "round-tripped Role", *back.Role, role)
		})
	}
}

// Python: tests/test_text_roles.py::TestTextMessageRoles::test_text_message_start_default_role
func TestPyTestTextMessageRoles_TextMessageStartDefaultRole(t *testing.T) {
	e := &agui.TextMessageStartEvent{MessageID: "test-msg"}
	eq(t, "Type", e.Type(), agui.EventTypeTextMessageStart)
	eq(t, "MessageID", e.MessageID, "test-msg")
	// Absent means assistant (schema prose); never materialised.
	if e.Role != nil {
		t.Errorf("Role = %v, want nil", *e.Role)
	}
	d, err := agui.ParseEvent([]byte(`{"type":"TEXT_MESSAGE_START","messageId":"test-msg"}`))
	if err != nil {
		t.Fatal(err)
	}
	if r := d.(*agui.TextMessageStartEvent).Role; r != nil {
		t.Errorf("decoded Role = %v, want nil", *r)
	}
}
