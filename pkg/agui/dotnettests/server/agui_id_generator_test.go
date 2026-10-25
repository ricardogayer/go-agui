package server_test

// Port of sdks/dotnet/tests/AGUI.Server.UnitTests/AGUIIdGeneratorTest.cs.
//
// AGUIIdGenerator is the .NET SDK's internal id minting ("<prefix>_" + 24
// alphanumeric characters). The protocol leaves id formats open — it only
// needs ids that are non-empty and unique where the rules say so — and Go
// mints UUIDs (stream.SSE.TextMessage, NewMessageID, ...). The format the C#
// tests pin has no Go counterpart.

import (
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/stream"
)

const naIDGenerator = "N/A in Go: pins the .NET AGUIIdGenerator format (prefix + '_' + 24 alphanumeric chars); the protocol does not define an id format and Go mints UUIDs"

// C#: tests/AGUI.Server.UnitTests/AGUIIdGeneratorTest.cs::AGUIIdGeneratorTest.NewId_DefaultPrefix_StartsWithId
func TestCsAGUIIdGeneratorTest_NewId_DefaultPrefix_StartsWithId(t *testing.T) { t.Skip(naIDGenerator) }

// C#: tests/AGUI.Server.UnitTests/AGUIIdGeneratorTest.cs::AGUIIdGeneratorTest.NewId_CustomPrefix_StartsWithPrefix
func TestCsAGUIIdGeneratorTest_NewId_CustomPrefix_StartsWithPrefix(t *testing.T) {
	t.Skip(naIDGenerator)
}

// C#: tests/AGUI.Server.UnitTests/AGUIIdGeneratorTest.cs::AGUIIdGeneratorTest.NewId_NullPrefix_ReturnsEntropyOnly
func TestCsAGUIIdGeneratorTest_NewId_NullPrefix_ReturnsEntropyOnly(t *testing.T) {
	t.Skip(naIDGenerator)
}

// C#: tests/AGUI.Server.UnitTests/AGUIIdGeneratorTest.cs::AGUIIdGeneratorTest.NewId_EmptyPrefix_ReturnsEntropyOnly
func TestCsAGUIIdGeneratorTest_NewId_EmptyPrefix_ReturnsEntropyOnly(t *testing.T) {
	t.Skip(naIDGenerator)
}

// C#: tests/AGUI.Server.UnitTests/AGUIIdGeneratorTest.cs::AGUIIdGeneratorTest.NewId_HasCorrectEntropyLength
func TestCsAGUIIdGeneratorTest_NewId_HasCorrectEntropyLength(t *testing.T) { t.Skip(naIDGenerator) }

// C#: tests/AGUI.Server.UnitTests/AGUIIdGeneratorTest.cs::AGUIIdGeneratorTest.NewId_UsesAlphanumericCharacters
func TestCsAGUIIdGeneratorTest_NewId_UsesAlphanumericCharacters(t *testing.T) { t.Skip(naIDGenerator) }

// C#: tests/AGUI.Server.UnitTests/AGUIIdGeneratorTest.cs::AGUIIdGeneratorTest.NewMessageId_StartsWithMsg
func TestCsAGUIIdGeneratorTest_NewMessageId_StartsWithMsg(t *testing.T) { t.Skip(naIDGenerator) }

// C#: tests/AGUI.Server.UnitTests/AGUIIdGeneratorTest.cs::AGUIIdGeneratorTest.NewMessageId_HasCorrectLength
func TestCsAGUIIdGeneratorTest_NewMessageId_HasCorrectLength(t *testing.T) { t.Skip(naIDGenerator) }

// C#: tests/AGUI.Server.UnitTests/AGUIIdGeneratorTest.cs::AGUIIdGeneratorTest.NewId_GeneratesUniqueIds
func TestCsAGUIIdGeneratorTest_NewId_GeneratesUniqueIds(t *testing.T) {
	// The one property the protocol relies on — ids the producer mints for
	// distinct items are distinct — does have a Go counterpart.
	seen := map[string]bool{}
	w := run(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		for i := 0; i < 100; i++ {
			_, err := s.TextMessage("x")
			must(err)
		}
		must(s.RunFinished(in.ThreadID, in.RunID))
	})
	for _, e := range ofType[*agui.TextMessageStartEvent](w.Events) {
		seen[e.MessageID] = true
	}
	if len(seen) != 100 {
		t.Fatalf("%d distinct ids for 100 messages", len(seen))
	}
}
