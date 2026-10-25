package client_test

// Port of sdks/dotnet/tests/AGUI.Client.UnitTests/CombinedJsonSerializerOptionsNullOmissionTest.cs.
//
// AGUIChatClient composes a caller's JsonSerializerOptions and must keep the
// "omit a field that has no value" rule for AG-UI wire types whatever the
// caller's resolver chain holds. Go has no resolver chain: the rule lives on
// the agui types' json tags (omitempty / omitzero) and applies on every
// encoding path. The protocol-relevant assertions — an unset optional member
// is absent, never null, on the wire; and the rule does not reach a caller's
// own (opaque) data — are kept; resolver-chain mechanics are N/A.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
)

// C#: tests/AGUI.Client.UnitTests/CombinedJsonSerializerOptionsNullOmissionTest.cs::CombinedJsonSerializerOptionsNullOmissionTest.CallerWithNoResolversGetsOmission
func TestCsCombinedJsonSerializerOptionsNullOmissionTest_CallerWithNoResolversGetsOmission(t *testing.T) {
	e := &agui.ToolCallStartEvent{ToolCallID: "tc_1", ToolCallName: "search"}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "parentMessageId") {
		t.Fatalf("unset parentMessageId serialized: %s", b)
	}
	// Same through the event interface and the generic form.
	var ev agui.Event = e
	b2, _ := json.Marshal(ev)
	g, _ := agui.ToGeneric(ev)
	if strings.Contains(string(b2), "parentMessageId") || strings.Contains(jsonString(g), "parentMessageId") {
		t.Fatalf("unset parentMessageId serialized: %s / %s", b2, jsonString(g))
	}
	if err := agui.Validate("Event", b); err != nil {
		t.Fatalf("the serialized event is not schema-valid: %v", err)
	}
}

// C#: tests/AGUI.Client.UnitTests/CombinedJsonSerializerOptionsNullOmissionTest.cs::CombinedJsonSerializerOptionsNullOmissionTest.CallerAlreadyHoldingTheBareContextStillGetsOmission
func TestCsCombinedJsonSerializerOptionsNullOmissionTest_CallerAlreadyHoldingTheBareContextStillGetsOmission(t *testing.T) {
	t.Skip("N/A in Go: concerns System.Text.Json's TypeInfoResolverChain (a caller inserting the bare source-generated AGUIJsonSerializerContext); Go encoding has no resolver chain — omission is fixed on the agui types' json tags, covered by TestCsCombinedJsonSerializerOptionsNullOmissionTest_CallerWithNoResolversGetsOmission")
}

// C#: tests/AGUI.Client.UnitTests/CombinedJsonSerializerOptionsNullOmissionTest.cs::CombinedJsonSerializerOptionsNullOmissionTest.CallerHoldingTheResolverBehindAnotherResolverStillGetsOmission
func TestCsCombinedJsonSerializerOptionsNullOmissionTest_CallerHoldingTheResolverBehindAnotherResolverStillGetsOmission(t *testing.T) {
	t.Skip("N/A in Go: concerns resolver ORDER in System.Text.Json's TypeInfoResolverChain; Go encoding has no resolver chain")
}

// C#: tests/AGUI.Client.UnitTests/CombinedJsonSerializerOptionsNullOmissionTest.cs::CombinedJsonSerializerOptionsNullOmissionTest.CallerExplicitlyDisablingOmissionStillGetsOmissionForAGUITypes
func TestCsCombinedJsonSerializerOptionsNullOmissionTest_CallerExplicitlyDisablingOmissionStillGetsOmissionForAGUITypes(t *testing.T) {
	t.Skip("N/A in Go: concerns a caller's JsonSerializerOptions.DefaultIgnoreCondition = Never; Go has no serializer-wide ignore condition a caller could set — encoding/json honours the agui types' own tags")
}

// C#: tests/AGUI.Client.UnitTests/CombinedJsonSerializerOptionsNullOmissionTest.cs::CombinedJsonSerializerOptionsNullOmissionTest.CombiningIsIdempotent
func TestCsCombinedJsonSerializerOptionsNullOmissionTest_CombiningIsIdempotent(t *testing.T) {
	t.Skip("N/A in Go: asserts AGUIChatClient.CombineJsonSerializerOptions does not add the resolver twice; Go has no serializer-options composition")
}

// C#: tests/AGUI.Client.UnitTests/CombinedJsonSerializerOptionsNullOmissionTest.cs::CombinedJsonSerializerOptionsNullOmissionTest.ResumePayloadOmitsItsUnsetFields
//
// .NET's declined-approval payload must serialize as {"approved":false} with
// no null members, since it rides inside the RunAgentInput. Go: a resume
// entry's payload travels exactly as given, and the entry's own unset members
// are absent (ResumeEntry.payload is "not null" in the schema).
func TestCsCombinedJsonSerializerOptionsNullOmissionTest_ResumePayloadOmitsItsUnsetFields(t *testing.T) {
	stub := newStub(t, interruptThenEcho("int-1"))
	agent, _ := newAgent(stub.URL)
	agent.AddMessage(user("u1", "hi"))
	if _, err := agent.Run(t.Context(), client.RunInput{}, nil); err != nil {
		t.Fatal(err)
	}
	_, err := agent.Run(t.Context(), client.RunInput{Resume: []agui.ResumeEntry{
		{InterruptID: "int-1", Status: agui.ResumeStatusResolved, Payload: map[string]any{"approved": false}},
		// The run raised only int-1; a second entry with nothing set would be refused,
		// so the bare-entry case is checked on a marshal below.
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	entry := mustAt(t, stub.lastBody(t), "resume.0")
	if got := jsonString(mustAt(t, entry, "payload")); got != `{"approved":false}` {
		t.Errorf("payload = %s", got)
	}
	if _, ok := at(entry, "metadata"); ok {
		t.Errorf("unset metadata sent: %s", jsonString(entry))
	}
	b, _ := json.Marshal(agui.ResumeEntry{InterruptID: "x", Status: agui.ResumeStatusCancelled})
	if string(b) != `{"interruptId":"x","status":"cancelled"}` {
		t.Errorf("bare resume entry = %s", b)
	}
}

// C#: tests/AGUI.Client.UnitTests/CombinedJsonSerializerOptionsNullOmissionTest.cs::CombinedJsonSerializerOptionsNullOmissionTest.TheRuleIsScopedToAGUITypes
//
// The omission must not reach the caller's own data: a null inside opaque
// caller material (forwardedProps, "any JSON value") is sent as the caller
// wrote it.
func TestCsCombinedJsonSerializerOptionsNullOmissionTest_TheRuleIsScopedToAGUITypes(t *testing.T) {
	type callerModel struct {
		Note *string `json:"note"`
	}
	stub := newStub(t, echo())
	agent, _ := newAgent(stub.URL)
	agent.AddMessage(user("u1", "hi"))
	if _, err := agent.Run(t.Context(), client.RunInput{ForwardedProps: map[string]any{"model": callerModel{}}}, nil); err != nil {
		t.Fatal(err)
	}
	v, ok := at(stub.lastBody(t), "forwardedProps.model.note")
	if !ok || v != nil {
		t.Fatalf("the caller's null was not kept: forwardedProps = %s", jsonString(mustAt(t, stub.lastBody(t), "forwardedProps")))
	}
}
