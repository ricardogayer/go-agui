package abstractions_events_test

// Port of tests/AGUI.Abstractions.UnitTests/AGUIDeveloperMessageTest.cs.

import (
	"encoding/json"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// C#: tests/AGUI.Abstractions.UnitTests/AGUIDeveloperMessageTest.cs::AGUIDeveloperMessageTest.Serializes_RoleAndContent
func TestCsAGUIDeveloperMessageTest_Serializes_RoleAndContent(t *testing.T) {
	var message agui.Message = &agui.DeveloperMessage{ID: "d1", Content: "be helpful"}
	b, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	if err := agui.Validate("Message", b); err != nil {
		t.Errorf("not a valid 1.0 Message: %v\n%s", err, b)
	}
	root := toObject(t, b)
	eq(t, "role", root["role"], "developer")
	eq(t, "id", root["id"], "d1")
	eq(t, "content", root["content"], "be helpful")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIDeveloperMessageTest.cs::AGUIDeveloperMessageTest.Deserializes_AsDeveloperMessage_NotSystemMessage
func TestCsAGUIDeveloperMessageTest_Deserializes_AsDeveloperMessage_NotSystemMessage(t *testing.T) {
	m, err := agui.UnmarshalMessage([]byte(`{ "id": "d1", "role": "developer", "content": "instructions" }`))
	if err != nil {
		t.Fatal(err)
	}
	developer := asType[*agui.DeveloperMessage](t, m)
	eq(t, "ID", developer.ID, "d1")
	eq(t, "Content", developer.Content, "instructions")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIDeveloperMessageTest.cs::AGUIDeveloperMessageTest.NameAndEncryptedValue_RoundTrip
func TestCsAGUIDeveloperMessageTest_NameAndEncryptedValue_RoundTrip(t *testing.T) {
	var message agui.Message = &agui.DeveloperMessage{
		ID:             "d1",
		Content:        "c",
		Name:           agui.Ptr("system-author"),
		EncryptedValue: agui.Ptr("enc"),
	}
	b, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	m, err := agui.UnmarshalMessage(b)
	if err != nil {
		t.Fatal(err)
	}
	developer := asType[*agui.DeveloperMessage](t, m)
	eq(t, "Name", agui.Deref(developer.Name), "system-author")
	eq(t, "EncryptedValue", agui.Deref(developer.EncryptedValue), "enc")
}
