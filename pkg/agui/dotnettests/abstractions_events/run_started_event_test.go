package abstractions_events_test

// Port of tests/AGUI.Abstractions.UnitTests/RunStartedEventTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// C#: tests/AGUI.Abstractions.UnitTests/RunStartedEventTest.cs::RunStartedEventTest.Serialization_RoundTrips
func TestCsRunStartedEventTest_Serialization_RoundTrips(t *testing.T) {
	evt := &agui.RunStartedEvent{
		BaseEvent: agui.BaseEvent{Timestamp: agui.Ptr(int64(1234567890))},
		ThreadID:  "t1",
		RunID:     "r1",
	}
	root := serialize(t, evt)
	eq(t, "type", root["type"], "RUN_STARTED")
	eq(t, "threadId", root["threadId"], "t1")
	eq(t, "runId", root["runId"], "r1")
	eq(t, "timestamp", root["timestamp"], float64(1234567890))
}
