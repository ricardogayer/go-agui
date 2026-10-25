package types_test

// Port of sdks/python/tests/test_types.py.
//
// Pydantic construction becomes a Go struct literal (or a decode of the
// camelCase JSON); model_dump(by_alias=True) becomes json.Marshal compared as
// decoded JSON; an expected ValidationError becomes a rejection by the strict
// schema check, agui.Validate.

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

func mustUnmarshalMessage(t *testing.T, doc string) agui.Message {
	t.Helper()
	m, err := agui.UnmarshalMessage([]byte(doc))
	if err != nil {
		t.Fatalf("UnmarshalMessage(%s): %v", doc, err)
	}
	return m
}

func eq(t *testing.T, what string, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %#v, want %#v", what, got, want)
	}
}

func TestPyTestBaseTypes_test_function_call_creation(t *testing.T) {
	// Python: tests/test_types.py::TestBaseTypes::test_function_call_creation
	fc := agui.FunctionCall{Name: "test_function", Arguments: "{}"}
	eq(t, "name", fc.Name, "test_function")
	eq(t, "arguments", fc.Arguments, "{}")
}

func TestPyTestBaseTypes_test_message_serialization(t *testing.T) {
	// Python: tests/test_types.py::TestBaseTypes::test_message_serialization
	s := marshalObj(t, agui.UserMessage{ID: "msg_123", Content: agui.TextContent("Hello, world!")})
	eq(t, "id", s["id"], "msg_123")
	eq(t, "role", s["role"], "user")
	eq(t, "content", s["content"], "Hello, world!")
}

func TestPyTestBaseTypes_test_tool_call_serialization(t *testing.T) {
	// Python: tests/test_types.py::TestBaseTypes::test_tool_call_serialization
	s := marshalObj(t, agui.ToolCall{ID: "call_123", Function: agui.FunctionCall{Name: "test_function", Arguments: "{}"}})
	if _, ok := s["function"]; !ok {
		t.Errorf("function missing from %v", s)
	}
}

func TestPyTestBaseTypes_test_tool_message_camel_case(t *testing.T) {
	// Python: tests/test_types.py::TestBaseTypes::test_tool_message_camel_case
	s := marshalObj(t, agui.ToolMessage{ID: "tool_123", Content: agui.TextContent("Tool result"), ToolCallID: "call_456"})
	if _, ok := s["toolCallId"]; !ok {
		t.Fatalf("toolCallId missing from %v", s)
	}
	eq(t, "toolCallId", s["toolCallId"], "call_456")
}

func TestPyTestBaseTypes_test_activity_message(t *testing.T) {
	// Python: tests/test_types.py::TestBaseTypes::test_activity_message
	content := map[string]any{"steps": []any{"search", "summarize"}}
	msg := agui.ActivityMessage{ID: "activity_123", ActivityType: "PLAN", Content: content}
	eq(t, "role", msg.MessageRole(), agui.RoleActivity)
	eq(t, "activityType", msg.ActivityType, "PLAN")
	eq(t, "content", msg.Content, content)

	s := marshalObj(t, msg)
	eq(t, "role", s["role"], "activity")
	eq(t, "activityType", s["activityType"], "PLAN")
	eq(t, "content", s["content"], content)
}

func TestPyTestBaseTypes_test_parse_camel_case_json_tool_message(t *testing.T) {
	// Python: tests/test_types.py::TestBaseTypes::test_parse_camel_case_json_tool_message
	var msg agui.ToolMessage
	if err := json.Unmarshal([]byte(`{"id":"tool_789","role":"tool","content":"Result from tool","toolCallId":"call_123"}`), &msg); err != nil {
		t.Fatal(err)
	}
	eq(t, "id", msg.ID, "tool_789")
	eq(t, "role", msg.MessageRole(), agui.RoleTool)
	eq(t, "content", msg.Content.String(), "Result from tool")
	if msg.Content.IsParts() {
		t.Error("string content decoded as parts")
	}
	eq(t, "toolCallId", msg.ToolCallID, "call_123")
}

