package client_test

// Port of sdks/dotnet/tests/AGUI.Formatting.UnitTests/SseEventStreamFormatterTest.cs.
//
// The .NET SseEventStreamFormatter frames typed events as SSE and reads them
// back. In Go the framing is pkg/sse (Writer/Reader); typed decoding is
// agui.ParseEvent, and the processing model applied to a peer's stream
// (unknown material dropped, malformed known values fatal) is the
// client.Pipeline / client.Agent.

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
	"github.com/ricardogayer/go-agui/pkg/sse"
)

func formatterEvents() []agui.Event {
	return []agui.Event{
		&agui.RunStartedEvent{ThreadID: "t1", RunID: "r1"},
		&agui.TextMessageContentEvent{MessageID: "m1", Delta: "hi"},
		&agui.RunFinishedEvent{ThreadID: "t1", RunID: "r1"},
	}
}

// C#: tests/AGUI.Formatting.UnitTests/SseEventStreamFormatterTest.cs::SseEventStreamFormatterTest.MediaType_IsServerSentEvents
func TestCsSseEventStreamFormatterTest_MediaType_IsServerSentEvents(t *testing.T) {
	if sse.ContentType != "text/event-stream" {
		t.Fatalf("got %q", sse.ContentType)
	}
	h := http.Header{}
	sse.SetHeaders(h)
	if h.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("SetHeaders wrote Content-Type %q", h.Get("Content-Type"))
	}
}

// C#: tests/AGUI.Formatting.UnitTests/SseEventStreamFormatterTest.cs::SseEventStreamFormatterTest.CanRead_MatchesServerSentEvents
//
// Go has no formatter registry: the client reads a response as SSE when its
// Content-Type is text/event-stream (case-insensitively, parameters allowed)
// and refuses anything else before parsing. The .NET formatter also accepts a
// response with no Content-Type (null / "") as SSE; that leniency is not
// required by the specification (http-sse.mdx: "A run that starts answers 200
// with Content-Type: text/event-stream"), and the Go client refuses a response
// that does not declare the binding's media type. Those two rows are adapted
// to that rule.
func TestCsSseEventStreamFormatterTest_CanRead_MatchesServerSentEvents(t *testing.T) {
	rows := []struct {
		name        string
		contentType *string
		expected    bool
	}{
		{"null", nil, false},         // .NET: true (no media type read as SSE); adapted, see above
		{"empty", strPtr(""), false}, // .NET: true; adapted, see above
		{"text/event-stream", strPtr("text/event-stream"), true},
		{"TEXT/EVENT-STREAM", strPtr("TEXT/EVENT-STREAM"), true},
		{"application/json", strPtr("application/json"), false},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if row.contentType == nil {
					w.Header()["Content-Type"] = nil // suppress sniffing: no header at all
				} else {
					w.Header()["Content-Type"] = []string{*row.contentType}
				}
				_, _ = io.WriteString(w, "data: {\"type\":\"RUN_STARTED\",\"threadId\":\"t1\",\"runId\":\"r1\"}\n\n"+
					"data: {\"type\":\"RUN_FINISHED\",\"threadId\":\"t1\",\"runId\":\"r1\"}\n\n")
			}))
			defer srv.Close()
			agent, _ := newAgent(srv.URL)
			res, err := agent.Run(t.Context(), client.RunInput{}, nil)
			if row.expected {
				if err != nil || !res.Succeeded() {
					t.Fatalf("expected the response to be read as SSE: %v %+v", err, res)
				}
				return
			}
			if err == nil {
				t.Fatal("expected the response to be refused as not SSE")
			}
			if !strings.Contains(err.Error(), "Content-Type") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func strPtr(s string) *string { return &s }

