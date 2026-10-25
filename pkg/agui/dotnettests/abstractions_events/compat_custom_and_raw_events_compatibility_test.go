package abstractions_events_test

// Port of tests/AGUI.Abstractions.UnitTests/Compatibility/CustomAndRawEventsCompatibilityTest.cs.

import (
	"encoding/json"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/CustomAndRawEventsCompatibilityTest.cs::CustomAndRawEventsCompatibilityTest.CustomEvent_WithValue_DeserializesFromTypeScriptPayload
func TestCsCustomAndRawEventsCompatibilityTest_CustomEvent_WithValue_DeserializesFromTypeScriptPayload(t *testing.T) {
	fx := loadFixture(t, "custom-and-raw-events.json")
	typed := asType[*agui.CustomEvent](t, decodeFixtureEvent(t, fx[0]))
	eq(t, "Name", typed.Name, "user_preference_updated")
	eq(t, "Timestamp", agui.Deref(typed.Timestamp), int64(1234567890))
	eq(t, "value.theme", at(t, typed.Value, "theme"), "dark")
	eq(t, "value.fontSize", at(t, typed.Value, "fontSize"), "medium")
	eq(t, "value.notifications", at(t, typed.Value, "notifications"), true)
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/CustomAndRawEventsCompatibilityTest.cs::CustomAndRawEventsCompatibilityTest.CustomEvent_WithoutValue_DeserializesFromTypeScriptPayload
//
// Adapted: CUSTOM.value is REQUIRED in AG-UI 1.0 (schema CustomEvent.required;
// spec fixture CustomEvent/invalid/value-missing.json), with null as one of
// its values. The lenient .NET model reads a missing value as null; that is
// .NET parser leniency, not protocol behaviour. Go's non-validating parser
// (UnmarshalEvent) reads it the same way, but the strict schema check and
// the processing-model stage must reject the document as malformed.
func TestCsCustomAndRawEventsCompatibilityTest_CustomEvent_WithoutValue_DeserializesFromTypeScriptPayload(t *testing.T) {
	fx := loadFixture(t, "custom-and-raw-events.json")
	e, err := agui.UnmarshalEvent(fx[1])
	if err != nil {
		t.Fatal(err)
	}
	typed := asType[*agui.CustomEvent](t, e)
	eq(t, "Name", typed.Name, "heartbeat")
	if typed.Value != nil {
		t.Errorf("Value = %#v, want nil", typed.Value)
	}
	if err := agui.Validate("Event", fx[1]); err == nil {
		t.Error("strict schema accepted a CUSTOM event without the required value")
	}
	if _, err := agui.Sanitize("Event", fx[1], agui.SanitizeOptions{}); err == nil {
		t.Error("processing model accepted a CUSTOM event without the required value")
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/CustomAndRawEventsCompatibilityTest.cs::CustomAndRawEventsCompatibilityTest.RawEvent_Basic_DeserializesFromTypeScriptPayload
func TestCsCustomAndRawEventsCompatibilityTest_RawEvent_Basic_DeserializesFromTypeScriptPayload(t *testing.T) {
	fx := loadFixture(t, "custom-and-raw-events.json")
	typed := asType[*agui.RawEvent](t, decodeFixtureEvent(t, fx[2]))
	eq(t, "Source", agui.Deref(typed.Source), "frontend")
	eq(t, "Timestamp", agui.Deref(typed.Timestamp), int64(1234567890))
	eq(t, "event.type", at(t, typed.Event, "type"), "user_action")
	eq(t, "event.action", at(t, typed.Event, "action"), "button_click")
	eq(t, "event.elementId", at(t, typed.Event, "elementId"), "submit-btn")
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/CustomAndRawEventsCompatibilityTest.cs::CustomAndRawEventsCompatibilityTest.RawEvent_ComplexNested_DeserializesFromTypeScriptPayload
func TestCsCustomAndRawEventsCompatibilityTest_RawEvent_ComplexNested_DeserializesFromTypeScriptPayload(t *testing.T) {
	fx := loadFixture(t, "custom-and-raw-events.json")
	typed := asType[*agui.RawEvent](t, decodeFixtureEvent(t, fx[3]))
	if typed.Source != nil {
		t.Errorf("Source = %q, want nil", *typed.Source)
	}
	eq(t, "event.type", at(t, typed.Event, "type"), "analytics_event")
	session := at(t, typed.Event, "session")
	eq(t, "session.id", at(t, session, "id"), "sess-12345")
	eq(t, "session.user.id", at(t, session, "user", "id"), "user-456")
	eq(t, "session.user.attributes.plan", at(t, session, "user", "attributes", "plan"), "premium")
	actions := at(t, session, "actions")
	eq(t, "len(actions)", arrLen(t, actions), 2)
	eq(t, "actions[0].type", at(t, actions, 0, "type"), "page_view")
	eq(t, "actions[1].type", at(t, actions, 1, "type"), "button_click")
	metadata := at(t, typed.Event, "metadata")
	eq(t, "metadata.source", at(t, metadata, "source"), "web")
	eq(t, "metadata.version", at(t, metadata, "version"), "1.2.3")
	eq(t, "metadata.environment", at(t, metadata, "environment"), "production")
}

// C#: tests/AGUI.Abstractions.UnitTests/Compatibility/CustomAndRawEventsCompatibilityTest.cs::CustomAndRawEventsCompatibilityTest.AllCustomAndRawEvents_RoundTrip_PreservesType
//
// Adapted: fixture 1 (CUSTOM without the required value) is malformed under
// 1.0 (see above) and cannot pass the processing model, so it is round-tripped
// through the non-validating parser; its re-serialisation must restore the
// required key as "value":null.
func TestCsCustomAndRawEventsCompatibilityTest_AllCustomAndRawEvents_RoundTrip_PreservesType(t *testing.T) {
	fx := loadFixture(t, "custom-and-raw-events.json")
	assertRoundTripPreservesType(t, []json.RawMessage{fx[0], fx[2], fx[3]})

	e, err := agui.UnmarshalEvent(fx[1])
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	if err := agui.Validate("Event", b); err != nil {
		t.Errorf("re-serialised CUSTOM is not valid 1.0: %v\n%s", err, b)
	}
	again, err := agui.UnmarshalEvent(b)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "type", again.Type(), e.Type())
}