func TestPyTestBaseTypes_test_parse_camel_case_json_function_call(t *testing.T) {
	// Python: tests/test_types.py::TestBaseTypes::test_parse_camel_case_json_function_call
	doc := []byte(`{"id":"call_abc","type":"function","function":{"name":"get_weather","arguments":"{\"location\":\"New York\"}"}}`)
	var tc agui.ToolCall
	if err := json.Unmarshal(doc, &tc); err != nil {
		t.Fatal(err)
	}
	eq(t, "id", tc.ID, "call_abc")
	// The Go type has no type field: the discriminator is fixed, and is
	// written back on marshal.
	eq(t, "type", marshalObj(t, tc)["type"], "function")
	eq(t, "function.name", tc.Function.Name, "get_weather")
	eq(t, "function.arguments", tc.Function.Arguments, `{"location":"New York"}`)
}

func TestPyTestBaseTypes_test_developer_message(t *testing.T) {
	// Python: tests/test_types.py::TestBaseTypes::test_developer_message
	s := marshalObj(t, agui.DeveloperMessage{ID: "dev_123", Content: "Developer note"})
	eq(t, "role", s["role"], "developer")
	eq(t, "content", s["content"], "Developer note")
}

func TestPyTestBaseTypes_test_system_message(t *testing.T) {
	// Python: tests/test_types.py::TestBaseTypes::test_system_message
	s := marshalObj(t, agui.SystemMessage{ID: "sys_123", Content: "System instruction"})
	eq(t, "role", s["role"], "system")
	eq(t, "content", s["content"], "System instruction")
}

func TestPyTestBaseTypes_test_assistant_message(t *testing.T) {
	// Python: tests/test_types.py::TestBaseTypes::test_assistant_message
	msg := agui.AssistantMessage{
		ID:        "asst_123",
		Content:   agui.Ptr("Assistant response"),
		ToolCalls: []agui.ToolCall{agui.NewToolCall("call_456", "get_data", `{"param": "value"}`)},
	}
	s := marshalObj(t, msg)
	eq(t, "role", s["role"], "assistant")
	eq(t, "content", s["content"], "Assistant response")
	calls, _ := s["toolCalls"].([]any)
	if len(calls) != 1 {
		t.Fatalf("toolCalls = %v", s["toolCalls"])
	}
	eq(t, "toolCalls[0].id", calls[0].(map[string]any)["id"], "call_456")
}

func TestPyTestBaseTypes_test_user_message(t *testing.T) {
	// Python: tests/test_types.py::TestBaseTypes::test_user_message
	s := marshalObj(t, agui.UserMessage{ID: "user_123", Content: agui.TextContent("User query")})
	eq(t, "role", s["role"], "user")
	eq(t, "content", s["content"], "User query")
}

func TestPyTestBaseTypes_test_user_message_multimodal_content(t *testing.T) {
	// Python: tests/test_types.py::TestBaseTypes::test_user_message_multimodal_content
	msg := agui.UserMessage{
		ID: "user_multi",
		Content: agui.PartsContent(
			agui.NewTextPart("Check this out"),
			agui.NewImagePart(&agui.URLSource{Value: "https://example.com/image.png", MimeType: agui.Ptr("image/png")}),
		),
	}
	if !msg.Content.IsParts() || len(msg.Content.Parts()) != 2 {
		t.Fatalf("content = %#v", msg.Content)
	}
	s := marshalObj(t, msg)
	parts, ok := s["content"].([]any)
	if !ok {
		t.Fatalf("content is %T", s["content"])
	}
	p0, p1 := parts[0].(map[string]any), parts[1].(map[string]any)
	eq(t, "content[0].type", p0["type"], "text")
	eq(t, "content[0].text", p0["text"], "Check this out")
	eq(t, "content[1].type", p1["type"], "image")
	src := p1["source"].(map[string]any)
	eq(t, "content[1].source.type", src["type"], "url")
	eq(t, "content[1].source.mimeType", src["mimeType"], "image/png")
}

func TestPyTestBaseTypes_test_user_message_multimodal_data_source(t *testing.T) {
	// Python: tests/test_types.py::TestBaseTypes::test_user_message_multimodal_data_source
	msg := agui.UserMessage{
		ID:      "user_multi_data",
		Content: agui.PartsContent(agui.NewAudioPart(&agui.DataSource{Value: "UklGRmQAAABXQVZF", MimeType: "audio/wav"})),
	}
	p0 := marshalObj(t, msg)["content"].([]any)[0].(map[string]any)
	eq(t, "content[0].type", p0["type"], "audio")
	src := p0["source"].(map[string]any)
	eq(t, "source.type", src["type"], "data")
	eq(t, "source.mimeType", src["mimeType"], "audio/wav")
}