// C#: tests/AGUI.Formatting.UnitTests/SseEventStreamFormatterTest.cs::SseEventStreamFormatterTest.WriteAsync_ProducesDataJsonShapeForEachEvent
func TestCsSseEventStreamFormatterTest_WriteAsync_ProducesDataJsonShapeForEachEvent(t *testing.T) {
	events := formatterEvents()
	var body bytes.Buffer
	w := sse.NewWriter(&body)
	var expected strings.Builder
	for _, e := range events {
		b, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		if err := w.WriteData(b); err != nil {
			t.Fatal(err)
		}
		expected.WriteString("data: " + string(b) + "\n\n")
	}
	if body.String() != expected.String() {
		t.Fatalf("got\n%q\nwant\n%q", body.String(), expected.String())
	}
}

// C#: tests/AGUI.Formatting.UnitTests/SseEventStreamFormatterTest.cs::SseEventStreamFormatterTest.ReadAsync_RoundTripsWrittenEvents
func TestCsSseEventStreamFormatterTest_ReadAsync_RoundTripsWrittenEvents(t *testing.T) {
	events := formatterEvents()
	var body bytes.Buffer
	w := sse.NewWriter(&body)
	for _, e := range events {
		b, _ := json.Marshal(e)
		if err := w.WriteData(b); err != nil {
			t.Fatal(err)
		}
	}
	r := sse.NewReader(&body)
	var read []agui.Event
	for {
		data, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		e, err := agui.ParseEvent(data)
		if err != nil {
			t.Fatal(err)
		}
		read = append(read, e)
	}
	if len(read) != len(events) {
		t.Fatalf("read %d events, want %d", len(read), len(events))
	}
	if _, ok := read[0].(*agui.RunStartedEvent); !ok {
		t.Errorf("read[0] is %T", read[0])
	}
	if _, ok := read[1].(*agui.TextMessageContentEvent); !ok {
		t.Errorf("read[1] is %T", read[1])
	}
	if _, ok := read[2].(*agui.RunFinishedEvent); !ok {
		t.Errorf("read[2] is %T", read[2])
	}
}

// C#: tests/AGUI.Formatting.UnitTests/SseEventStreamFormatterTest.cs::SseEventStreamFormatterTest.ReadAsync_SkipsAnUnknownEventTypeAndKeepsReading
//
// The processing model (processing.mdx): an event of a type this version does
// not describe is removed, and the stream carries on.
func TestCsSseEventStreamFormatterTest_ReadAsync_SkipsAnUnknownEventTypeAndKeepsReading(t *testing.T) {
	body := "data: {\"type\":\"RUN_STARTED\",\"threadId\":\"t1\",\"runId\":\"r1\"}\n\n" +
		"data: {\"type\":\"SOMETHING_NEWER\",\"whatever\":1}\n\n" +
		"data: {\"type\":\"RUN_FINISHED\",\"threadId\":\"t1\",\"runId\":\"r1\"}\n\n"
	agent, warnings := newAgent("unused")
	res, err := agent.Consume(strings.NewReader(body), nil)
	if err != nil {
		t.Fatalf("an unknown event type must not end the stream: %v", err)
	}
	if got := types(res.Events); !eqStrings(got, []string{"RUN_STARTED", "RUN_FINISHED"}) {
		t.Fatalf("delivered %v", got)
	}
	if !res.Succeeded() {
		t.Fatalf("status %s", res.Status)
	}
	if len(*warnings) == 0 {
		t.Error("dropping an unknown event should be announced with a warning")
	}
}

// C#: tests/AGUI.Formatting.UnitTests/SseEventStreamFormatterTest.cs::SseEventStreamFormatterTest.ReadAsync_ThrowsForAMalformedKnownEvent
func TestCsSseEventStreamFormatterTest_ReadAsync_ThrowsForAMalformedKnownEvent(t *testing.T) {
	body := "data: {\"type\":\"RUN_STARTED\",\"threadId\":[]}\n\n"
	agent, _ := newAgent("unused")
	_, err := agent.Consume(strings.NewReader(body), nil)
	var ve *agui.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("a malformed known event must be fatal with a validation error, got %v", err)
	}
	// The typed decoder agrees on its own.
	if _, err := agui.ParseEvent([]byte(`{"type":"RUN_STARTED","threadId":[]}`)); err == nil {
		t.Fatal("ParseEvent accepted a malformed RUN_STARTED")
	}
}
