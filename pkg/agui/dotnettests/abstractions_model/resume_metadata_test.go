package abstractions_model_test

// Port of sdks/dotnet/tests/AGUI.Abstractions.UnitTests/ResumeMetadataTest.cs.

import (
	"encoding/json"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

const resumeValueShapesJSON = `{
  "nullValue": null,
  "string": "afterModel-review",
  "number": 42,
  "float": 1.5,
  "boolean": true,
  "emptyArray": [],
  "array": [1, "two", null, { "nested": true }],
  "emptyObject": {},
  "nested": { "signature": { "alg": "ed25519", "hash": "abc" }, "tags": ["a", "b"] }
}`

func decodeResume(t *testing.T, data []byte) agui.ResumeEntry {
	t.Helper()
	var r agui.ResumeEntry
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatalf("decode %s: %v", data, err)
	}
	return r
}

// C#: tests/AGUI.Abstractions.UnitTests/ResumeMetadataTest.cs::ResumeMetadataTest.Resume_RoundTripsEveryValueShape
func TestCsResumeMetadataTest_Resume_RoundTripsEveryValueShape(t *testing.T) {
	resume := agui.ResumeEntry{InterruptID: "int-1", Status: agui.ResumeStatusResolved, Metadata: valueShapes(t, resumeValueShapesJSON)}
	data := mustJSON(t, resume)
	if err := agui.Validate("ResumeEntry", data); err != nil {
		t.Fatalf("schema rejects the entry: %v", err)
	}
	back := decodeResume(t, data)
	if back.Metadata == nil {
		t.Fatal("metadata lost")
	}
	if !jsonEqual(t, mustJSON(t, back.Metadata), []byte(resumeValueShapesJSON)) {
		t.Fatalf("metadata changed: %s", mustJSON(t, back.Metadata))
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/ResumeMetadataTest.cs::ResumeMetadataTest.Resume_OmitsAbsentMetadataFromTheWire
func TestCsResumeMetadataTest_Resume_OmitsAbsentMetadataFromTheWire(t *testing.T) {
	assertOmitted(t, obj(t, agui.ResumeEntry{InterruptID: "int-1", Status: agui.ResumeStatusResolved}), "metadata")
}

// C#: tests/AGUI.Abstractions.UnitTests/ResumeMetadataTest.cs::ResumeMetadataTest.Resume_ReadsExplicitNullAsAbsent
func TestCsResumeMetadataTest_Resume_ReadsExplicitNullAsAbsent(t *testing.T) {
	r := decodeResume(t, []byte(`{"interruptId":"int-1","status":"resolved","metadata":null}`))
	if r.Metadata != nil {
		t.Fatalf("explicit null must read as absent, got %v", r.Metadata)
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/ResumeMetadataTest.cs::ResumeMetadataTest.Resume_DistinguishesEmptyObjectFromAbsent
func TestCsResumeMetadataTest_Resume_DistinguishesEmptyObjectFromAbsent(t *testing.T) {
	r := decodeResume(t, []byte(`{"interruptId":"int-1","status":"resolved","metadata":{}}`))
	if r.Metadata == nil {
		t.Fatal("an empty object must not read as absent")
	}
	if len(r.Metadata) != 0 {
		t.Fatalf("expected an empty object, got %v", r.Metadata)
	}
}

// C#: tests/AGUI.Abstractions.UnitTests/ResumeMetadataTest.cs::ResumeMetadataTest.Resume_CarriesMetadataOnCancelledEntries
func TestCsResumeMetadataTest_Resume_CarriesMetadataOnCancelledEntries(t *testing.T) {
	r := decodeResume(t, []byte(`{"interruptId":"int-1","status":"cancelled","metadata":{"reason":"timeout"}}`))
	assertEq(t, r.Status, agui.ResumeStatusCancelled, "status")
	assertEq(t, r.Metadata["reason"], "timeout", "metadata.reason")
}

// C#: tests/AGUI.Abstractions.UnitTests/ResumeMetadataTest.cs::ResumeMetadataTest.Resume_MetadataReachesTheServerThroughRunAgentInput
//
// Read the way a Go producer reads a request body: through
// agui.ParseRunAgentInput (the processing model), which must keep resume
// metadata intact and raise no warning for it.
func TestCsResumeMetadataTest_Resume_MetadataReachesTheServerThroughRunAgentInput(t *testing.T) {
	body := []byte(`{
	  "threadId": "t-1",
	  "runId": "r-1",
	  "messages": [],
	  "resume": [
	    {
	      "interruptId": "generic-1",
	      "status": "resolved",
	      "payload": { "approved": true },
	      "metadata": { "ag-ui": {}, "definitionId": "review-plan", "key": "afterModel-review" }
	    }
	  ]
	}`)
	in, warnings, err := agui.ParseRunAgentInput(body)
	if err != nil {
		t.Fatalf("ParseRunAgentInput: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	if len(in.Resume) != 1 {
		t.Fatalf("expected one resume entry, got %+v", in.Resume)
	}
	md := in.Resume[0].Metadata
	if md == nil {
		t.Fatal("resume metadata lost")
	}
	assertEq(t, md["definitionId"], "review-plan", "definitionId")
	assertEq(t, md["key"], "afterModel-review", "key")
	objOf(t, md[reservedMetadataKey])
}
