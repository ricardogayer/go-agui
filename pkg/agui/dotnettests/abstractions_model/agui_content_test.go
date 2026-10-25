package abstractions_model_test

// Port of sdks/dotnet/tests/AGUI.Abstractions.UnitTests/AGUIContentTest.cs.
//
// This file also holds the small JSON helpers the other files of this package
// share.

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %T: %v", v, err)
	}
	return b
}

// decode parses JSON text into a generic value (numbers as float64).
func decode(t *testing.T, data []byte) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("unmarshal %s: %v", data, err)
	}
	return v
}

// obj marshals v and returns the resulting JSON object.
func obj(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := decode(t, mustJSON(t, v)).(map[string]any)
	if !ok {
		t.Fatalf("%T did not marshal to an object", v)
	}
	return m
}

func objOf(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("expected a JSON object, got %T (%v)", v, v)
	}
	return m
}

func arrOf(t *testing.T, v any) []any {
	t.Helper()
	a, ok := v.([]any)
	if !ok {
		t.Fatalf("expected a JSON array, got %T (%v)", v, v)
	}
	return a
}

func assertOmitted(t *testing.T, o map[string]any, names ...string) {
	t.Helper()
	for _, n := range names {
		if _, ok := o[n]; ok {
			t.Errorf("expected %q to be omitted from %v", n, o)
		}
	}
}

func assertEq(t *testing.T, got, want any, what string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s: got %#v, want %#v", what, got, want)
	}
}

// jsonEqual compares two JSON documents by value (key order aside).
func jsonEqual(t *testing.T, a, b []byte) bool {
	t.Helper()
	return reflect.DeepEqual(decode(t, a), decode(t, b))
}

func roundTripMessage(t *testing.T, m agui.Message) agui.Message {
	t.Helper()
	back, err := agui.UnmarshalMessage(mustJSON(t, m))
	if err != nil {
		t.Fatalf("UnmarshalMessage: %v", err)
	}
	return back
}

func roundTripEvent(t *testing.T, e agui.Event) agui.Event {
	t.Helper()
	back, err := agui.UnmarshalEvent(mustJSON(t, e))
	if err != nil {
		t.Fatalf("UnmarshalEvent: %v", err)
	}
	return back
}

func textOf(t *testing.T, p agui.ContentPart) string {
	t.Helper()
	tp, ok := p.(*agui.TextPart)
	if !ok {
		t.Fatalf("expected *agui.TextPart, got %T", p)
	}
	return tp.Text
}

// compact re-encodes JSON text without insignificant whitespace.
func compact(t *testing.T, s string) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(s)); err != nil {
		t.Fatalf("compact: %v", err)
	}
	return buf.Bytes()
}

// ---------------------------------------------------------------------------
// AGUIContentTest
// ---------------------------------------------------------------------------

// C#: tests/AGUI.Abstractions.UnitTests/AGUIContentTest.cs::AGUIContentTest.ImplicitFromString_StoresTextAndSurfacesAsSingleTextPart
//
// .NET's AGUIContent also enumerates string content as one synthetic text
// part; that is a collection convenience of the .NET type, not wire behaviour.
// Go's Content keeps the string as a string: it is not a part list, and its
// text is what String returns.
func TestCsAGUIContentTest_ImplicitFromString_StoresTextAndSurfacesAsSingleTextPart(t *testing.T) {
	c := agui.TextContent("hello")
	if c.IsParts() {
		t.Fatal("string content must not report IsParts")
	}
	assertEq(t, c.String(), "hello", "String()")
	if c.Parts() != nil {
		t.Fatalf("string content has no parts in Go, got %v", c.Parts())
	}
	assertEq(t, string(mustJSON(t, c)), `"hello"`, "wire form")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIContentTest.cs::AGUIContentTest.ImplicitFromList_StoresPartsAndIsNotText
func TestCsAGUIContentTest_ImplicitFromList_StoresPartsAndIsNotText(t *testing.T) {
	parts := []agui.ContentPart{agui.NewTextPart("a"), agui.NewTextPart("b")}
	c := agui.PartsContent(parts...)
	if !c.IsParts() {
		t.Fatal("expected list content")
	}
	if len(c.Parts()) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(c.Parts()))
	}
	assertEq(t, textOf(t, c.Parts()[0]), "a", "part 0")
	assertEq(t, textOf(t, c.Parts()[1]), "b", "part 1")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIContentTest.cs::AGUIContentTest.CollectionExpression_InitializesParts
