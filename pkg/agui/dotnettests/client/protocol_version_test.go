package client_test

// Port of sdks/dotnet/tests/AGUI.Client.UnitTests/ProtocolVersionTest.cs.
//
// The in-band version handshake on the consumer side: the version this client
// declares on the request (run-input.mdx, protocolVersion), and what it makes
// of the version a producer declares on RUN_STARTED (versioning.mdx). The .NET
// warnings are Trace calls; the Go client's are delivered to Agent.Warn /
// Pipeline.Warn.

import (
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
)

// replayVersion feeds RUN_STARTED (declaring version, nil for absent) and
// RUN_FINISHED through the consumer pipeline and returns its warnings.
func replayVersion(t *testing.T, version *string) []string {
	t.Helper()
	f := newFeed()
	f.mustPush(t,
		&agui.RunStartedEvent{ThreadID: "t1", RunID: "r1", ProtocolVersion: version},
		&agui.RunFinishedEvent{ThreadID: "t1", RunID: "r1"})
	return f.warnings
}

// C#: tests/AGUI.Client.UnitTests/ProtocolVersionTest.cs::ProtocolVersionTest.Request_DeclaresTheProtocolVersionItSpeaks
func TestCsProtocolVersionTest_Request_DeclaresTheProtocolVersionItSpeaks(t *testing.T) {
	stub := newStub(t, echo())
	agent, _ := newAgent(stub.URL)
	agent.AddMessage(user("u1", "hi"))
	if _, err := agent.Run(t.Context(), client.RunInput{}, nil); err != nil {
		t.Fatal(err)
	}
	if got := mustAt(t, stub.lastBody(t), "protocolVersion"); got != "1.0" {
		t.Fatalf("protocolVersion on the wire = %v, want \"1.0\"", got)
	}
	if got := agui.Deref(stub.lastInput(t).ProtocolVersion); got != agui.ProtocolVersion {
		t.Fatalf("protocolVersion %q != agui.ProtocolVersion %q", got, agui.ProtocolVersion)
	}
}

// C#: tests/AGUI.Client.UnitTests/ProtocolVersionTest.cs::ProtocolVersionTest.ProducerDeclaresNewerVersion_Warns
func TestCsProtocolVersionTest_ProducerDeclaresNewerVersion_Warns(t *testing.T) {
	warnings := replayVersion(t, agui.Ptr("1.1"))
	if !anyContains(warnings, "1.1") {
		t.Fatalf("no warning names 1.1: %v", warnings)
	}
}

// C#: tests/AGUI.Client.UnitTests/ProtocolVersionTest.cs::ProtocolVersionTest.ProducerDeclaresUninterpretableVersion_Warns
func TestCsProtocolVersionTest_ProducerDeclaresUninterpretableVersion_Warns(t *testing.T) {
	warnings := replayVersion(t, agui.Ptr("draft"))
	if !anyContains(warnings, "cannot interpret") {
		t.Fatalf("no warning says \"cannot interpret\": %v", warnings)
	}
}

// C#: tests/AGUI.Client.UnitTests/ProtocolVersionTest.cs::ProtocolVersionTest.ProducerDeclaresTheSameVersion_IsQuiet
func TestCsProtocolVersionTest_ProducerDeclaresTheSameVersion_IsQuiet(t *testing.T) {
	if w := replayVersion(t, agui.Ptr(agui.ProtocolVersion)); len(w) != 0 {
		t.Fatalf("expected no warnings, got %v", w)
	}
}

// C#: tests/AGUI.Client.UnitTests/ProtocolVersionTest.cs::ProtocolVersionTest.ProducerDeclaresNoVersion_IsQuiet
func TestCsProtocolVersionTest_ProducerDeclaresNoVersion_IsQuiet(t *testing.T) {
	if w := replayVersion(t, nil); len(w) != 0 {
		t.Fatalf("expected no warnings, got %v", w)
	}
}

// C#: tests/AGUI.Client.UnitTests/ProtocolVersionTest.cs::ProtocolVersionTest.ProducerDeclaresOlderVersion_IsQuiet
func TestCsProtocolVersionTest_ProducerDeclaresOlderVersion_IsQuiet(t *testing.T) {
	if w := replayVersion(t, agui.Ptr("0.9")); len(w) != 0 {
		t.Fatalf("expected no warnings, got %v", w)
	}
}

func anyContains(list []string, sub string) bool {
	for _, s := range list {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
