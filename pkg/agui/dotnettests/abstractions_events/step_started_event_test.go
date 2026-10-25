package abstractions_events_test

// Port of tests/AGUI.Abstractions.UnitTests/StepStartedEventTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// C#: tests/AGUI.Abstractions.UnitTests/StepStartedEventTest.cs::StepStartedEventTest.Serialization_RoundTrips
func TestCsStepStartedEventTest_Serialization_RoundTrips(t *testing.T) {
	root := serialize(t, &agui.StepStartedEvent{StepName: "Process"})
	eq(t, "type", root["type"], "STEP_STARTED")
	eq(t, "stepName", root["stepName"], "Process")
	for _, k := range []string{"stepId", "parentStepId", "metadata"} {
		if has(root, k) {
			t.Errorf("%s present: %v", k, root)
		}
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/StepStartedEventTest.cs::StepStartedEventTest.Deserialize_RoundTrips
func TestCsStepStartedEventTest_Deserialize_RoundTrips(t *testing.T) {
	got := roundTrip(t, &agui.StepStartedEvent{StepName: "Initialize"})
	eq(t, "StepName", got.StepName, "Initialize")
}
