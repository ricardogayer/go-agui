package abstractions_events_test

// Port of tests/AGUI.Abstractions.UnitTests/RunErrorEventTest.cs.

import (
	"encoding/json"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// C#: tests/AGUI.Abstractions.UnitTests/RunErrorEventTest.cs::RunErrorEventTest.Serialization_RoundTrips
func TestCsRunErrorEventTest_Serialization_RoundTrips(t *testing.T) {
	root := serialize(t, &agui.RunErrorEvent{Message: "Something went wrong", Code: agui.Ptr("ERR_001")})
	eq(t, "type", root["type"], "RUN_ERROR")
	eq(t, "message", root["message"], "Something went wrong")
	eq(t, "code", root["code"], "ERR_001")
}

// C#: tests/AGUI.Abstractions.UnitTests/RunErrorEventTest.cs::RunErrorEventTest.Serialization_OmitsNullCode
func TestCsRunErrorEventTest_Serialization_OmitsNullCode(t *testing.T) {
	root := serialize(t, &agui.RunErrorEvent{Message: "error"})
	eq(t, "type", root["type"], "RUN_ERROR")
	eq(t, "message", root["message"], "error")
	if has(root, "code") {
		t.Errorf("code present: %v", root)
	}
	if has(root, "usage") {
		t.Errorf("usage present: %v", root)
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/RunErrorEventTest.cs::RunErrorEventTest.Serialization_WithPartialUsage
func TestCsRunErrorEventTest_Serialization_WithPartialUsage(t *testing.T) {
	evt := &agui.RunErrorEvent{
		Message: "model call failed mid-run",
		Usage:   []agui.TokenUsage{{Provider: agui.Ptr("openai"), Model: agui.Ptr("gpt-4o"), InputTokens: agui.Ptr(int64(120))}},
	}
	root := serialize(t, evt)
	eq(t, "len(usage)", arrLen(t, root["usage"]), 1)
	eq(t, "usage[0].provider", at(t, root, "usage", 0, "provider"), "openai")
	eq(t, "usage[0].model", at(t, root, "usage", 0, "model"), "gpt-4o")
	eq(t, "usage[0].inputTokens", at(t, root, "usage", 0, "inputTokens"), float64(120))
}

// C#: tests/AGUI.Abstractions.UnitTests/RunErrorEventTest.cs::RunErrorEventTest.Deserialization_WithUsage
func TestCsRunErrorEventTest_Deserialization_WithUsage(t *testing.T) {
	const doc = `{ "type": "RUN_ERROR", "message": "boom", "usage": [{ "totalTokens": 9 }] }`
	var evt agui.RunErrorEvent
	if err := json.Unmarshal([]byte(doc), &evt); err != nil {
		t.Fatal(err)
	}
	if len(evt.Usage) != 1 {
		t.Fatalf("len(Usage) = %d, want 1", len(evt.Usage))
	}
	eq(t, "TotalTokens", agui.Deref(evt.Usage[0].TotalTokens), int64(9))
	if evt.Usage[0].Provider != nil {
		t.Errorf("Provider = %q, want nil", *evt.Usage[0].Provider)
	}
}
