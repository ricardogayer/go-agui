package abstractions_model_test

// Port of sdks/dotnet/tests/AGUI.Abstractions.UnitTests/NullOmissionFixtureTest.cs.
// Runs the shared cross-language fixture sdks/fixtures/null-omission.json.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

const nullOmissionFixture = "../../testdata/sdk-fixtures/null-omission.json"

// C#: tests/AGUI.Abstractions.UnitTests/NullOmissionFixtureTest.cs::NullOmissionFixtureTest.FixtureCoversThisSdk
func TestCsNullOmissionFixtureTest_FixtureCoversThisSdk(t *testing.T) {
	covered, _ := loadFixtureCases(t, nullOmissionFixture, "stream")
	if len(covered) <= 15 {
		t.Fatalf("the fixture only has %d case(s) for %s", len(covered), fixtureSDK)
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/NullOmissionFixtureTest.cs::NullOmissionFixtureTest.CaseReserializesToItsExpectedJson
//
// Each input is decoded into its Go event type (agui.UnmarshalEvent, the
// polymorphic decode) and written back through the event's own MarshalJSON,
// the bytes stream.SSE frames.
func TestCsNullOmissionFixtureTest_CaseReserializesToItsExpectedJson(t *testing.T) {
	covered, _ := loadFixtureCases(t, nullOmissionFixture, "stream")
	for _, c := range covered {
		t.Run(c.Name, func(t *testing.T) {
			e, err := agui.UnmarshalEvent(c.Input)
			if err != nil {
				t.Fatalf("decode input: %v", err)
			}
			var asEvent agui.Event = e
			produced := mustJSON(t, asEvent)
			if !jsonEqual(t, produced, c.Expected) {
				t.Fatalf("case %q\nexpected: %s\nproduced: %s", c.Name, c.Expected, produced)
			}
		})
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/NullOmissionFixtureTest.cs::NullOmissionFixtureTest.CaseReserializesToItsExpectedJsonThroughHostOptionsWithTheBareContext
func TestCsNullOmissionFixtureTest_CaseReserializesToItsExpectedJsonThroughHostOptionsWithTheBareContext(t *testing.T) {
	covered, _ := loadFixtureCases(t, nullOmissionFixture, "stream")
	for _, c := range covered {
		t.Run(c.Name, func(t *testing.T) {
			t.Skipf("N/A in Go: re-runs case %s through host-owned JsonSerializerOptions holding the bare source-generated context (Microsoft Agent Framework's SSE path, which loses the context's DefaultIgnoreCondition); Go's omission lives in the types' tags/MarshalJSON and has no separate options path", c.Name)
		})
	}
}