func TestPyTestBaseTypes_test_user_message_multimodal_file_source(t *testing.T) {
	// Python: tests/test_types.py::TestBaseTypes::test_user_message_multimodal_file_source
	msg := agui.UserMessage{
		ID: "user_multi_file",
		Content: agui.PartsContent(agui.NewDocumentPart(&agui.FileSource{
			Value: "file-abc123", Provider: agui.Ptr("openai"), MimeType: agui.Ptr("application/pdf"),
		})),
	}
	p0 := marshalObj(t, msg)["content"].([]any)[0].(map[string]any)
	eq(t, "content[0].type", p0["type"], "document")
	eq(t, "content[0].source", p0["source"], map[string]any{
		"type": "file", "value": "file-abc123", "provider": "openai", "mimeType": "application/pdf",
	})
}

func TestPyTestBaseTypes_test_document_part_with_metadata(t *testing.T) {
	// Python: tests/test_types.py::TestBaseTypes::test_document_part_with_metadata
	part := agui.NewDocumentPart(&agui.URLSource{Value: "https://example.com/report.pdf", MimeType: agui.Ptr("application/pdf")})
	part.Metadata = map[string]any{"provider": "anthropic", "media_type": "application/pdf"}
	msg := agui.UserMessage{ID: "user_doc", Content: agui.PartsContent(part)}
	p0 := marshalObj(t, msg)["content"].([]any)[0].(map[string]any)
	eq(t, "content[0].type", p0["type"], "document")
	eq(t, "content[0].metadata.provider", p0["metadata"].(map[string]any)["provider"], "anthropic")
}

func TestPyTestBaseTypes_test_message_union_deserialization(t *testing.T) {
	// Python: tests/test_types.py::TestBaseTypes::test_message_union_deserialization
	cases := []struct {
		doc      string
		wantType any
		id       string
		role     agui.Role
		content  any
	}{
		{`{"id":"dev_123","role":"developer","content":"Developer note"}`, &agui.DeveloperMessage{}, "dev_123", agui.RoleDeveloper, "Developer note"},
		{`{"id":"sys_456","role":"system","content":"System instruction"}`, &agui.SystemMessage{}, "sys_456", agui.RoleSystem, "System instruction"},
		{`{"id":"asst_789","role":"assistant","content":"Assistant response"}`, &agui.AssistantMessage{}, "asst_789", agui.RoleAssistant, "Assistant response"},
		{`{"id":"user_101","role":"user","content":"User query"}`, &agui.UserMessage{}, "user_101", agui.RoleUser, "User query"},
		{`{"id":"tool_202","role":"tool","content":"Tool result","toolCallId":"call_303"}`, &agui.ToolMessage{}, "tool_202", agui.RoleTool, "Tool result"},
		{`{"id":"activity_404","role":"activity","activityType":"PLAN","content":{"steps":[]}}`, &agui.ActivityMessage{}, "activity_404", agui.RoleActivity, map[string]any{"steps": []any{}}},
	}
	for _, c := range cases {
		m := mustUnmarshalMessage(t, c.doc)
		if reflect.TypeOf(m) != reflect.TypeOf(c.wantType) {
			t.Errorf("%s decoded to %T, want %T", c.doc, m, c.wantType)
			continue
		}
		eq(t, "id", m.MessageID(), c.id)
		eq(t, "role", m.MessageRole(), c.role)
		var content any
		switch v := m.(type) {
		case *agui.DeveloperMessage:
			content = v.Content
		case *agui.SystemMessage:
			content = v.Content
		case *agui.AssistantMessage:
			content = agui.Deref(v.Content)
		case *agui.UserMessage:
			content = v.Content.String()
		case *agui.ToolMessage:
			content = v.Content.String()
		case *agui.ActivityMessage:
			content = v.Content
		}
		eq(t, "content", content, c.content)
	}
}

func TestPyTestBaseTypes_test_message_union_with_tool_calls(t *testing.T) {
	// Python: tests/test_types.py::TestBaseTypes::test_message_union_with_tool_calls
	m := mustUnmarshalMessage(t, `{"id":"asst_123","role":"assistant","content":"I'll help with that",
		"toolCalls":[{"id":"call_456","type":"function","function":{"name":"search_data","arguments":"{\"query\": \"python\"}"}}]}`)
	a, ok := m.(*agui.AssistantMessage)
	if !ok {
		t.Fatalf("decoded %T", m)
	}
	if len(a.ToolCalls) != 1 {
		t.Fatalf("%d tool calls", len(a.ToolCalls))
	}
	eq(t, "function.name", a.ToolCalls[0].Function.Name, "search_data")
}

