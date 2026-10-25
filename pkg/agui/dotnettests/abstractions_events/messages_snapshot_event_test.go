package abstractions_events_test

// Port of tests/AGUI.Abstractions.UnitTests/MessagesSnapshotEventTest.cs.

import (
	"encoding/json"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// C#: tests/AGUI.Abstractions.UnitTests/MessagesSnapshotEventTest.cs::MessagesSnapshotEventTest.MessagesSnapshotEvent_SerializesAndDeserializesViaBaseEvent
func TestCsMessagesSnapshotEventTest_MessagesSnapshotEvent_SerializesAndDeserializesViaBaseEvent(t *testing.T) {
	evt := &agui.MessagesSnapshotEvent{
		Messages: agui.Messages{
			&agui.UserMessage{ID: "msg-1", Content: agui.PartsContent(agui.NewTextPart("Hello"))},
		},
	}
	root := serialize(t, evt)
	eq(t, "type", root["type"], "MESSAGES_SNAPSHOT")
	eq(t, "messages[0].id", at(t, root, "messages", 0, "id"), "msg-1")
	eq(t, "messages[0].role", at(t, root, "messages", 0, "role"), "user")

	b, _ := json.Marshal(evt)
	snapshot := asType[*agui.MessagesSnapshotEvent](t, viaBaseEvent(t, string(b)))
	if len(snapshot.Messages) != 1 {
		t.Fatalf("len(Messages) = %d, want 1", len(snapshot.Messages))
	}
	message := asType[*agui.UserMessage](t, snapshot.Messages[0])
	parts := message.Content.Parts()
	if !message.Content.IsParts() || len(parts) != 1 {
		t.Fatalf("content = %#v, want one part", message.Content)
	}
	text := asType[*agui.TextPart](t, parts[0])
	eq(t, "Text", text.Text, "Hello")
}

// C#: tests/AGUI.Abstractions.UnitTests/MessagesSnapshotEventTest.cs::MessagesSnapshotEventTest.MessagesSnapshotEvent_DeserializesViaBaseEvent
//
// Adapted: .NET normalises string content into a single AGUITextInputContent.
// The spec keeps "content": "hello" a string (UserMessage.content is a string
// or a list of parts, and the two are distinct), so Go keeps it as string
// content; the intent (the text "hello" is read) is asserted.
func TestCsMessagesSnapshotEventTest_MessagesSnapshotEvent_DeserializesViaBaseEvent(t *testing.T) {
	evt := viaBaseEvent(t, `{
	  "type": "MESSAGES_SNAPSHOT",
	  "messages": [
	    { "id": "msg-1", "role": "user", "content": "hello" }
	  ]
	}`)
	snapshot := asType[*agui.MessagesSnapshotEvent](t, evt)
	if len(snapshot.Messages) != 1 {
		t.Fatalf("len(Messages) = %d, want 1", len(snapshot.Messages))
	}
	message := asType[*agui.UserMessage](t, snapshot.Messages[0])
	eq(t, "ID", message.ID, "msg-1")
	eq(t, "IsParts", message.Content.IsParts(), false)
	eq(t, "Content", message.Content.String(), "hello")
}
