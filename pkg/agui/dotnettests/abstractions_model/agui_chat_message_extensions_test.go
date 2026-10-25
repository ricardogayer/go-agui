package abstractions_model_test

// Port of sdks/dotnet/tests/AGUI.Abstractions.UnitTests/AGUIChatMessageExtensionsTest.cs.
//
// The .NET tests convert between AG-UI messages and Microsoft.Extensions.AI
// (MEAI) ChatMessage/AIContent. Go has no MEAI. Where a test starts from an
// AG-UI message, its AG-UI-side claims (what the message looks like on the
// wire, which roles/parts/sources exist, what survives a round trip) are
// ported; tests that start from MEAI types or assert only MEAI-side mapping
// are skipped as N/A.

import (
	"encoding/json"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

const naMEAI = "N/A in Go: "

// C#: tests/AGUI.Abstractions.UnitTests/AGUIChatMessageExtensionsTest.cs::AGUIChatMessageExtensionsTest.AsChatMessages_UserMessageWithName_SetsAuthorName
func TestCsAGUIChatMessageExtensionsTest_AsChatMessages_UserMessageWithName_SetsAuthorName(t *testing.T) {
	msg := &agui.UserMessage{ID: "msg-1", Name: agui.Ptr("Alice"), Content: agui.PartsContent(agui.NewTextPart("Hello"))}
	o := obj(t, msg)
	assertEq(t, o["role"], "user", "role")
	assertEq(t, o["name"], "Alice", "name")
	back := roundTripMessage(t, msg).(*agui.UserMessage)
	assertEq(t, agui.Deref(back.Name), "Alice", "name after round trip")
	assertEq(t, back.MessageRole(), agui.RoleUser, "role after round trip")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIChatMessageExtensionsTest.cs::AGUIChatMessageExtensionsTest.AsChatMessages_UserMessageWithoutName_AuthorNameIsNull
func TestCsAGUIChatMessageExtensionsTest_AsChatMessages_UserMessageWithoutName_AuthorNameIsNull(t *testing.T) {
	msg := &agui.UserMessage{ID: "msg-1", Content: agui.PartsContent(agui.NewTextPart("Hello"))}
	assertOmitted(t, obj(t, msg), "name")
	if back := roundTripMessage(t, msg).(*agui.UserMessage); back.Name != nil {
		t.Fatalf("name must stay absent, got %q", *back.Name)
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIChatMessageExtensionsTest.cs::AGUIChatMessageExtensionsTest.AsAGUIMessages_UserWithAuthorName_SetsName
func TestCsAGUIChatMessageExtensionsTest_AsAGUIMessages_UserWithAuthorName_SetsName(t *testing.T) {
	t.Skip(naMEAI + "maps MEAI ChatMessage.AuthorName to AGUIUserMessage.Name; no MEAI in Go")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIChatMessageExtensionsTest.cs::AGUIChatMessageExtensionsTest.AsAGUIMessages_UserWithoutAuthorName_NameIsNull
func TestCsAGUIChatMessageExtensionsTest_AsAGUIMessages_UserWithoutAuthorName_NameIsNull(t *testing.T) {
	t.Skip(naMEAI + "maps MEAI ChatMessage without AuthorName; no MEAI in Go")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIChatMessageExtensionsTest.cs::AGUIChatMessageExtensionsTest.RoundTrip_UserMessageWithName_PreservesName
func TestCsAGUIChatMessageExtensionsTest_RoundTrip_UserMessageWithName_PreservesName(t *testing.T) {
	t.Skip(naMEAI + "ChatMessage -> AG-UI -> ChatMessage round trip of AuthorName/MessageId; the AG-UI wire round trip of name is covered by AsChatMessages_UserMessageWithName_SetsAuthorName")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIChatMessageExtensionsTest.cs::AGUIChatMessageExtensionsTest.AsChatMessages_ToolMessageWithToolCallId_CreatesFunctionResultContent
func TestCsAGUIChatMessageExtensionsTest_AsChatMessages_ToolMessageWithToolCallId_CreatesFunctionResultContent(t *testing.T) {
	msg := &agui.ToolMessage{ID: "tool-msg-1", ToolCallID: "tc_1", Content: agui.TextContent("72°F, sunny")}
	data := mustJSON(t, msg)
	if err := agui.Validate("ToolMessage", data); err != nil {
		t.Fatalf("schema rejects the tool message: %v", err)
	}
	o := objOf(t, decode(t, data))
	assertEq(t, o["role"], "tool", "role")
	assertEq(t, o["toolCallId"], "tc_1", "toolCallId")
	assertEq(t, o["content"], "72°F, sunny", "content")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIChatMessageExtensionsTest.cs::AGUIChatMessageExtensionsTest.AsAGUIMessages_ToolMessage_CreatesAGUIToolMessage
func TestCsAGUIChatMessageExtensionsTest_AsAGUIMessages_ToolMessage_CreatesAGUIToolMessage(t *testing.T) {
	t.Skip(naMEAI + "maps an MEAI FunctionResultContent to an AGUIToolMessage keyed on the call id; no MEAI in Go")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIChatMessageExtensionsTest.cs::AGUIChatMessageExtensionsTest.AsAGUIMessages_ToolMessageWithMultipleResults_EmitsOneMessagePerResult
func TestCsAGUIChatMessageExtensionsTest_AsAGUIMessages_ToolMessageWithMultipleResults_EmitsOneMessagePerResult(t *testing.T) {
	t.Skip(naMEAI + "splits a batched MEAI tool ChatMessage into AG-UI tool messages; no MEAI in Go")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIChatMessageExtensionsTest.cs::AGUIChatMessageExtensionsTest.AsChatMessages_MultipleToolMessages_MapToOneFunctionResultEach
//
// AG-UI side only: each tool result is its own tool message on the wire, and
// a list of them round-trips in order with its own toolCallId each.
func TestCsAGUIChatMessageExtensionsTest_AsChatMessages_MultipleToolMessages_MapToOneFunctionResultEach(t *testing.T) {
	msgs := agui.Messages{
		&agui.ToolMessage{ID: "call_weather", ToolCallID: "call_weather", Content: agui.TextContent("Paris: 22°C, sunny")},
		&agui.ToolMessage{ID: "call_time", ToolCallID: "call_time", Content: agui.TextContent("Asia/Tokyo: 2026-06-18 18:30")},
	}
	var back agui.Messages
	if err := json.Unmarshal(mustJSON(t, msgs), &back); err != nil {
		t.Fatal(err)
	}
	if len(back) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(back))
	}
	for i, want := range []string{"call_weather", "call_time"} {
		tm, ok := back[i].(*agui.ToolMessage)
		if !ok {
			t.Fatalf("message %d: expected *agui.ToolMessage, got %T", i, back[i])
		}
		assertEq(t, tm.ToolCallID, want, "toolCallId")
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIChatMessageExtensionsTest.cs::AGUIChatMessageExtensionsTest.AsAGUIMessages_ParallelToolResults_RoundTripPreservesBothResults
func TestCsAGUIChatMessageExtensionsTest_AsAGUIMessages_ParallelToolResults_RoundTripPreservesBothResults(t *testing.T) {
	t.Skip(naMEAI + "MEAI tool ChatMessage -> AG-UI -> MEAI round trip; no MEAI in Go")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIChatMessageExtensionsTest.cs::AGUIChatMessageExtensionsTest.AsAGUIMessages_ToolResultObjectContent_SerializesToJson
func TestCsAGUIChatMessageExtensionsTest_AsAGUIMessages_ToolResultObjectContent_SerializesToJson(t *testing.T) {
	t.Skip(naMEAI + "serialization of an MEAI FunctionResultContent object result into tool message text; no MEAI in Go")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIChatMessageExtensionsTest.cs::AGUIChatMessageExtensionsTest.AsChatMessages_AssistantMessageWithToolCalls_CreatesFunctionCallContent
func TestCsAGUIChatMessageExtensionsTest_AsChatMessages_AssistantMessageWithToolCalls_CreatesFunctionCallContent(t *testing.T) {
	msg := &agui.AssistantMessage{
		ID: "asst-1", Content: agui.Ptr(""),
		ToolCalls: []agui.ToolCall{agui.NewToolCall("tc_1", "get_weather", `{"city":"NYC"}`)},
	}
	data := mustJSON(t, msg)
	if err := agui.Validate("AssistantMessage", data); err != nil {
		t.Fatalf("schema rejects the assistant message: %v", err)
	}
	o := objOf(t, decode(t, data))
	assertEq(t, o["role"], "assistant", "role")
	calls := arrOf(t, o["toolCalls"])
	if len(calls) != 1 {
		t.Fatalf("expected one tool call, got %v", calls)
	}
	call := objOf(t, calls[0])
	assertEq(t, call["id"], "tc_1", "id")
	assertEq(t, call["type"], "function", "type")
	fn := objOf(t, call["function"])
	assertEq(t, fn["name"], "get_weather", "function.name")
	args, ok := fn["arguments"].(string)
	if !ok {
		t.Fatalf("arguments must be a string, got %T", fn["arguments"])
	}
	assertEq(t, objOf(t, decode(t, []byte(args)))["city"], "NYC", "arguments.city")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIChatMessageExtensionsTest.cs::AGUIChatMessageExtensionsTest.AsAGUIMessages_AssistantWithFunctionCallContent_CreatesToolCalls
func TestCsAGUIChatMessageExtensionsTest_AsAGUIMessages_AssistantWithFunctionCallContent_CreatesToolCalls(t *testing.T) {
	t.Skip(naMEAI + "maps MEAI FunctionCallContent to AG-UI tool calls; no MEAI in Go")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIChatMessageExtensionsTest.cs::AGUIChatMessageExtensionsTest.RoundTrip_MultiTurnToolCallConversation_PreservesStructure
func TestCsAGUIChatMessageExtensionsTest_RoundTrip_MultiTurnToolCallConversation_PreservesStructure(t *testing.T) {
	t.Skip(naMEAI + "MEAI conversation -> AG-UI -> MEAI round trip; no MEAI in Go")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIChatMessageExtensionsTest.cs::AGUIChatMessageExtensionsTest.RoundTrip_ToolMessageJsonSerialization_PreservesContent
func TestCsAGUIChatMessageExtensionsTest_RoundTrip_ToolMessageJsonSerialization_PreservesContent(t *testing.T) {
	var m agui.Message = &agui.ToolMessage{ID: "tool-1", ToolCallID: "tc_1", Content: agui.TextContent("plain string result")}
	back, ok := roundTripMessage(t, m).(*agui.ToolMessage)
	if !ok {
		t.Fatal("expected a tool message back")
	}
	assertEq(t, back.MessageRole(), agui.RoleTool, "role")
	assertEq(t, back.ToolCallID, "tc_1", "toolCallId")
	if back.Content.IsParts() {
		t.Fatal("string content came back as parts")
	}
	assertEq(t, back.Content.String(), "plain string result", "content")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIChatMessageExtensionsTest.cs::AGUIChatMessageExtensionsTest.RoundTrip_AssistantMessageWithToolCalls_JsonSerialization
func TestCsAGUIChatMessageExtensionsTest_RoundTrip_AssistantMessageWithToolCalls_JsonSerialization(t *testing.T) {
	var m agui.Message = &agui.AssistantMessage{
		ID: "asst-1", Content: agui.Ptr(""),
		ToolCalls: []agui.ToolCall{agui.NewToolCall("tc_1", "get_weather", `{"city":"NYC"}`)},
	}
	back, ok := roundTripMessage(t, m).(*agui.AssistantMessage)
	if !ok {
		t.Fatal("expected an assistant message back")
	}
	if len(back.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %+v", back.ToolCalls)
	}
	tc := back.ToolCalls[0]
	assertEq(t, tc.ID, "tc_1", "id")
	assertEq(t, tc.Function.Name, "get_weather", "function.name")
	assertEq(t, tc.Function.Arguments, `{"city":"NYC"}`, "function.arguments")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIChatMessageExtensionsTest.cs::AGUIChatMessageExtensionsTest.AsChatMessages_ToolMessageWithJsonContent_PreservesMessageId
//
// AG-UI side: the id survives, and JSON-looking content stays the string it
// was (the protocol never parses tool content).
func TestCsAGUIChatMessageExtensionsTest_AsChatMessages_ToolMessageWithJsonContent_PreservesMessageId(t *testing.T) {
	const content = `{"status":"success","value":42}`
	back := roundTripMessage(t, &agui.ToolMessage{ID: "msg1", Content: agui.TextContent(content), ToolCallID: "call_abc"})
	assertEq(t, back.MessageRole(), agui.RoleTool, "role")
	assertEq(t, back.MessageID(), "msg1", "id")
	tm := back.(*agui.ToolMessage)
	if tm.Content.IsParts() {
		t.Fatal("content came back as parts")
	}
	assertEq(t, tm.Content.String(), content, "content")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIChatMessageExtensionsTest.cs::AGUIChatMessageExtensionsTest.AsChatMessages_ToolMessageWithPlainTextContent_PreservesMessageId
func TestCsAGUIChatMessageExtensionsTest_AsChatMessages_ToolMessageWithPlainTextContent_PreservesMessageId(t *testing.T) {
	back := roundTripMessage(t, &agui.ToolMessage{ID: "tool-msg-1", Content: agui.TextContent("72°F, sunny"), ToolCallID: "tc_1"})
	assertEq(t, back.MessageRole(), agui.RoleTool, "role")
	assertEq(t, back.MessageID(), "tool-msg-1", "id")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIChatMessageExtensionsTest.cs::AGUIChatMessageExtensionsTest.RunAgentInput_CanonicalMultimodalContent_DeserializesAndMapsInOrder
//
// AG-UI side: the canonical multimodal run input is valid, is read by the
// producer (agui.ParseRunAgentInput) without warnings, and keeps every part —
// type, source, MIME type and metadata — in order. The mapping to MEAI
// TextContent/UriContent/DataContent and back is N/A.
func TestCsAGUIChatMessageExtensionsTest_RunAgentInput_CanonicalMultimodalContent_DeserializesAndMapsInOrder(t *testing.T) {
	body := []byte(`{
	  "threadId": "thread-1",
	  "runId": "run-1",
	  "messages": [
	    {
	      "id": "m1", "role": "user", "name": "Alice",
	      "content": [
	        { "type": "text", "text": "Compare these files." },
	        { "type": "image", "source": { "type": "url", "value": "https://example.com/image.png", "mimeType": "image/png" }, "metadata": { "detail": "high" } },
	        { "type": "audio", "source": { "type": "data", "value": "AQIDBA==", "mimeType": "audio/wav" } },
	        { "type": "video", "source": { "type": "url", "value": "https://example.com/video.mp4" } },
	        { "type": "document", "source": { "type": "data", "value": "JVBERg==", "mimeType": "application/pdf" },
	          "metadata": { "filename": "report.pdf", "providerHint": { "quality": "high" } } }
	      ]
	    }
	  ],
	  "context": []
	}`)
	if err := agui.Validate("RunAgentInput", body); err != nil {
		t.Fatalf("schema rejects the canonical input: %v", err)
	}
	in, warnings, err := agui.ParseRunAgentInput(body)
	if err != nil {
		t.Fatalf("ParseRunAgentInput: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	if len(in.Messages) != 1 {
		t.Fatalf("expected one message, got %d", len(in.Messages))
	}
	u, ok := in.Messages[0].(*agui.UserMessage)
	if !ok {
		t.Fatalf("expected *agui.UserMessage, got %T", in.Messages[0])
	}
	assertEq(t, u.ID, "m1", "id")
	assertEq(t, agui.Deref(u.Name), "Alice", "name")
	parts := u.Content.Parts()
	if len(parts) != 5 {
		t.Fatalf("expected 5 parts, got %d", len(parts))
	}
	assertEq(t, textOf(t, parts[0]), "Compare these files.", "parts[0]")

	img, ok := parts[1].(*agui.ImagePart)
	if !ok {
		t.Fatalf("parts[1]: %T", parts[1])
	}
	us := img.Source.(*agui.URLSource)
	assertEq(t, us.Value, "https://example.com/image.png", "image url")
	assertEq(t, agui.Deref(us.MimeType), "image/png", "image mime")
	assertEq(t, objOf(t, img.Metadata)["detail"], "high", "image metadata.detail")

	audio, ok := parts[2].(*agui.AudioPart)
	if !ok {
		t.Fatalf("parts[2]: %T", parts[2])
	}
	ds := audio.Source.(*agui.DataSource)
	assertEq(t, ds.MimeType, "audio/wav", "audio mime")
	assertEq(t, ds.Value, "AQIDBA==", "audio data")

	video, ok := parts[3].(*agui.VideoPart)
	if !ok {
		t.Fatalf("parts[3]: %T", parts[3])
	}
	vs := video.Source.(*agui.URLSource)
	assertEq(t, vs.Value, "https://example.com/video.mp4", "video url")
	if vs.MimeType != nil {
		t.Errorf("video mimeType must stay absent, got %q", *vs.MimeType)
	}

	doc, ok := parts[4].(*agui.DocumentPart)
	if !ok {
		t.Fatalf("parts[4]: %T", parts[4])
	}
	dds := doc.Source.(*agui.DataSource)
	assertEq(t, dds.MimeType, "application/pdf", "document mime")
	assertEq(t, dds.Value, "JVBERg==", "document data")
	md := objOf(t, doc.Metadata)
	assertEq(t, md["filename"], "report.pdf", "document metadata.filename")
	assertEq(t, objOf(t, md["providerHint"])["quality"], "high", "document metadata.providerHint.quality")

	// And the parts survive being written back out (the AG-UI half of the
	// .NET round trip).
	again := roundTripMessage(t, u).(*agui.UserMessage)
	assertEq(t, objOf(t, again.Content.Parts()[1].(*agui.ImagePart).Metadata)["detail"], "high", "image metadata after round trip")
	md2 := objOf(t, again.Content.Parts()[4].(*agui.DocumentPart).Metadata)
	assertEq(t, md2["filename"], "report.pdf", "document filename after round trip")
	assertEq(t, objOf(t, md2["providerHint"])["quality"], "high", "document providerHint after round trip")
}

func newMediaPart(t *testing.T, mediaType string, src agui.PartSource) agui.ContentPart {
	t.Helper()
	switch mediaType {
	case "image":
		return agui.NewImagePart(src)
	case "audio":
		return agui.NewAudioPart(src)
	case "video":
		return agui.NewVideoPart(src)
	case "document":
		return agui.NewDocumentPart(src)
	}
	t.Fatalf("unexpected media type %q", mediaType)
	return nil
}

func mediaSource(t *testing.T, p agui.ContentPart) agui.PartSource {
	t.Helper()
	switch x := p.(type) {
	case *agui.ImagePart:
		return x.Source
	case *agui.AudioPart:
		return x.Source
	case *agui.VideoPart:
		return x.Source
	case *agui.DocumentPart:
		return x.Source
	}
	t.Fatalf("not a media part: %T", p)
	return nil
}

func extensionFor(mediaType string) string {
	return map[string]string{"image": "png", "audio": "wav", "video": "mp4", "document": "pdf"}[mediaType]
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIChatMessageExtensionsTest.cs::AGUIChatMessageExtensionsTest.AsChatMessages_CanonicalMediaTypes_MapBothSourceTypes
//
// AG-UI side: every media part type accepts both a data and a url source;
// the message is valid and part type, source kind, value and MIME type
// survive the round trip.
func TestCsAGUIChatMessageExtensionsTest_AsChatMessages_CanonicalMediaTypes_MapBothSourceTypes(t *testing.T) {
	rows := []struct{ mediaType, sourceType, mimeType string }{
		{"image", "url", "image/png"}, {"image", "data", "image/png"},
		{"audio", "url", "audio/wav"}, {"audio", "data", "audio/wav"},
		{"video", "url", "video/mp4"}, {"video", "data", "video/mp4"},
		{"document", "url", "application/pdf"}, {"document", "data", "application/pdf"},
	}
	for _, r := range rows {
		t.Run(r.mediaType+"/"+r.sourceType, func(t *testing.T) {
			url := "https://example.com/content." + extensionFor(r.mediaType)
			var src agui.PartSource
			if r.sourceType == "data" {
				src = &agui.DataSource{Value: "AQIDBA==", MimeType: r.mimeType}
			} else {
				src = &agui.URLSource{Value: url, MimeType: agui.Ptr(r.mimeType)}
			}
			msg := &agui.UserMessage{ID: "message-1", Content: agui.PartsContent(newMediaPart(t, r.mediaType, src))}
			data := mustJSON(t, msg)
			if err := agui.Validate("UserMessage", data); err != nil {
				t.Fatalf("schema rejects the message: %v", err)
			}
			back, err := agui.UnmarshalMessage(data)
			if err != nil {
				t.Fatal(err)
			}
			parts := back.(*agui.UserMessage).Content.Parts()
			if len(parts) != 1 {
				t.Fatalf("expected one part, got %v", parts)
			}
			assertEq(t, parts[0].PartType(), r.mediaType, "part type")
			switch s := mediaSource(t, parts[0]).(type) {
			case *agui.DataSource:
				if r.sourceType != "data" {
					t.Fatalf("got a data source for a %s row", r.sourceType)
				}
				assertEq(t, s.MimeType, r.mimeType, "mimeType")
				assertEq(t, s.Value, "AQIDBA==", "value")
			case *agui.URLSource:
				if r.sourceType != "url" {
					t.Fatalf("got a url source for a %s row", r.sourceType)
				}
				assertEq(t, agui.Deref(s.MimeType), r.mimeType, "mimeType")
				assertEq(t, s.Value, url, "value")
			default:
				t.Fatalf("unexpected source %T", s)
			}
		})
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIChatMessageExtensionsTest.cs::AGUIChatMessageExtensionsTest.AsChatMessages_CanonicalUrlWithoutMimeTypeOrExtension_PreservesModality
//
// AG-UI side: a url source without mimeType is valid, mimeType stays absent
// (no SDK invents one on the wire) and the part type still carries the
// modality. The MEAI wildcard media types ("image/*", ...) are N/A.
func TestCsAGUIChatMessageExtensionsTest_AsChatMessages_CanonicalUrlWithoutMimeTypeOrExtension_PreservesModality(t *testing.T) {
	rows := []struct{ mediaType, meaiMimeType string }{
		{"image", "image/*"}, {"audio", "audio/*"}, {"video", "video/*"}, {"document", "application/octet-stream"},
	}
	for _, r := range rows {
		t.Run(r.mediaType, func(t *testing.T) {
			msg := &agui.UserMessage{ID: "m", Content: agui.PartsContent(newMediaPart(t, r.mediaType, &agui.URLSource{Value: "https://example.com/content"}))}
			data := mustJSON(t, msg)
			if err := agui.Validate("UserMessage", data); err != nil {
				t.Fatalf("schema rejects the message: %v", err)
			}
			src := objOf(t, objOf(t, arrOf(t, objOf(t, decode(t, data))["content"])[0])["source"])
			assertOmitted(t, src, "mimeType")
			back, err := agui.UnmarshalMessage(data)
			if err != nil {
				t.Fatal(err)
			}
			part := back.(*agui.UserMessage).Content.Parts()[0]
			assertEq(t, part.PartType(), r.mediaType, "modality (part type)")
		})
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIChatMessageExtensionsTest.cs::AGUIChatMessageExtensionsTest.AsChatMessages_CanonicalMediaWithNonObjectMetadata_PreservesMetadataValue
//
// AG-UI side: media part metadata is unconstrained (any JSON value but null),
// so a string is valid and survives.
func TestCsAGUIChatMessageExtensionsTest_AsChatMessages_CanonicalMediaWithNonObjectMetadata_PreservesMetadataValue(t *testing.T) {
	img := agui.NewImagePart(&agui.DataSource{Value: "AQIDBA==", MimeType: "image/png"})
	img.Metadata = "provider-specific"
	msg := &agui.UserMessage{ID: "m", Content: agui.PartsContent(img)}
	data := mustJSON(t, msg)
	if err := agui.Validate("UserMessage", data); err != nil {
		t.Fatalf("schema rejects non-object part metadata: %v", err)
	}
	back, err := agui.UnmarshalMessage(data)
	if err != nil {
		t.Fatal(err)
	}
	assertEq(t, back.(*agui.UserMessage).Content.Parts()[0].(*agui.ImagePart).Metadata, "provider-specific", "metadata")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIChatMessageExtensionsTest.cs::AGUIChatMessageExtensionsTest.AsAGUIMessages_DataContent_MapsAdditionalPropertiesToCanonicalMetadata
func TestCsAGUIChatMessageExtensionsTest_AsAGUIMessages_DataContent_MapsAdditionalPropertiesToCanonicalMetadata(t *testing.T) {
	for _, mime := range []string{"image/png", "IMAGE/PNG", "audio/wav", "video/mp4", "video/mp4; codecs=avc1", "application/pdf"} {
		t.Run(mime, func(t *testing.T) {
			t.Skip(naMEAI + "maps an MEAI DataContent (MIME type, Name, AdditionalProperties) to an AG-UI media part; no MEAI in Go")
		})
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIChatMessageExtensionsTest.cs::AGUIChatMessageExtensionsTest.AsAGUIMessages_DataContentWithPreservedMetadata_MergesNameAsFilename
func TestCsAGUIChatMessageExtensionsTest_AsAGUIMessages_DataContentWithPreservedMetadata_MergesNameAsFilename(t *testing.T) {
	t.Skip(naMEAI + "merges MEAI DataContent.Name into preserved metadata; no MEAI in Go")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIChatMessageExtensionsTest.cs::AGUIChatMessageExtensionsTest.AsAGUIMessages_DataContentWithPreservedFilename_DoesNotOverwriteFilename
func TestCsAGUIChatMessageExtensionsTest_AsAGUIMessages_DataContentWithPreservedFilename_DoesNotOverwriteFilename(t *testing.T) {
	t.Skip(naMEAI + "MEAI DataContent.Name vs preserved metadata filename precedence; no MEAI in Go")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIChatMessageExtensionsTest.cs::AGUIChatMessageExtensionsTest.AsAGUIMessages_UriContent_MapsAdditionalPropertiesToCanonicalMetadata
func TestCsAGUIChatMessageExtensionsTest_AsAGUIMessages_UriContent_MapsAdditionalPropertiesToCanonicalMetadata(t *testing.T) {
	t.Skip(naMEAI + "maps MEAI UriContent.AdditionalProperties to AG-UI part metadata; no MEAI in Go")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIChatMessageExtensionsTest.cs::AGUIChatMessageExtensionsTest.AsChatMessages_ParallelToolCalls_CoalescedIntoSingleAssistantMessage
func TestCsAGUIChatMessageExtensionsTest_AsChatMessages_ParallelToolCalls_CoalescedIntoSingleAssistantMessage(t *testing.T) {
	t.Skip(naMEAI + "coalescing of split parallel tool calls into one MEAI assistant ChatMessage (provider history shape); no MEAI in Go")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIChatMessageExtensionsTest.cs::AGUIChatMessageExtensionsTest.DeveloperMessage_RoundTripsThroughChatMessageAndBack
//
// AG-UI side: a developer message keeps its role, id and content through the
// wire. The MEAI ChatRole("developer") leg is N/A.
func TestCsAGUIChatMessageExtensionsTest_DeveloperMessage_RoundTripsThroughChatMessageAndBack(t *testing.T) {
	msg := &agui.DeveloperMessage{ID: "dev-1", Content: "follow the rules"}
	data := mustJSON(t, msg)
	if err := agui.Validate("DeveloperMessage", data); err != nil {
		t.Fatalf("schema rejects the developer message: %v", err)
	}
	assertEq(t, objOf(t, decode(t, data))["role"], "developer", "role")
	back, ok := roundTripMessage(t, msg).(*agui.DeveloperMessage)
	if !ok {
		t.Fatal("expected a developer message back")
	}
	assertEq(t, back.ID, "dev-1", "id")
	assertEq(t, back.Content, "follow the rules", "content")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIChatMessageExtensionsTest.cs::AGUIChatMessageExtensionsTest.AsChatMessages_ParallelToolCalls_FollowedByResults_ProducesValidOrder
func TestCsAGUIChatMessageExtensionsTest_AsChatMessages_ParallelToolCalls_FollowedByResults_ProducesValidOrder(t *testing.T) {
	t.Skip(naMEAI + "reorders/merges AG-UI messages into a provider-valid MEAI history; no MEAI in Go")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIChatMessageExtensionsTest.cs::AGUIChatMessageExtensionsTest.AsChatMessages_SequentialToolCalls_NotMerged
func TestCsAGUIChatMessageExtensionsTest_AsChatMessages_SequentialToolCalls_NotMerged(t *testing.T) {
	t.Skip(naMEAI + "MEAI history merging rule for sequential tool calls; no MEAI in Go")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIChatMessageExtensionsTest.cs::AGUIChatMessageExtensionsTest.AsChatMessages_FileSource_MapsToHostedFileContent
//
// AG-UI side: a file source with value, provider and mimeType is valid and
// the handle and MIME type survive. HostedFileContent is N/A.
func TestCsAGUIChatMessageExtensionsTest_AsChatMessages_FileSource_MapsToHostedFileContent(t *testing.T) {
	msg := &agui.UserMessage{ID: "message-1", Content: agui.PartsContent(agui.NewDocumentPart(&agui.FileSource{
		Value: "file-abc123", Provider: agui.Ptr("openai"), MimeType: agui.Ptr("application/pdf"),
	}))}
	data := mustJSON(t, msg)
	if err := agui.Validate("UserMessage", data); err != nil {
		t.Fatalf("schema rejects the message: %v", err)
	}
	back, err := agui.UnmarshalMessage(data)
	if err != nil {
		t.Fatal(err)
	}
	fs := back.(*agui.UserMessage).Content.Parts()[0].(*agui.DocumentPart).Source.(*agui.FileSource)
	assertEq(t, fs.Value, "file-abc123", "file id")
	assertEq(t, agui.Deref(fs.MimeType), "application/pdf", "mimeType")
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIChatMessageExtensionsTest.cs::AGUIChatMessageExtensionsTest.AsChatMessages_FileSourceWithoutMimeType_LeavesMediaTypeUnset
func TestCsAGUIChatMessageExtensionsTest_AsChatMessages_FileSourceWithoutMimeType_LeavesMediaTypeUnset(t *testing.T) {
	msg := &agui.UserMessage{ID: "message-1", Content: agui.PartsContent(agui.NewDocumentPart(&agui.FileSource{Value: "file-abc123"}))}
	data := mustJSON(t, msg)
	if err := agui.Validate("UserMessage", data); err != nil {
		t.Fatalf("schema rejects the message: %v", err)
	}
	back, err := agui.UnmarshalMessage(data)
	if err != nil {
		t.Fatal(err)
	}
	fs := back.(*agui.UserMessage).Content.Parts()[0].(*agui.DocumentPart).Source.(*agui.FileSource)
	assertEq(t, fs.Value, "file-abc123", "file id")
	if fs.MimeType != nil {
		t.Fatalf("mimeType must stay unset, got %q", *fs.MimeType)
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIChatMessageExtensionsTest.cs::AGUIChatMessageExtensionsTest.AsAGUIMessages_HostedFileContent_MapsToFileSource
func TestCsAGUIChatMessageExtensionsTest_AsAGUIMessages_HostedFileContent_MapsToFileSource(t *testing.T) {
	for _, mime := range []string{"image/png", "audio/wav", "video/mp4", "application/pdf", "<null>"} {
		t.Run(mime, func(t *testing.T) {
			t.Skip(naMEAI + "maps an MEAI HostedFileContent to an AG-UI media part with a file source; no MEAI in Go")
		})
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/AGUIChatMessageExtensionsTest.cs::AGUIChatMessageExtensionsTest.HostedFileContent_RoundTripsThroughAGUIAndBack
func TestCsAGUIChatMessageExtensionsTest_HostedFileContent_RoundTripsThroughAGUIAndBack(t *testing.T) {
	t.Skip(naMEAI + "MEAI HostedFileContent -> AG-UI -> MEAI round trip; the AG-UI file source round trip is covered by AsChatMessages_FileSource_MapsToHostedFileContent")
}
