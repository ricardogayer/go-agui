package types_test

// Port of sdks/python/tests/test_deprecated_names.py: the two 0.x names the
// Python SDK keeps importable for one release. Go has neither name; where the
// test pins wire behaviour (the retired binary part, the old subAgents key),
// the Go counterpart is tested.

import (
	"encoding/json"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

func TestPyTestBinaryInputContent_test_is_exported(t *testing.T) {
	// Python: tests/test_deprecated_names.py::TestBinaryInputContent::test_is_exported
	t.Skip("N/A in Go: checks ag_ui.core.__all__ lists the Python-only deprecated class BinaryInputContent; Go has no such type")
}

func TestPyTestBinaryInputContent_test_validates_the_legacy_wire_shape(t *testing.T) {
	// Python: tests/test_deprecated_names.py::TestBinaryInputContent::test_validates_the_legacy_wire_shape
	// Go has no BinaryInputContent class. The legacy wire shape is still read
	// in two ways: the parsing layer keeps it verbatim (as *agui.UnknownPart),
	// and the compatibility shim in ParseRunAgentInput upgrades it to the 1.0
	// media part carrying the same mime type, data and filename.
	legacy := `{"type":"binary","mimeType":"image/png","data":"aGk=","filename":"a.png"}`
	p, err := agui.UnmarshalContentPart([]byte(legacy))
	if err != nil {
		t.Fatal(err)
	}
	u, ok := p.(*agui.UnknownPart)
	if !ok {
		t.Fatalf("decoded %T", p)
	}
	eq(t, "kind", u.Kind, "binary")
	eq(t, "verbatim dump", marshalObj(t, u), map[string]any{"type": "binary", "mimeType": "image/png", "data": "aGk=", "filename": "a.png"})

	in, warnings, err := agui.ParseRunAgentInput([]byte(`{"threadId":"t","runId":"r","messages":[{"id":"u","role":"user","content":[` + legacy + `]}]}`))
	if err != nil {
		t.Fatalf("ParseRunAgentInput: %v", err)
	}
	if len(warnings) == 0 {
		t.Error("upgrade of a retired shape was not announced")
	}
	parts := in.Messages[0].(*agui.UserMessage).Content.Parts()
	if len(parts) != 1 {
		t.Fatalf("parts = %#v", parts)
	}
	img, ok := parts[0].(*agui.ImagePart)
	if !ok {
		t.Fatalf("upgraded to %T, want *agui.ImagePart", parts[0])
	}
	src, ok := img.Source.(*agui.DataSource)
	if !ok {
		t.Fatalf("source is %T", img.Source)
	}
	eq(t, "mimeType", src.MimeType, "image/png")
	eq(t, "data", src.Value, "aGk=")
	eq(t, "filename", img.Metadata, map[string]any{"filename": "a.png"})
}

func TestPyTestBinaryInputContent_test_requires_a_payload_source_as_0x_did(t *testing.T) {
	// Python: tests/test_deprecated_names.py::TestBinaryInputContent::test_requires_a_payload_source_as_0x_did
	// A binary part with neither data nor url cannot be upgraded: the shim
	// reports it, no media part is invented, and the processing model strips
	// what is left (it is not a 1.0 part).
	in, warnings, err := agui.ParseRunAgentInput([]byte(`{"threadId":"t","runId":"r","messages":[
		{"id":"u","role":"user","content":[{"type":"binary","mimeType":"image/png"}]}]}`))
	if err != nil {
		t.Fatalf("ParseRunAgentInput: %v", err)
	}
	if len(warnings) == 0 {
		t.Error("an unconvertible binary part was not reported")
	}
	for _, m := range in.Messages {
		if u, ok := m.(*agui.UserMessage); ok {
			for _, p := range u.Content.Parts() {
				t.Errorf("a part survived: %T", p)
			}
		}
	}
}

func TestPyTestBinaryInputContent_test_is_not_a_content_part(t *testing.T) {
	// Python: tests/test_deprecated_names.py::TestBinaryInputContent::test_is_not_a_content_part
	// The protocol retired the part: a message carrying one is rejected at
	// validation.
	doc := []byte(`{"id":"u","role":"user","content":[{"type":"binary","mimeType":"image/png","data":"aGk="}]}`)
	if err := agui.Validate("UserMessage", doc); err == nil {
		t.Error("Validate accepted a binary part")
	}
	var msg agui.UserMessage
	if err := json.Unmarshal(doc, &msg); err != nil {
		t.Fatal(err)
	}
	if _, ok := msg.Content.Parts()[0].(*agui.UnknownPart); !ok {
		t.Errorf("binary part decoded as %T, want *agui.UnknownPart", msg.Content.Parts()[0])
	}
}

func TestPyTestSubAgentInfo_test_is_an_alias_of_the_1_0_class(t *testing.T) {
	// Python: tests/test_deprecated_names.py::TestSubAgentInfo::test_is_an_alias_of_the_1_0_class
	t.Skip("N/A in Go: checks the Python-only deprecated alias SubAgentInfo is SubagentInfo and listed in __all__; Go only has agui.SubagentInfo")
}

func TestPyTestSubAgentInfo_test_wire_key_is_the_1_0_spelling_only(t *testing.T) {
	// Python: tests/test_deprecated_names.py::TestSubAgentInfo::test_wire_key_is_the_1_0_spelling_only
	var caps agui.MultiAgentCapabilities
	if err := json.Unmarshal([]byte(`{"subagents":[{"name":"researcher"}]}`), &caps); err != nil {
		t.Fatal(err)
	}
	if len(caps.Subagents) != 1 {
		t.Fatalf("subagents = %v", caps.Subagents)
	}
	eq(t, "subagents[0].name", caps.Subagents[0].Name, "researcher")

	// The old key is unknown material, never the subagents field. The
	// processing model strips it ...
	old := []byte(`{"subAgents":[{"name":"researcher"}]}`)
	res, err := agui.Sanitize("MultiAgentCapabilities", old, agui.SanitizeOptions{})
	if err != nil {
		t.Fatalf("Sanitize: %v", err)
	}
	if _, ok := res.Value.(map[string]any)["subAgents"]; ok || len(res.Warnings) == 0 {
		t.Errorf("Sanitize kept subAgents or did not warn: %v %v", res.Value, res.Warnings)
	}
	// ... and the tolerant typed decode must not read it as subagents either.
	// JSON member names are case-sensitive and the schema names the property
	// "subagents" only (spec/1.0/schema.json MultiAgentCapabilities; fixture
	// MultiAgentCapabilities/invalid/old-subagents-key.json), and
	// basic/processing.mdx: unrecognised material MUST NOT be delivered "as
	// though the protocol described it".
	var oldKey agui.MultiAgentCapabilities
	if err := json.Unmarshal(old, &oldKey); err != nil {
		t.Fatal(err)
	}
	if oldKey.Subagents != nil {
		t.Errorf("the 0.x key subAgents populated Subagents (%v): encoding/json matches member names case-insensitively", oldKey.Subagents)
	}
}