const runAgentInputDoc = `{
  "threadId": "thread_12345",
  "runId": "run_67890",
  "parentRunId": "run_parent_123",
  "state": {"conversation_state": "active", "custom_data": {"key": "value"}},
  "messages": [
    {"id": "sys_001", "role": "system", "content": "You are a helpful assistant."},
    {"id": "user_001", "role": "user", "content": "Can you help me analyze this data?"},
    {"id": "dev_001", "role": "developer", "content": "The assistant should provide a detailed analysis."},
    {"id": "asst_001", "role": "assistant", "content": "I'll analyze the data for you.",
     "toolCalls": [{"id": "call_001", "type": "function",
       "function": {"name": "analyze_data", "arguments": "{\"dataset\": \"sales_2023\", \"metrics\": [\"mean\", \"median\"]}"}}]},
    {"id": "tool_001", "role": "tool", "content": "{\"mean\": 42.5, \"median\": 38.0}", "toolCallId": "call_001"},
    {"id": "user_002", "role": "user", "content": [
      {"type": "text", "text": "Can you explain these results?"},
      {"type": "image", "source": {"type": "url", "value": "https://example.com/results-chart.png", "mimeType": "image/png"}}
    ]}
  ],
  "tools": [
    {"name": "analyze_data", "description": "Analyze a dataset and return statistics",
     "parameters": {"type": "object", "properties": {"dataset": {"type": "string"},
       "metrics": {"type": "array", "items": {"type": "string"}}}, "required": ["dataset"]}},
    {"name": "fetch_data", "description": "Fetch data from a database",
     "parameters": {"type": "object", "properties": {"source": {"type": "string"}, "query": {"type": "string"}},
       "required": ["source", "query"]}}
  ],
  "context": [
    {"description": "User preferences", "value": "{\"theme\": \"dark\", \"language\": \"English\"}"},
    {"description": "Environment", "value": "production"}
  ],
  "forwardedProps": {"api_version": "v1", "custom_settings": {"max_tokens": 500}}
}`

func TestPyTestBaseTypes_test_run_agent_input_deserialization(t *testing.T) {
	// Python: tests/test_types.py::TestBaseTypes::test_run_agent_input_deserialization
	var in agui.RunAgentInput
	if err := json.Unmarshal([]byte(runAgentInputDoc), &in); err != nil {
		t.Fatal(err)
	}
	// The document is spec-conformant, so the strict check accepts it too.
	if err := agui.Validate("RunAgentInput", []byte(runAgentInputDoc)); err != nil {
		t.Errorf("Validate: %v", err)
	}
	eq(t, "threadId", in.ThreadID, "thread_12345")
	eq(t, "runId", in.RunID, "run_67890")
	eq(t, "parentRunId", agui.Deref(in.ParentRunID), "run_parent_123")
	eq(t, "state.conversation_state", in.State.(map[string]any)["conversation_state"], "active")

	if len(in.Messages) != 6 {
		t.Fatalf("%d messages", len(in.Messages))
	}
	wantTypes := []agui.Message{&agui.SystemMessage{}, &agui.UserMessage{}, &agui.DeveloperMessage{}, &agui.AssistantMessage{}, &agui.ToolMessage{}, &agui.UserMessage{}}
	for i, w := range wantTypes {
		if reflect.TypeOf(in.Messages[i]) != reflect.TypeOf(w) {
			t.Errorf("messages[%d] is %T, want %T", i, in.Messages[i], w)
		}
	}
	eq(t, "messages[0].content", in.Messages[0].(*agui.SystemMessage).Content, "You are a helpful assistant.")
	eq(t, "messages[1].content", in.Messages[1].(*agui.UserMessage).Content.String(), "Can you help me analyze this data?")
	mm := in.Messages[5].(*agui.UserMessage).Content
	if !mm.IsParts() {
		t.Fatal("multimodal content is not a list")
	}
	parts := mm.Parts()
	eq(t, "parts[0].type", parts[0].PartType(), "text")
	eq(t, "parts[0].text", parts[0].(*agui.TextPart).Text, "Can you explain these results?")
	eq(t, "parts[1].type", parts[1].PartType(), "image")
	img := parts[1].(*agui.ImagePart)
	eq(t, "parts[1].source.type", img.Source.SourceType(), "url")
	eq(t, "parts[1].source.value", img.Source.(*agui.URLSource).Value, "https://example.com/results-chart.png")

	asst := in.Messages[3].(*agui.AssistantMessage)
	if len(asst.ToolCalls) != 1 {
		t.Fatalf("%d tool calls", len(asst.ToolCalls))
	}
	eq(t, "toolCalls[0].function.name", asst.ToolCalls[0].Function.Name, "analyze_data")

	tool := in.Messages[4].(*agui.ToolMessage)
	eq(t, "toolCallId", tool.ToolCallID, "call_001")
	eq(t, "tool content", tool.Content.String(), `{"mean": 42.5, "median": 38.0}`)

	if len(in.Tools) != 2 {
		t.Fatalf("%d tools", len(in.Tools))
	}
	eq(t, "tools[0].name", in.Tools[0].Name, "analyze_data")
	eq(t, "tools[1].name", in.Tools[1].Name, "fetch_data")

	if len(in.Context) != 2 {
		t.Fatalf("%d context entries", len(in.Context))
	}
	eq(t, "context[0].description", in.Context[0].Description, "User preferences")
	eq(t, "context[1].value", in.Context[1].Value, "production")

	fp := in.ForwardedProps.(map[string]any)
	eq(t, "forwardedProps.api_version", fp["api_version"], "v1")
	eq(t, "forwardedProps.custom_settings.max_tokens", fp["custom_settings"].(map[string]any)["max_tokens"], float64(500))
}

