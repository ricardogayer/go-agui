package server_test

// Port of sdks/dotnet/tests/AGUI.Server.UnitTests/InterruptContentTest.cs.
//
// InterruptRequestContent / InterruptResponseContent are MEAI AIContent
// carriers for the protocol's Interrupt and ResumeEntry. The Go counterparts
// are agui.Interrupt and agui.ResumeEntry themselves: their fields, required
// members (by the schema) and wire shape are ported. MEAI type hierarchy and
// the "$type" polymorphic AIContent serialization are N/A.

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/stream"
)

const (
	naMEAIHierarchy = "N/A in Go: pins the MEAI InputRequestContent/InputResponseContent/AIContent class hierarchy; Go carries agui.Interrupt/agui.ResumeEntry directly"
	naAIContentJSON = "N/A in Go: pins System.Text.Json polymorphic AIContent serialization (the \"$type\" discriminator registered by RegisterInterruptContentTypes), which is not AG-UI wire format"
	naEmptyIDGuard  = "N/A in Go: .NET ArgumentException guard on an empty/whitespace id; the 1.0 schema puts no length constraint on the id, so there is no Go-side rule to port"
)

func wireOf(t *testing.T, v any) map[string]any {
	t.Helper()
	g, ok := toGeneric(t, v).(map[string]any)
	if !ok {
		t.Fatalf("%T does not marshal to an object", v)
	}
	return g
}

// C#: tests/AGUI.Server.UnitTests/InterruptContentTest.cs::InterruptContentTest.InterruptRequestContent_Constructor_SetsRequestId
func TestCsInterruptContentTest_InterruptRequestContent_Constructor_SetsRequestId(t *testing.T) {
	// Adapted: reason is required by the 1.0 schema, so the minimal
	// interrupt carries id and reason; every optional field is absent.
	i := agui.Interrupt{ID: "req-123", Reason: "tool_call"}
	if i.Message != nil || i.ToolCallID != nil || i.ResponseSchema != nil || i.ExpiresAt != nil || i.Metadata != nil {
		t.Fatalf("%+v", i)
	}
	if err := agui.ValidateValue("Interrupt", wireOf(t, i)); err != nil {
		t.Fatal(err)
	}
}

// C#: tests/AGUI.Server.UnitTests/InterruptContentTest.cs::InterruptContentTest.InterruptRequestContent_Properties_Roundtrip
func TestCsInterruptContentTest_InterruptRequestContent_Properties_Roundtrip(t *testing.T) {
	i := agui.Interrupt{
		ID: "req-456", Reason: "input_required", Message: agui.Ptr("Please provide input"), ToolCallID: agui.Ptr("call-1"),
		ResponseSchema: map[string]any{"type": "string"}, ExpiresAt: agui.Ptr("2026-12-31T23:59:59Z"),
		Metadata: agui.Metadata{"key": "value"},
	}
	b, err := json.Marshal(i)
	if err != nil {
		t.Fatal(err)
	}
	if err := agui.Validate("Interrupt", b); err != nil {
		t.Fatal(err)
	}
	var back agui.Interrupt
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back, i) {
		t.Fatalf("round trip\n got: %+v\nwant: %+v", back, i)
	}
	if back.ResponseSchema["type"] != "string" || back.Metadata["key"] != "value" || agui.Deref(back.ExpiresAt) != "2026-12-31T23:59:59Z" {
		t.Fatalf("%+v", back)
	}
}

// C#: tests/AGUI.Server.UnitTests/InterruptContentTest.cs::InterruptContentTest.InterruptRequestContent_IsInputRequestContent
func TestCsInterruptContentTest_InterruptRequestContent_IsInputRequestContent(t *testing.T) {
	t.Skip(naMEAIHierarchy)
}

// C#: tests/AGUI.Server.UnitTests/InterruptContentTest.cs::InterruptContentTest.InterruptRequestContent_NullRequestId_Throws
func TestCsInterruptContentTest_InterruptRequestContent_NullRequestId_Throws(t *testing.T) {
	if err := agui.Validate("Interrupt", []byte(`{"reason":"tool_call"}`)); err == nil {
		t.Fatal("an interrupt without id was accepted")
	}
}

// C#: tests/AGUI.Server.UnitTests/InterruptContentTest.cs::InterruptContentTest.InterruptRequestContent_EmptyRequestId_Throws
func TestCsInterruptContentTest_InterruptRequestContent_EmptyRequestId_Throws(t *testing.T) {
	t.Skip(naEmptyIDGuard)
}

