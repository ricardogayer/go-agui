package server_test

// Port of sdks/dotnet/tests/AGUI.Server.UnitTests/RunAgentInputExtensionsTest.cs.
//
// TryGetRunAgentState<T> reads RunAgentInput.state, as received, into a typed
// value. The Go counterpart is the input the handler parses
// (agui.ParseRunAgentInput, under the processing model) and decoding its State
// into a Go type; tryGetState below is that one-liner.

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// documentState is the C# DocumentState test type.
type documentState struct {
	DocumentTitle string `json:"document_title"`
	Revision      int    `json:"revision"`
}

// parseState parses a run input whose state member is stateJSON ("" for
// absent), as the handler does.
func parseState(t *testing.T, stateJSON string) (*agui.RunAgentInput, []agui.Warning) {
	t.Helper()
	body := `{"threadId":"t","runId":"r","messages":[]`
	if stateJSON != "" {
		body += `,"state":` + stateJSON
	}
	in, warnings, err := agui.ParseRunAgentInput([]byte(body + "}"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return in, warnings
}

// tryGetState decodes the input's state into T; ok is false when the input
// carried no state.
func tryGetState[T any](in *agui.RunAgentInput) (value T, ok bool, err error) {
	if in.State == nil {
		return value, false, nil
	}
	b, err := json.Marshal(in.State)
	if err != nil {
		return value, true, err
	}
	err = json.Unmarshal(b, &value)
	return value, true, err
}

// C#: tests/AGUI.Server.UnitTests/RunAgentInputExtensionsTest.cs::RunAgentInputExtensionsTest.TryGetRunAgentState_NoInput_ReturnsFalse
func TestCsRunAgentInputExtensionsTest_TryGetRunAgentState_NoInput_ReturnsFalse(t *testing.T) {
	t.Skip("N/A in Go: pins MEAI ChatOptions carrying no RunAgentInput in AdditionalProperties; a Go agent always receives the parsed *agui.RunAgentInput")
}

// C#: tests/AGUI.Server.UnitTests/RunAgentInputExtensionsTest.cs::RunAgentInputExtensionsTest.TryGetRunAgentState_NoState_ReturnsFalse
func TestCsRunAgentInputExtensionsTest_TryGetRunAgentState_NoState_ReturnsFalse(t *testing.T) {
	for _, row := range []struct{ name, json string }{{"null", ""}, {`"null"`, "null"}} {
		t.Run(row.name, func(t *testing.T) {
			in, warnings := parseState(t, row.json)
			state, ok, err := tryGetState[*documentState](in)
			if ok || state != nil || err != nil {
				t.Fatalf("ok=%v state=%v err=%v", ok, state, err)
			}
			// run-input.mdx: an absent state means the empty object.
			if !reflect.DeepEqual(in.StartingState(), map[string]any{}) {
				t.Fatalf("starting state %v", in.StartingState())
			}
			// "state": null is not 1.0 (the schema says not null); it is read
			// as absent, with a warning.
			if row.json == "null" && len(warnings) == 0 {
				t.Fatal("state: null was accepted without a warning")
			}
		})
	}
}

// C#: tests/AGUI.Server.UnitTests/RunAgentInputExtensionsTest.cs::RunAgentInputExtensionsTest.TryGetRunAgentState_UndefinedState_ReturnsFalse
func TestCsRunAgentInputExtensionsTest_TryGetRunAgentState_UndefinedState_ReturnsFalse(t *testing.T) {
	in, _ := parseState(t, "")
	if _, ok, err := tryGetState[documentState](in); ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

// C#: tests/AGUI.Server.UnitTests/RunAgentInputExtensionsTest.cs::RunAgentInputExtensionsTest.TryGetRunAgentState_Object_UsesProvidedMetadataAndDoesNotMutateInput
func TestCsRunAgentInputExtensionsTest_TryGetRunAgentState_Object_UsesProvidedMetadataAndDoesNotMutateInput(t *testing.T) {
	in, _ := parseState(t, `{"document_title":"Draft","revision":2}`)
	state, ok, err := tryGetState[*documentState](in)
	if !ok || err != nil || state == nil || state.DocumentTitle != "Draft" || state.Revision != 2 {
		t.Fatalf("ok=%v err=%v state=%+v", ok, err, state)
	}
	state.DocumentTitle = "Changed"
	raw := in.State.(map[string]any)
	if raw["document_title"] != "Draft" || raw["revision"] != float64(2) {
		t.Fatalf("the input's state was mutated: %v", raw)
	}
}

// C#: tests/AGUI.Server.UnitTests/RunAgentInputExtensionsTest.cs::RunAgentInputExtensionsTest.TryGetRunAgentState_CustomConverter_ReturningNullStillReportsSuppliedState
func TestCsRunAgentInputExtensionsTest_TryGetRunAgentState_CustomConverter_ReturningNullStillReportsSuppliedState(t *testing.T) {
	t.Skip("N/A in Go: pins a System.Text.Json custom JsonConverter returning null through JsonTypeInfo metadata; Go decoding has no converter registry")
}

// C#: tests/AGUI.Server.UnitTests/RunAgentInputExtensionsTest.cs::RunAgentInputExtensionsTest.TryGetRunAgentState_EmptyObject_ReturnsTrue
func TestCsRunAgentInputExtensionsTest_TryGetRunAgentState_EmptyObject_ReturnsTrue(t *testing.T) {
	in, _ := parseState(t, "{}")
	state, ok, err := tryGetState[*documentState](in)
	if !ok || err != nil || state == nil || state.DocumentTitle != "" || state.Revision != 0 {
		t.Fatalf("ok=%v err=%v state=%+v", ok, err, state)
	}
}

// C#: tests/AGUI.Server.UnitTests/RunAgentInputExtensionsTest.cs::RunAgentInputExtensionsTest.TryGetRunAgentState_ValueType_ReturnsTrue
func TestCsRunAgentInputExtensionsTest_TryGetRunAgentState_ValueType_ReturnsTrue(t *testing.T) {
	for _, row := range []struct {
		json string
		want int
	}{{"0", 0}, {"42", 42}} {
		t.Run(row.json, func(t *testing.T) {
			in, _ := parseState(t, row.json)
			state, ok, err := tryGetState[int](in)
			if !ok || err != nil || state != row.want {
				t.Fatalf("ok=%v err=%v state=%v", ok, err, state)
			}
		})
	}
}

// C#: tests/AGUI.Server.UnitTests/RunAgentInputExtensionsTest.cs::RunAgentInputExtensionsTest.TryGetRunAgentState_MissingValueType_ReturnsFalseWithDefault
func TestCsRunAgentInputExtensionsTest_TryGetRunAgentState_MissingValueType_ReturnsFalseWithDefault(t *testing.T) {
	in, _ := parseState(t, "")
	state, ok, err := tryGetState[int](in)
	if ok || err != nil || state != 0 {
		t.Fatalf("ok=%v err=%v state=%v", ok, err, state)
	}
}

// C#: tests/AGUI.Server.UnitTests/RunAgentInputExtensionsTest.cs::RunAgentInputExtensionsTest.TryGetRunAgentState_Array_ReturnsTypedArray
func TestCsRunAgentInputExtensionsTest_TryGetRunAgentState_Array_ReturnsTypedArray(t *testing.T) {
	in, _ := parseState(t, "[1,2]")
	state, ok, err := tryGetState[[]int](in)
	if !ok || err != nil || !reflect.DeepEqual(state, []int{1, 2}) {
		t.Fatalf("ok=%v err=%v state=%v", ok, err, state)
	}
}

// C#: tests/AGUI.Server.UnitTests/RunAgentInputExtensionsTest.cs::RunAgentInputExtensionsTest.TryGetRunAgentState_IncompatibleState_ThrowsJsonException
func TestCsRunAgentInputExtensionsTest_TryGetRunAgentState_IncompatibleState_ThrowsJsonException(t *testing.T) {
	for _, js := range []string{"[]", "42", `{"revision":"invalid"}`} {
		t.Run(js, func(t *testing.T) {
			// Any JSON value is a valid state for the protocol: the input
			// parses; it is the typed read that fails.
			in, _ := parseState(t, js)
			if _, ok, err := tryGetState[*documentState](in); !ok || err == nil {
				t.Fatalf("ok=%v err=%v", ok, err)
			}
		})
	}
}

// C#: tests/AGUI.Server.UnitTests/RunAgentInputExtensionsTest.cs::RunAgentInputExtensionsTest.TryGetRunAgentState_NullOptions_ThrowsArgumentNullException
func TestCsRunAgentInputExtensionsTest_TryGetRunAgentState_NullOptions_ThrowsArgumentNullException(t *testing.T) {
	t.Skip("N/A in Go: .NET ArgumentNullException guard on a null ChatOptions receiver")
}

// C#: tests/AGUI.Server.UnitTests/RunAgentInputExtensionsTest.cs::RunAgentInputExtensionsTest.TryGetRunAgentState_NullMetadata_ThrowsEvenWithoutState
func TestCsRunAgentInputExtensionsTest_TryGetRunAgentState_NullMetadata_ThrowsEvenWithoutState(t *testing.T) {
	t.Skip("N/A in Go: .NET ArgumentNullException guard on a null JsonTypeInfo argument")
}