func TestPyTestBaseTypes_test_validation_errors(t *testing.T) {
	// Python: tests/test_types.py::TestBaseTypes::test_validation_errors
	invalidRole := []byte(`{"id":"msg_123","role":"invalid_role","content":"Hello"}`)
	if err := agui.Validate("Message", invalidRole); err == nil {
		t.Error("Validate accepted an invalid role")
	}
	if _, err := agui.UnmarshalMessage(invalidRole); err == nil {
		t.Error("UnmarshalMessage accepted an invalid role")
	}

	if err := agui.Validate("UserMessage", []byte(`{"role":"user","content":"Hello"}`)); err == nil {
		t.Error("Validate accepted a user message without id")
	}

	// Extra fields are tolerated for backwards compatibility. In Go the
	// tolerant layer is typed decoding (which ignores the member) and the
	// processing model is Sanitize (which strips it with a warning).
	extra := []byte(`{"id":"msg_456","role":"user","content":"Hello","extra_field":"This is allowed for backwards compatibility"}`)
	var msg agui.UserMessage
	if err := json.Unmarshal(extra, &msg); err != nil {
		t.Fatalf("typed decode rejected an extra field: %v", err)
	}
	eq(t, "id", msg.ID, "msg_456")
	eq(t, "content", msg.Content.String(), "Hello")
	res, err := agui.Sanitize("UserMessage", extra, agui.SanitizeOptions{})
	if err != nil {
		t.Fatalf("Sanitize rejected an extra field: %v", err)
	}
	if len(res.Warnings) == 0 || res.Warnings[0].Path != "/extra_field" {
		t.Errorf("Sanitize warnings = %v, want one for /extra_field", res.Warnings)
	}

	if err := agui.Validate("ToolMessage", []byte(`{"id":"tool_789","role":"tool","content":"Result"}`)); err == nil {
		t.Error("Validate accepted a tool message without toolCallId")
	}
}

func TestPyTestBaseTypes_test_empty_collections(t *testing.T) {
	// Python: tests/test_types.py::TestBaseTypes::test_empty_collections
	doc := []byte(`{"threadId":"thread_empty","runId":"run_empty","state":{},"messages":[],"tools":[],"context":[],"forwardedProps":{}}`)
	var in agui.RunAgentInput
	if err := json.Unmarshal(doc, &in); err != nil {
		t.Fatal(err)
	}
	eq(t, "threadId", in.ThreadID, "thread_empty")
	eq(t, "runId", in.RunID, "run_empty")
	eq(t, "len(messages)", len(in.Messages), 0)
	eq(t, "len(tools)", len(in.Tools), 0)
	eq(t, "len(context)", len(in.Context), 0)
	eq(t, "forwardedProps", in.ForwardedProps, map[string]any{})
}