// C#: tests/AGUI.Server.UnitTests/InterruptContentTest.cs::InterruptContentTest.InterruptResponseContent_Constructor_SetsRequestId
func TestCsInterruptContentTest_InterruptResponseContent_Constructor_SetsRequestId(t *testing.T) {
	r := agui.ResumeEntry{InterruptID: "req-123", Status: agui.ResumeStatusResolved}
	if r.Payload != nil {
		t.Fatal("payload set")
	}
	d := wireOf(t, r)
	if _, ok := d["payload"]; ok {
		t.Fatalf("absent payload on the wire: %v", d)
	}
	if err := agui.ValidateValue("ResumeEntry", d); err != nil {
		t.Fatal(err)
	}
}

// C#: tests/AGUI.Server.UnitTests/InterruptContentTest.cs::InterruptContentTest.InterruptResponseContent_Properties_Roundtrip
func TestCsInterruptContentTest_InterruptResponseContent_Properties_Roundtrip(t *testing.T) {
	r := agui.ResumeEntry{InterruptID: "req-456", Status: agui.ResumeStatusResolved, Payload: map[string]any{"approved": true}}
	b, _ := json.Marshal(r)
	var back agui.ResumeEntry
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.InterruptID != "req-456" || back.Payload.(map[string]any)["approved"] != true {
		t.Fatalf("%+v", back)
	}
}

// C#: tests/AGUI.Server.UnitTests/InterruptContentTest.cs::InterruptContentTest.InterruptResponseContent_IsInputResponseContent
func TestCsInterruptContentTest_InterruptResponseContent_IsInputResponseContent(t *testing.T) {
	t.Skip(naMEAIHierarchy)
}

// C#: tests/AGUI.Server.UnitTests/InterruptContentTest.cs::InterruptContentTest.InterruptResponseContent_NullRequestId_Throws
func TestCsInterruptContentTest_InterruptResponseContent_NullRequestId_Throws(t *testing.T) {
	if err := agui.Validate("ResumeEntry", []byte(`{"status":"resolved"}`)); err == nil {
		t.Fatal("a resume entry without interruptId was accepted")
	}
}

// C#: tests/AGUI.Server.UnitTests/InterruptContentTest.cs::InterruptContentTest.InterruptResponseContent_EmptyRequestId_Throws
func TestCsInterruptContentTest_InterruptResponseContent_EmptyRequestId_Throws(t *testing.T) {
	t.Skip(naEmptyIDGuard)
}

const resumeWithMetadata = `{"threadId":"t1","runId":"r1","protocolVersion":"1.0","messages":[],
 "resume":[{"interruptId":"generic-1","status":"resolved","payload":{"approved":true},
            "metadata":{"definitionId":"review-plan","key":"afterModel-review"}}]}`

