package types_test

// Port of sdks/python/tests/test_content_parts.py: tool results carry the same
// content parts as user messages; the file source arm; the pre-1.0 names.

import (
	"encoding/json"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// partsFixture is PARTS: a text part with id and metadata, and a document
// part with an inline data source.
func partsFixture() agui.Content {
	text := agui.NewTextPart("Proration rules: when a plan changes mid-cycle...")
	text.ID = agui.Ptr("p1")
	text.Metadata = map[string]any{"source": "https://docs.internal/billing/proration", "title": "Proration rules"}
	doc := agui.NewDocumentPart(&agui.DataSource{Value: "JVBERi0x", MimeType: "application/pdf"})
	return agui.PartsContent(text, doc)
}

func TestPyTestToolResultContent_test_tool_message_accepts_a_string(t *testing.T) {
	// Python: tests/test_content_parts.py::TestToolResultContent::test_tool_message_accepts_a_string
	msg := agui.ToolMessage{ID: "m1", ToolCallID: "c1", Content: agui.TextContent("3 results found.")}
	if msg.Content.IsParts() {
		t.Error("string content reported as parts")
	}
	eq(t, "content", msg.Content.String(), "3 results found.")
	eq(t, "wire content", marshalObj(t, msg)["content"], "3 results found.")
}

func TestPyTestToolResultContent_test_tool_message_accepts_parts(t *testing.T) {
	// Python: tests/test_content_parts.py::TestToolResultContent::test_tool_message_accepts_parts
	msg := agui.ToolMessage{ID: "m1", ToolCallID: "c1", Content: partsFixture()}
	content := marshalObj(t, msg)["content"].([]any)
	p0, p1 := content[0].(map[string]any), content[1].(map[string]any)
	eq(t, "content[0].type", p0["type"], "text")
	eq(t, "content[0].metadata.title", p0["metadata"].(map[string]any)["title"], "Proration rules")
	eq(t, "content[1].source.mimeType", p1["source"].(map[string]any)["mimeType"], "application/pdf")
	b, _ := json.Marshal(msg)
	if err := agui.Validate("ToolMessage", b); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestPyTestToolResultContent_test_tool_message_parts_round_trip_from_camel_case_json(t *testing.T) {
	// Python: tests/test_content_parts.py::TestToolResultContent::test_tool_message_parts_round_trip_from_camel_case_json
	var msg agui.ToolMessage
	err := json.Unmarshal([]byte(`{"id":"m1","role":"tool","toolCallId":"c1","content":[
		{"type":"text","text":"Invoice attached."},
		{"type":"document","id":"inv","source":{"type":"url","value":"https://example.com/i.pdf"}}]}`), &msg)
	if err != nil {
		t.Fatal(err)
	}
	if !msg.Content.IsParts() {
		t.Fatal("content is not a list")
	}
	parts := msg.Content.Parts()
	if _, ok := parts[0].(*agui.TextPart); !ok {
		t.Errorf("parts[0] is %T", parts[0])
	}
	doc, ok := parts[1].(*agui.DocumentPart)
	if !ok {
		t.Fatalf("parts[1] is %T", parts[1])
	}
	if _, ok := doc.Source.(*agui.URLSource); !ok {
		t.Errorf("source is %T", doc.Source)
	}
	eq(t, "parts[1].id", agui.Deref(doc.ID), "inv")
}

func TestPyTestToolResultContent_test_tool_call_result_event_accepts_parts(t *testing.T) {
	// Python: tests/test_content_parts.py::TestToolResultContent::test_tool_call_result_event_accepts_parts
	event := agui.ToolCallResultEvent{MessageID: "m2", ToolCallID: "c1", Content: partsFixture()}
	dumped := marshalObj(t, event)
	eq(t, "type", dumped["type"], "TOOL_CALL_RESULT")
	eq(t, "len(content)", len(dumped["content"].([]any)), 2)
	if err := agui.ValidateEvent(&event); err != nil {
		t.Errorf("ValidateEvent: %v", err)
	}
}

func TestPyTestToolResultContent_test_message_union_reads_a_tool_message_with_parts(t *testing.T) {
	// Python: tests/test_content_parts.py::TestToolResultContent::test_message_union_reads_a_tool_message_with_parts
	m := mustUnmarshalMessage(t, `{"id":"m1","role":"tool","toolCallId":"c1","content":[{"type":"text","text":"hi"}]}`)
	tool, ok := m.(*agui.ToolMessage)
	if !ok {
		t.Fatalf("decoded %T", m)
	}
	eq(t, "content[0].text", tool.Content.Parts()[0].(*agui.TextPart).Text, "hi")
}

func TestPyTestToolResultContent_test_unknown_part_type_is_rejected(t *testing.T) {
	// Python: tests/test_content_parts.py::TestToolResultContent::test_unknown_part_type_is_rejected
	doc := []byte(`{"id":"m1","role":"tool","toolCallId":"c1","content":[{"type":"search_result","source":"https://example.com","title":"x"}]}`)
	if err := agui.Validate("ToolMessage", doc); err == nil {
		t.Error("Validate accepted an unknown part type")
	}
	// Go's typed layer keeps the part as *agui.UnknownPart, never as a known
	// part, and the processing model strips it with a warning.
	var msg agui.ToolMessage
	if err := json.Unmarshal(doc, &msg); err != nil {
		t.Fatal(err)
	}
	if u, ok := msg.Content.Parts()[0].(*agui.UnknownPart); !ok || u.Kind != "search_result" {
		t.Errorf("part decoded as %T", msg.Content.Parts()[0])
	}
	res, err := agui.Sanitize("ToolMessage", doc, agui.SanitizeOptions{})
	if err != nil {
		t.Fatalf("Sanitize: %v", err)
	}
	if len(res.Warnings) == 0 {
		t.Error("Sanitize stripped the part without a warning")
	}
	if parts := res.Value.(map[string]any)["content"].([]any); len(parts) != 0 {
		t.Errorf("unknown part survived Sanitize: %v", parts)
	}
}

func TestPyTestToolResultContent_test_structured_data_is_serialised_not_passed_as_an_object(t *testing.T) {
	// Python: tests/test_content_parts.py::TestToolResultContent::test_structured_data_is_serialised_not_passed_as_an_object
	doc := []byte(`{"id":"m1","role":"tool","toolCallId":"c1","content":{"temperature":22.5}}`)
	if err := agui.Validate("ToolMessage", doc); err == nil {
		t.Error("Validate accepted object content")
	}
	var msg agui.ToolMessage
	if err := json.Unmarshal(doc, &msg); err == nil {
		t.Error("typed decode accepted object content")
	}
}

func TestPyTestTextPart_test_carries_optional_id_and_metadata(t *testing.T) {
	// Python: tests/test_content_parts.py::TestTextPart::test_carries_optional_id_and_metadata
	p, err := agui.UnmarshalContentPart([]byte(`{"type":"text","id":"p1","text":"hi","metadata":{"title":"t"}}`))
	if err != nil {
		t.Fatal(err)
	}
	part, ok := p.(*agui.TextPart)
	if !ok {
		t.Fatalf("decoded %T", p)
	}
	eq(t, "id", agui.Deref(part.ID), "p1")
	eq(t, "metadata", part.Metadata, map[string]any{"title": "t"})
	eq(t, "bare dump", marshalObj(t, agui.NewTextPart("hi")), map[string]any{"type": "text", "text": "hi"})
}

func TestPyTestFileSource_test_user_message_document_part_round_trips(t *testing.T) {
	// Python: tests/test_content_parts.py::TestFileSource::test_user_message_document_part_round_trips
	source := map[string]any{"type": "file", "value": "file-abc123", "provider": "openai", "mimeType": "application/pdf"}
	doc, _ := json.Marshal(map[string]any{"id": "m1", "role": "user", "content": []any{map[string]any{"type": "document", "source": source}}})
	var msg agui.UserMessage
	if err := json.Unmarshal(doc, &msg); err != nil {
		t.Fatal(err)
	}
	part, ok := msg.Content.Parts()[0].(*agui.DocumentPart)
	if !ok {
		t.Fatalf("part is %T", msg.Content.Parts()[0])
	}
	fs, ok := part.Source.(*agui.FileSource)
	if !ok {
		t.Fatalf("source is %T", part.Source)
	}
	eq(t, "value", fs.Value, "file-abc123")
	eq(t, "provider", agui.Deref(fs.Provider), "openai")
	eq(t, "mimeType", agui.Deref(fs.MimeType), "application/pdf")
	dumped := marshalObj(t, msg)["content"].([]any)[0].(map[string]any)["source"]
	eq(t, "dumped source", dumped, source)
}

func TestPyTestFileSource_test_minimal_file_source_omits_the_optional_keys(t *testing.T) {
	// Python: tests/test_content_parts.py::TestFileSource::test_minimal_file_source_omits_the_optional_keys
	p, err := agui.UnmarshalContentPart([]byte(`{"type":"document","source":{"type":"file","value":"x"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := p.(*agui.DocumentPart).Source.(*agui.FileSource); !ok {
		t.Fatalf("source is %T", p.(*agui.DocumentPart).Source)
	}
	eq(t, "dump", marshalObj(t, p), map[string]any{"type": "document", "source": map[string]any{"type": "file", "value": "x"}})
}

func TestPyTestFileSource_test_file_source_without_a_value_is_rejected(t *testing.T) {
	// Python: tests/test_content_parts.py::TestFileSource::test_file_source_without_a_value_is_rejected
	doc := []byte(`{"type":"document","source":{"type":"file","provider":"openai"}}`)
	if err := agui.Validate("ContentPart", doc); err == nil {
		t.Error("Validate accepted a file source without a value")
	}
}

func TestPyTestFileSource_test_tool_message_carries_a_file_source_part(t *testing.T) {
	// Python: tests/test_content_parts.py::TestFileSource::test_tool_message_carries_a_file_source_part
	var msg agui.ToolMessage
	if err := json.Unmarshal([]byte(`{"id":"m1","role":"tool","toolCallId":"c1","content":[
		{"type":"document","source":{"type":"file","value":"files/xyz","provider":"google"}}]}`), &msg); err != nil {
		t.Fatal(err)
	}
	part, ok := msg.Content.Parts()[0].(*agui.DocumentPart)
	if !ok {
		t.Fatalf("part is %T", msg.Content.Parts()[0])
	}
	fs, ok := part.Source.(*agui.FileSource)
	if !ok {
		t.Fatalf("source is %T", part.Source)
	}
	eq(t, "value", fs.Value, "files/xyz")
}

func TestPyTestFileSource_test_file_source_is_exported_from_ag_ui_core(t *testing.T) {
	// Python: tests/test_content_parts.py::TestFileSource::test_file_source_is_exported_from_ag_ui_core
	// The Python test pins re-exports (ag_ui.core vs ag_ui.core.types, and
	// __all__). Go has one package, so the counterpart is that the file arm
	// is an exported PartSource with discriminator "file".
	var src agui.PartSource = &agui.FileSource{Value: "x"}
	eq(t, "SourceType", src.SourceType(), agui.SourceTypeFile)
	eq(t, "SourceTypeFile", agui.SourceTypeFile, "file")
}

func TestPyTestFileSource_test_no_legacy_input_content_alias(t *testing.T) {
	// Python: tests/test_content_parts.py::TestFileSource::test_no_legacy_input_content_alias
	t.Skip("N/A in Go: asserts a Python module attribute (InputContentFileSource) is absent; the Go package never had the 0.x InputContent* names, and an identifier's absence is a compile-time fact, not a runtime check")
}

func TestPyTestPre10Names_test_aliases_are_identical_classes(t *testing.T) {
	// Python: tests/test_content_parts.py::TestPre10Names::test_aliases_are_identical_classes
	t.Skip("N/A in Go: tests Python-only deprecated class aliases (InputContent, TextInputContent, InputContentUrlSource, ...) being identical objects; the Go package has no 0.x names")
}

func TestPyTestPre10Names_test_old_names_still_construct_and_validate(t *testing.T) {
	// Python: tests/test_content_parts.py::TestPre10Names::test_old_names_still_construct_and_validate
	t.Skip("N/A in Go: constructs parts through the Python-only 0.x aliases (TextInputContent, InputContentUrlSource, ImageInputContent); the 1.0 shapes they produce are covered by the TestPyTestBaseTypes_* and TestPyTestFileSource_* ports")
}