func TestPyTestBaseTypes_test_multiple_tool_calls(t *testing.T) {
	// Python: tests/test_types.py::TestBaseTypes::test_multiple_tool_calls
	doc := []byte(`{"id":"asst_multi","role":"assistant","content":"I'll perform multiple operations","toolCalls":[
		{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"location\": \"New York\"}"}},
		{"id":"call_2","type":"function","function":{"name":"search_database","arguments":"{\"query\": \"recent sales\"}"}},
		{"id":"call_3","type":"function","function":{"name":"calculate","arguments":"{\"operation\": \"sum\", \"values\": [1, 2, 3, 4, 5]}"}}]}`)
	var a agui.AssistantMessage
	if err := json.Unmarshal(doc, &a); err != nil {
		t.Fatal(err)
	}
	eq(t, "id", a.ID, "asst_multi")
	if len(a.ToolCalls) != 3 {
		t.Fatalf("%d tool calls", len(a.ToolCalls))
	}
	for i, w := range []struct{ id, name string }{{"call_1", "get_weather"}, {"call_2", "search_database"}, {"call_3", "calculate"}} {
		eq(t, "id", a.ToolCalls[i].ID, w.id)
		eq(t, "function.name", a.ToolCalls[i].Function.Name, w.name)
	}
}

func TestPyTestBaseTypes_test_serialization_round_trip(t *testing.T) {
	// Python: tests/test_types.py::TestBaseTypes::test_serialization_round_trip
	doc := []byte(`{"threadId":"thread_round_trip","runId":"run_round_trip","state":{"status":"active"},
		"messages":[
			{"id":"sys_rt","role":"system","content":"You are a helpful assistant."},
			{"id":"user_rt","role":"user","content":"Help me with my task."},
			{"id":"asst_rt","role":"assistant","content":"I'll help you.","toolCalls":[
				{"id":"call_rt","type":"function","function":{"name":"get_task_info","arguments":"{}"}}]}],
		"tools":[{"name":"get_task_info","description":"Get task information","parameters":{"type":"object","properties":{}}}],
		"context":[{"description":"Session","value":"123456"}],
		"forwardedProps":{"timestamp":1648214400}}`)
	var original agui.RunAgentInput
	if err := json.Unmarshal(doc, &original); err != nil {
		t.Fatal(err)
	}
	serialized, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var back agui.RunAgentInput
	if err := json.Unmarshal(serialized, &back); err != nil {
		t.Fatal(err)
	}
	eq(t, "threadId", back.ThreadID, original.ThreadID)
	eq(t, "runId", back.RunID, original.RunID)
	eq(t, "len(messages)", len(back.Messages), len(original.Messages))
	for i, w := range []agui.Message{&agui.SystemMessage{}, &agui.UserMessage{}, &agui.AssistantMessage{}} {
		if reflect.TypeOf(back.Messages[i]) != reflect.TypeOf(w) {
			t.Errorf("messages[%d] is %T, want %T", i, back.Messages[i], w)
		}
	}
	ba, oa := back.Messages[2].(*agui.AssistantMessage), original.Messages[2].(*agui.AssistantMessage)
	eq(t, "len(toolCalls)", len(ba.ToolCalls), 1)
	eq(t, "toolCalls[0].function.name", ba.ToolCalls[0].Function.Name, oa.ToolCalls[0].Function.Name)
}

func TestPyTestBaseTypes_test_content_edge_cases(t *testing.T) {
	// Python: tests/test_types.py::TestBaseTypes::test_content_edge_cases
	var empty agui.UserMessage
	if err := json.Unmarshal([]byte(`{"id":"msg_empty","role":"user","content":""}`), &empty); err != nil {
		t.Fatal(err)
	}
	eq(t, "empty content", empty.Content.String(), "")
	if empty.Content.IsParts() {
		t.Error("empty string decoded as parts")
	}

	// Null content on an assistant message reads as absent in the tolerant
	// layer (the wire form a producer sends omits it).
	var null agui.AssistantMessage
	if err := json.Unmarshal([]byte(`{"id":"asst_null","role":"assistant","content":null,
		"toolCalls":[{"id":"call_null","type":"function","function":{"name":"get_data","arguments":"{}"}}]}`), &null); err != nil {
		t.Fatal(err)
	}
	if null.Content != nil {
		t.Errorf("content = %q, want nil", *null.Content)
	}

	large := strings.Repeat("A", 10000)
	b, _ := json.Marshal(map[string]any{"id": "msg_large", "role": "user", "content": large})
	var lm agui.UserMessage
	if err := json.Unmarshal(b, &lm); err != nil {
		t.Fatal(err)
	}
	eq(t, "len(large content)", len(lm.Content.String()), 10000)

	special := "Special chars: 你好 こんにちは 안녕하세요 👋 🌍 \n\t\"'\\/<>{}[]"
	b, _ = json.Marshal(map[string]any{"id": "msg_special", "role": "user", "content": special})
	var sm agui.UserMessage
	if err := json.Unmarshal(b, &sm); err != nil {
		t.Fatal(err)
	}
	eq(t, "special content", sm.Content.String(), special)
}

