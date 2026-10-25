package server_test

// Port of sdks/dotnet/tests/AGUI.Hosting.AspNetCore.IntegrationTests/Samples/GettingStarted/Step05_StateManagementTest.cs.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
)

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/Samples/GettingStarted/Step05_StateManagementTest.cs::Step05_StateManagementTest.PostRun_WithState_EmitsStateSnapshotAndSummary
func TestCsStep05_StateManagementTest_PostRun_WithState_EmitsStateSnapshotAndSummary(t *testing.T) {
	r := newSampleRun(t, "Step05_StateManagement")
	for _, m := range r.baselineMessages(1) {
		r.client.AddMessage(m)
	}
	// The sample client starts from an empty recipe as the agreed state.
	r.client.Store.State = r.turns[0].Request["state"]
	res := r.runTurn(1, client.RunInput{Tools: r.baselineTools(1)})
	r.assertRequest(1, res)
	if !res.Succeeded() {
		t.Fatalf("status %s", res.Status)
	}
	// (The C# test is named for two turns but its sample makes one call.)
	snapshot := single[*agui.StateSnapshotEvent](t, res.Events).Snapshot
	if canon(r.client.Store.State) != canon(snapshot) {
		t.Fatalf("store state %s, snapshot %s", canon(r.client.Store.State), canon(snapshot))
	}
	recipe := r.client.Store.State.(map[string]any)["recipe"].(map[string]any)
	if recipe["cuisine"] != "Italian" || len(recipe["ingredients"].([]any)) == 0 {
		t.Fatalf("recipe %v", recipe)
	}
}