func TestCsAGUIContentTest_CollectionExpression_InitializesParts(t *testing.T) {
	c := agui.PartsContent(agui.NewTextPart("x"), agui.NewTextPart("y"))
	if !c.IsParts() {
		t.Fatal("expected list content")
	}
	if len(c.Parts()) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(c.Parts()))
	}
	assertEq(t, textOf(t, c.Parts()[0]), "x", "part 0")
	assertEq(t, textOf(t, c.Parts()[1]), "y", "part 1")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIContentTest.cs::AGUIContentTest.Default_IsEmpty
//
// .NET's default AGUIContent has a null Value; Go's zero Content is the empty
// string (there is no null content on the wire: content is required).
func TestCsAGUIContentTest_Default_IsEmpty(t *testing.T) {
	var c agui.Content
	if c.IsParts() {
		t.Fatal("zero content must not be list content")
	}
	if len(c.Parts()) != 0 {
		t.Fatalf("zero content has parts: %v", c.Parts())
	}
	assertEq(t, c.String(), "", "String()")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIContentTest.cs::AGUIContentTest.UserMessage_StringContent_SerializesAsJsonString
func TestCsAGUIContentTest_UserMessage_StringContent_SerializesAsJsonString(t *testing.T) {
	o := obj(t, &agui.UserMessage{ID: "u1", Content: agui.TextContent("hello")})
	assertEq(t, o["role"], "user", "role")
	assertEq(t, o["content"], "hello", "content")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIContentTest.cs::AGUIContentTest.UserMessage_MultipleParts_SerializesAsArray
func TestCsAGUIContentTest_UserMessage_MultipleParts_SerializesAsArray(t *testing.T) {
	o := obj(t, &agui.UserMessage{ID: "u1", Content: agui.PartsContent(agui.NewTextPart("a"), agui.NewTextPart("b"))})
	content := arrOf(t, o["content"])
	if len(content) != 2 {
		t.Fatalf("expected 2 parts, got %v", content)
	}
	assertEq(t, objOf(t, content[0])["type"], "text", "content[0].type")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIContentTest.cs::AGUIContentTest.UserMessage_StringContent_RoundTrips
func TestCsAGUIContentTest_UserMessage_StringContent_RoundTrips(t *testing.T) {
	back := roundTripMessage(t, &agui.UserMessage{ID: "u1", Content: agui.TextContent("round trip")})
	u, ok := back.(*agui.UserMessage)
	if !ok {
		t.Fatalf("expected *agui.UserMessage, got %T", back)
	}
	if u.Content.IsParts() {
		t.Fatal("string content came back as parts")
	}
	assertEq(t, u.Content.String(), "round trip", "content")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIContentTest.cs::AGUIContentTest.UserMessage_DeserializesStringContent_AsSingleTextPart
//
// .NET surfaces string content as a single text part; Go keeps it as the
// string it was on the wire.
func TestCsAGUIContentTest_UserMessage_DeserializesStringContent_AsSingleTextPart(t *testing.T) {
	m, err := agui.UnmarshalMessage([]byte(`{ "id": "u1", "role": "user", "content": "hi there" }`))
	if err != nil {
		t.Fatal(err)
	}
	u, ok := m.(*agui.UserMessage)
	if !ok {
		t.Fatalf("expected *agui.UserMessage, got %T", m)
	}
	if u.Content.IsParts() {
		t.Fatal("string content decoded as parts")
	}
	assertEq(t, u.Content.String(), "hi there", "content")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIContentTest.cs::AGUIContentTest.ToolMessage_Parts_SerializeAsArrayAndRoundTrip
func TestCsAGUIContentTest_ToolMessage_Parts_SerializeAsArrayAndRoundTrip(t *testing.T) {
	msg := &agui.ToolMessage{
		ID:         "t1",
		ToolCallID: "c1",
		Content: agui.PartsContent(
			&agui.TextPart{ID: agui.Ptr("p1"), Text: "Invoice attached.", Metadata: map[string]any{"title": "INV-2291"}},
			agui.NewDocumentPart(&agui.URLSource{Value: "https://example.com/i.pdf", MimeType: agui.Ptr("application/pdf")}),
		),
	}
	data := mustJSON(t, msg)
	if err := agui.Validate("ToolMessage", data); err != nil {
		t.Fatalf("schema rejects the tool message: %v", err)
	}
	content := arrOf(t, objOf(t, decode(t, data))["content"])
	p0 := objOf(t, content[0])
	assertEq(t, p0["id"], "p1", "content[0].id")
	assertEq(t, objOf(t, p0["metadata"])["title"], "INV-2291", "content[0].metadata.title")
	assertEq(t, objOf(t, content[1])["type"], "document", "content[1].type")

	m, err := agui.UnmarshalMessage(data)
	if err != nil {
		t.Fatal(err)
	}
	tool := m.(*agui.ToolMessage)
	parts := tool.Content.Parts()
	if !tool.Content.IsParts() || len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %#v", tool.Content)
	}
	assertEq(t, textOf(t, parts[0]), "Invoice attached.", "parts[0].text")
	doc, ok := parts[1].(*agui.DocumentPart)
	if !ok {
		t.Fatalf("expected *agui.DocumentPart, got %T", parts[1])
	}
	src, ok := doc.Source.(*agui.URLSource)
	if !ok {
		t.Fatalf("expected *agui.URLSource, got %T", doc.Source)
	}
	assertEq(t, src.Value, "https://example.com/i.pdf", "source.value")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIContentTest.cs::AGUIContentTest.ToolMessage_StringContent_StillWritesAString
func TestCsAGUIContentTest_ToolMessage_StringContent_StillWritesAString(t *testing.T) {
	msg := &agui.ToolMessage{ID: "t1", ToolCallID: "c1", Content: agui.TextContent("3 results")}
	o := obj(t, msg)
	assertEq(t, o["content"], "3 results", "content")
	back := roundTripMessage(t, msg).(*agui.ToolMessage)
	if back.Content.IsParts() {
		t.Fatal("string content came back as parts")
	}
	assertEq(t, back.Content.String(), "3 results", "content")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIContentTest.cs::AGUIContentTest.ToolCallResultEvent_Parts_RoundTrip
func TestCsAGUIContentTest_ToolCallResultEvent_Parts_RoundTrip(t *testing.T) {
	evt := &agui.ToolCallResultEvent{
		MessageID:  "m2",
		ToolCallID: "c1",
		Content:    agui.PartsContent(agui.NewTextPart("a"), agui.NewTextPart("b")),
	}
	o := obj(t, evt)
	if n := len(arrOf(t, o["content"])); n != 2 {
		t.Fatalf("expected 2 content parts, got %d", n)
	}
	back := roundTripEvent(t, evt)
	typed, ok := back.(*agui.ToolCallResultEvent)
	if !ok {
		t.Fatalf("expected *agui.ToolCallResultEvent, got %T", back)
	}
	assertEq(t, len(typed.Content.Parts()), 2, "part count")
	assertEq(t, typed.Content.String(), "ab", "flattened text")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIContentTest.cs::AGUIContentTest.SingleTextPart_WithIdOrMetadata_IsWrittenAsArray_SoNothingIsLost
func TestCsAGUIContentTest_SingleTextPart_WithIdOrMetadata_IsWrittenAsArray_SoNothingIsLost(t *testing.T) {
	msg := &agui.ToolMessage{
		ID:         "t1",
		ToolCallID: "c1",
		Content:    agui.PartsContent(&agui.TextPart{ID: agui.Ptr("p1"), Text: "hit", Metadata: map[string]any{"title": "Source"}}),
	}
	data := mustJSON(t, msg)
	content := arrOf(t, objOf(t, decode(t, data))["content"])
	p0 := objOf(t, content[0])
	assertEq(t, p0["id"], "p1", "content[0].id")
	assertEq(t, objOf(t, p0["metadata"])["title"], "Source", "content[0].metadata.title")

	m, err := agui.UnmarshalMessage(data)
	if err != nil {
		t.Fatal(err)
	}
	parts := m.(*agui.ToolMessage).Content.Parts()
	if len(parts) != 1 {
		t.Fatalf("expected one part, got %v", parts)
	}
	part, ok := parts[0].(*agui.TextPart)
	if !ok {
		t.Fatalf("expected *agui.TextPart, got %T", parts[0])
	}
	assertEq(t, agui.Deref(part.ID), "p1", "part.id")
	assertEq(t, objOf(t, part.Metadata)["title"], "Source", "part.metadata.title")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIContentTest.cs::AGUIContentTest.SingleBareTextPart_StillCollapsesToAString
//
// Adapted: collapsing a single bare text part into a string is a .NET
// serializer choice; the spec allows content to be either a string or a part
// list and does not require the collapse. Go writes what it was given (a
// one-element list). What the spec does require holds: the document is valid
// and a string-only reader sees the same text.
func TestCsAGUIContentTest_SingleBareTextPart_StillCollapsesToAString(t *testing.T) {
	msg := &agui.ToolMessage{ID: "t1", ToolCallID: "c1", Content: agui.PartsContent(agui.NewTextPart("just text"))}
	data := mustJSON(t, msg)
	if err := agui.Validate("ToolMessage", data); err != nil {
		t.Fatalf("schema rejects the tool message: %v", err)
	}
	back := roundTripMessage(t, msg).(*agui.ToolMessage)
	assertEq(t, back.Content.String(), "just text", "string rendering")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIContentTest.cs::AGUIContentTest.ToString_FlattensToText_DroppingMedia
func TestCsAGUIContentTest_ToString_FlattensToText_DroppingMedia(t *testing.T) {
	c := agui.PartsContent(
		agui.NewTextPart("a"),
		agui.NewImagePart(&agui.URLSource{Value: "https://example.com/x.png"}),
		agui.NewTextPart("b"),
	)
	assertEq(t, c.String(), "ab", "flattened")
	assertEq(t, agui.TextContent("plain").String(), "plain", "string content")
	var zero agui.Content
	assertEq(t, zero.String(), "", "zero content")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIContentTest.cs::AGUIContentTest.UserMessage_FileSourceWithEveryField_RoundTrips
func TestCsAGUIContentTest_UserMessage_FileSourceWithEveryField_RoundTrips(t *testing.T) {
	msg := &agui.UserMessage{
		ID: "u1",
		Content: agui.PartsContent(agui.NewDocumentPart(&agui.FileSource{
			Value: "file-abc123", Provider: agui.Ptr("openai"), MimeType: agui.Ptr("application/pdf"),
		})),
	}
	data := mustJSON(t, msg)
	if err := agui.Validate("UserMessage", data); err != nil {
		t.Fatalf("schema rejects the message: %v", err)
	}
	src := objOf(t, objOf(t, arrOf(t, objOf(t, decode(t, data))["content"])[0])["source"])
	assertEq(t, src["type"], "file", "source.type")
	assertEq(t, src["value"], "file-abc123", "source.value")
	assertEq(t, src["provider"], "openai", "source.provider")
	assertEq(t, src["mimeType"], "application/pdf", "source.mimeType")

	m, err := agui.UnmarshalMessage(data)
	if err != nil {
		t.Fatal(err)
	}
	parts := m.(*agui.UserMessage).Content.Parts()
	if len(parts) != 1 {
		t.Fatalf("expected one part, got %v", parts)
	}
	doc, ok := parts[0].(*agui.DocumentPart)
	if !ok {
		t.Fatalf("expected *agui.DocumentPart, got %T", parts[0])
	}
	fs, ok := doc.Source.(*agui.FileSource)
	if !ok {
		t.Fatalf("expected *agui.FileSource, got %T", doc.Source)
	}
	assertEq(t, fs.Value, "file-abc123", "value")
	assertEq(t, agui.Deref(fs.Provider), "openai", "provider")
	assertEq(t, agui.Deref(fs.MimeType), "application/pdf", "mimeType")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIContentTest.cs::AGUIContentTest.UserMessage_FileSourceWithoutOptionals_OmitsThemAndRoundTrips
func TestCsAGUIContentTest_UserMessage_FileSourceWithoutOptionals_OmitsThemAndRoundTrips(t *testing.T) {
	msg := &agui.UserMessage{ID: "u1", Content: agui.PartsContent(agui.NewDocumentPart(&agui.FileSource{Value: "file-abc123"}))}
	data := mustJSON(t, msg)
	src := objOf(t, objOf(t, arrOf(t, objOf(t, decode(t, data))["content"])[0])["source"])
	assertEq(t, src["type"], "file", "source.type")
	assertEq(t, src["value"], "file-abc123", "source.value")
	assertOmitted(t, src, "provider", "mimeType")

	m, err := agui.UnmarshalMessage(data)
	if err != nil {
		t.Fatal(err)
	}
	fs, ok := m.(*agui.UserMessage).Content.Parts()[0].(*agui.DocumentPart).Source.(*agui.FileSource)
	if !ok {
		t.Fatal("expected a file source")
	}
	assertEq(t, fs.Value, "file-abc123", "value")
	if fs.Provider != nil || fs.MimeType != nil {
		t.Fatalf("absent optionals came back set: provider=%v mimeType=%v", fs.Provider, fs.MimeType)
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIContentTest.cs::AGUIContentTest.Source_WithoutValue_DeserializesToTheArmWithAnEmptyValue
//
// The typed parsing layer is as lenient as .NET's: a source without "value"
// decodes to its arm with an empty value. The spec makes "value" required on
// every source, so the strict schema check must reject the document — that is
// the layer a producer or the processing model uses.
func TestCsAGUIContentTest_Source_WithoutValue_DeserializesToTheArmWithAnEmptyValue(t *testing.T) {
	rows := []struct {
		sourceJSON string
		want       string // expected Go source type
	}{
		{`{ "type": "file" }`, "*agui.FileSource"},
		{`{ "type": "url" }`, "*agui.URLSource"},
		{`{ "type": "data" }`, "*agui.DataSource"},
	}
	for _, r := range rows {
		t.Run(r.sourceJSON, func(t *testing.T) {
			doc := []byte(`{ "id": "u1", "role": "user", "content": [{ "type": "document", "source": ` + r.sourceJSON + ` }] }`)
			m, err := agui.UnmarshalMessage(doc)
			if err != nil {
				t.Fatalf("typed decode failed: %v", err)
			}
			parts := m.(*agui.UserMessage).Content.Parts()
			if len(parts) != 1 {
				t.Fatalf("expected one part, got %v", parts)
			}
			src := parts[0].(*agui.DocumentPart).Source
			assertEq(t, reflect.TypeOf(src).String(), r.want, "source type")
			var value string
			switch s := src.(type) {
			case *agui.FileSource:
				value = s.Value
			case *agui.URLSource:
				value = s.Value
			case *agui.DataSource:
				value = s.Value
			default:
				value = "unexpected"
			}
			assertEq(t, value, "", "value")

			if err := agui.Validate("UserMessage", doc); err == nil {
				t.Fatal("schema must reject a source without its required value")
			}
		})
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIContentTest.cs::AGUIContentTest.Source_WithUnknownDiscriminator_Throws
func TestCsAGUIContentTest_Source_WithUnknownDiscriminator_Throws(t *testing.T) {
	doc := []byte(`{ "id": "u1", "role": "user", "content": [{ "type": "document", "source": { "type": "handle", "value": "x" } }] }`)
	if _, err := agui.UnmarshalMessage(doc); err == nil {
		t.Fatal("typed decode must fail on an unknown source discriminator")
	}
	if err := agui.Validate("UserMessage", doc); err == nil {
		t.Fatal("schema must reject an unknown source discriminator")
	}
}