func TestPyTestBaseTypes_test_name_field_handling(t *testing.T) {
	// Python: tests/test_types.py::TestBaseTypes::test_name_field_handling
	var u agui.UserMessage
	if err := json.Unmarshal([]byte(`{"id":"user_named","role":"user","content":"Hello","name":"John"}`), &u); err != nil {
		t.Fatal(err)
	}
	eq(t, "user name", agui.Deref(u.Name), "John")

	asstDoc := `{"id":"asst_named","role":"assistant","content":"Hello","name":"AI Assistant"}`
	var a agui.AssistantMessage
	if err := json.Unmarshal([]byte(asstDoc), &a); err != nil {
		t.Fatal(err)
	}
	eq(t, "assistant name", agui.Deref(a.Name), "AI Assistant")
	eq(t, "serialized name", marshalObj(t, a)["name"], "AI Assistant")

	parsed := mustUnmarshalMessage(t, asstDoc)
	pa, ok := parsed.(*agui.AssistantMessage)
	if !ok {
		t.Fatalf("decoded %T", parsed)
	}
	eq(t, "union name", agui.Deref(pa.Name), "AI Assistant")
}

func TestPyTestBaseTypes_test_state_variations(t *testing.T) {
	// Python: tests/test_types.py::TestBaseTypes::test_state_variations
	scalarDoc := []byte(`{"threadId":"thread_scalar","runId":"run_scalar","state":"ACTIVE","messages":[],"tools":[],"context":[],"forwardedProps":{}}`)
	var scalar agui.RunAgentInput
	if err := json.Unmarshal(scalarDoc, &scalar); err != nil {
		t.Fatal(err)
	}
	eq(t, "scalar state", scalar.State, "ACTIVE")
	// State is any JSON value but null, so a scalar is valid on the wire too.
	if err := agui.Validate("RunAgentInput", scalarDoc); err != nil {
		t.Errorf("Validate rejected scalar state: %v", err)
	}

	complexDoc := []byte(`{"threadId":"thread_complex","runId":"run_complex","state":{
		"session":{"id":"sess_123","user":{"id":"user_456","preferences":{"theme":"dark","notifications":true,"filters":["important","urgent"]}},
			"metrics":{"requests":42,"tokens":{"input":1024,"output":2048}}},
		"timestamp":1648214400,"version":"1.0.0"},
		"messages":[],"tools":[],"context":[],"forwardedProps":{}}`)
	var complexIn agui.RunAgentInput
	if err := json.Unmarshal(complexDoc, &complexIn); err != nil {
		t.Fatal(err)
	}
	state := complexIn.State.(map[string]any)
	session := state["session"].(map[string]any)
	user := session["user"].(map[string]any)
	eq(t, "session.id", session["id"], "sess_123")
	eq(t, "session.user.id", user["id"], "user_456")
	eq(t, "preferences.theme", user["preferences"].(map[string]any)["theme"], "dark")
	eq(t, "metrics.tokens.output", session["metrics"].(map[string]any)["tokens"].(map[string]any)["output"], float64(2048))
	eq(t, "version", state["version"], "1.0.0")

	serialized, err := json.Marshal(complexIn)
	if err != nil {
		t.Fatal(err)
	}
	var back agui.RunAgentInput
	if err := json.Unmarshal(serialized, &back); err != nil {
		t.Fatal(err)
	}
	filters := back.State.(map[string]any)["session"].(map[string]any)["user"].(map[string]any)["preferences"].(map[string]any)["filters"]
	eq(t, "filters", filters, []any{"important", "urgent"})
}
