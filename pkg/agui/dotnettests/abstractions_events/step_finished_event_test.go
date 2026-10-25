package abstractions_events_test

// Port of tests/AGUI.Abstractions.UnitTests/StepFinishedEventTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// C#: tests/AGUI.Abstractions.UnitTests/StepFinishedEventTest.cs::StepFinishedEventTest.Serialization_RoundTrips
func TestCsStepFinishedEventTest_Serialization_RoundTrips(t *testing.T) {
	root := serialize(t, &agui.StepFinishedEvent{StepName: "Process"})
	eq(t, "type", root["type"], "STEP_FINISHED")
	eq(t, "stepName", root["stepName"], "Process")
	for _, k := range []string{"stepId", "status", "result"} {
		if has(root, k) {
			t.Errorf("%s present: %v", k, root)
		}
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/StepFinishedEventTest.cs::StepFinishedEventTest.Deserialize_RoundTrips
func TestCsStepFinishedEventTest_Deserialize_RoundTrips(t *testing.T) {
	got := roundTrip(t, &agui.StepFinishedEvent{StepName: "Cleanup"})
	eq(t, "StepName", got.StepName, "Cleanup")
}