// C#: tests/AGUI.Server.UnitTests/InterruptContentTest.cs::InterruptContentTest.ResumeMetadata_SurvivesTranslationToInterruptResponseContent
func TestCsInterruptContentTest_ResumeMetadata_SurvivesTranslationToInterruptResponseContent(t *testing.T) {
	var seen []agui.ResumeEntry
	runWith(t, resumeWithMetadata, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		seen = in.Resume
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	if len(seen) != 1 || seen[0].InterruptID != "generic-1" {
		t.Fatalf("resume %+v", seen)
	}
	if seen[0].Metadata["definitionId"] != "review-plan" || seen[0].Metadata["key"] != "afterModel-review" {
		t.Fatalf("metadata %+v", seen[0].Metadata)
	}
}

// C#: tests/AGUI.Server.UnitTests/InterruptContentTest.cs::InterruptContentTest.ResumeWithoutMetadata_TranslatesWithoutIt
func TestCsInterruptContentTest_ResumeWithoutMetadata_TranslatesWithoutIt(t *testing.T) {
	var seen []agui.ResumeEntry
	body := strings.Replace(resumeWithMetadata, `,
            "metadata":{"definitionId":"review-plan","key":"afterModel-review"}`, "", 1)
	runWith(t, body, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		seen = in.Resume
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	if len(seen) != 1 || seen[0].Metadata != nil {
		t.Fatalf("resume %+v", seen)
	}
}

// C#: tests/AGUI.Server.UnitTests/InterruptContentTest.cs::InterruptContentTest.InterruptRequestContent_SerializesAs_AIContent_WithDiscriminator
func TestCsInterruptContentTest_InterruptRequestContent_SerializesAs_AIContent_WithDiscriminator(t *testing.T) {
	t.Skip(naAIContentJSON)
}

// C#: tests/AGUI.Server.UnitTests/InterruptContentTest.cs::InterruptContentTest.InterruptRequestContent_DeserializesFrom_AIContent
func TestCsInterruptContentTest_InterruptRequestContent_DeserializesFrom_AIContent(t *testing.T) {
	t.Skip(naAIContentJSON)
}

// C#: tests/AGUI.Server.UnitTests/InterruptContentTest.cs::InterruptContentTest.InterruptRequestContent_DeserializesFrom_InputRequestContent
func TestCsInterruptContentTest_InterruptRequestContent_DeserializesFrom_InputRequestContent(t *testing.T) {
	t.Skip(naAIContentJSON)
}

// C#: tests/AGUI.Server.UnitTests/InterruptContentTest.cs::InterruptContentTest.InterruptResponseContent_SerializesAs_AIContent_WithDiscriminator
func TestCsInterruptContentTest_InterruptResponseContent_SerializesAs_AIContent_WithDiscriminator(t *testing.T) {
	t.Skip(naAIContentJSON)
}

// C#: tests/AGUI.Server.UnitTests/InterruptContentTest.cs::InterruptContentTest.InterruptResponseContent_DeserializesFrom_AIContent
func TestCsInterruptContentTest_InterruptResponseContent_DeserializesFrom_AIContent(t *testing.T) {
	t.Skip(naAIContentJSON)
}

// C#: tests/AGUI.Server.UnitTests/InterruptContentTest.cs::InterruptContentTest.InterruptResponseContent_DeserializesFrom_InputResponseContent
func TestCsInterruptContentTest_InterruptResponseContent_DeserializesFrom_InputResponseContent(t *testing.T) {
	t.Skip(naAIContentJSON)
}

// C#: tests/AGUI.Server.UnitTests/InterruptContentTest.cs::InterruptContentTest.InterruptRequestContent_RoundTrips_ThroughAIContent
func TestCsInterruptContentTest_InterruptRequestContent_RoundTrips_ThroughAIContent(t *testing.T) {
	t.Skip(naAIContentJSON)
}

// C#: tests/AGUI.Server.UnitTests/InterruptContentTest.cs::InterruptContentTest.InterruptResponseContent_RoundTrips_ThroughAIContent
func TestCsInterruptContentTest_InterruptResponseContent_RoundTrips_ThroughAIContent(t *testing.T) {
	t.Skip(naAIContentJSON)
}

// C#: tests/AGUI.Server.UnitTests/InterruptContentTest.cs::InterruptContentTest.InterruptRequestContent_NullFields_OmittedFromJson
func TestCsInterruptContentTest_InterruptRequestContent_NullFields_OmittedFromJson(t *testing.T) {
	b, _ := json.Marshal(agui.Interrupt{ID: "req-no-fields", Reason: "tool_call"})
	for _, k := range []string{`"metadata"`, `"message"`, `"toolCallId"`, `"responseSchema"`, `"expiresAt"`, "null"} {
		if strings.Contains(string(b), k) {
			t.Fatalf("%s present in %s", k, b)
		}
	}
}

// C#: tests/AGUI.Server.UnitTests/InterruptContentTest.cs::InterruptContentTest.InterruptContent_InsideChatResponseUpdate_RoundTrips
func TestCsInterruptContentTest_InterruptContent_InsideChatResponseUpdate_RoundTrips(t *testing.T) {
	t.Skip(naAIContentJSON)
}

// C#: tests/AGUI.Server.UnitTests/InterruptContentTest.cs::InterruptContentTest.RegisterInterruptContentTypes_RegistersBothTypes
func TestCsInterruptContentTest_RegisterInterruptContentTypes_RegistersBothTypes(t *testing.T) {
	t.Skip(naAIContentJSON)
}

// C#: tests/AGUI.Server.UnitTests/InterruptContentTest.cs::InterruptContentTest.RegisterInterruptContentTypes_RegistersUnderInputRequestContent
func TestCsInterruptContentTest_RegisterInterruptContentTypes_RegistersUnderInputRequestContent(t *testing.T) {
	t.Skip(naAIContentJSON)
}

// C#: tests/AGUI.Server.UnitTests/InterruptContentTest.cs::InterruptContentTest.RegisterInterruptContentTypes_RegistersUnderInputResponseContent
func TestCsInterruptContentTest_RegisterInterruptContentTypes_RegistersUnderInputResponseContent(t *testing.T) {
	t.Skip(naAIContentJSON)
}
