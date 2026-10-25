package client_test

// Port of sdks/dotnet/tests/AGUI.Client.UnitTests/AGUIEventStreamHandlerTest.cs.
//
// The .NET AGUIEventStreamHandler negotiates the binding: it advertises every
// registered formatter in Accept and reads the response with the formatter
// matching its Content-Type. The Go client implements only the SSE binding
// (HTTP + Protobuf is OPTIONAL in transports/index.mdx and out of scope), so
// these tests check that negotiation from the Go side: what Accept is sent,
// and what happens to a response in another media type.

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
	"github.com/ricardogayer/go-agui/pkg/protobuf"
	"github.com/ricardogayer/go-agui/pkg/sse"
)

// C#: tests/AGUI.Client.UnitTests/AGUIEventStreamHandlerTest.cs::AGUIEventStreamHandlerTest.SendAsync_SetsAcceptHeaderFromFormattersInOrder
//
// The request advertises every binding the client can read (http-sse.mdx:
// "Accept: text/event-stream (adding the protobuf media type when it can
// consume that binding too)"); relative order carries no meaning, since the
// specification says qualities between the two are not consulted. A client
// configured without the protobuf binding advertises SSE only.
func TestCsAGUIEventStreamHandlerTest_SendAsync_SetsAcceptHeaderFromFormattersInOrder(t *testing.T) {
	stub := newStub(t, echo())
	agent, _ := newAgent(stub.URL)
	if _, err := agent.Run(t.Context(), client.RunInput{}, nil); err != nil {
		t.Fatal(err)
	}
	r := stub.lastRequest(t)
	if r.Method != http.MethodPost {
		t.Errorf("method %s, want POST", r.Method)
	}
	if got := r.Header.Get("Accept"); got != "text/event-stream, "+protobuf.ContentType {
		t.Errorf("Accept = %q, want both bindings", got)
	}
	if got := r.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}

	sseOnly, _ := newAgent(stub.URL)
	sseOnly.DisableProtobuf = true
	if _, err := sseOnly.Run(t.Context(), client.RunInput{}, nil); err != nil {
		t.Fatal(err)
	}
	if got := stub.lastRequest(t).Header.Get("Accept"); got != "text/event-stream" {
		t.Errorf("Accept = %q, want text/event-stream", got)
	}
}

// C#: tests/AGUI.Client.UnitTests/AGUIEventStreamHandlerTest.cs::AGUIEventStreamHandlerTest.SendAsync_RecordsChosenFormatterForResponseContentType
//
// The parser is chosen by the response's Content-Type: a protobuf response is
// read as length-prefixed protobuf frames.
func TestCsAGUIEventStreamHandlerTest_SendAsync_RecordsChosenFormatterForResponseContentType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", protobuf.ContentType)
		fw := protobuf.NewFrameWriter(w)
		for _, e := range []agui.Event{&agui.RunStartedEvent{ThreadID: "t1", RunID: "r1"}, &agui.RunFinishedEvent{ThreadID: "t1", RunID: "r1"}} {
			b, err := protobuf.Encode(e)
			if err != nil {
				t.Error(err)
				return
			}
			_ = fw.WriteFrame(b)
		}
	}))
	defer srv.Close()
	agent, _ := newAgent(srv.URL)
	var delivered []agui.Event
	res, err := agent.Run(t.Context(), client.RunInput{}, func(e agui.Event) { delivered = append(delivered, e) })
	if err != nil {
		t.Fatal(err)
	}
	if len(delivered) != 2 || delivered[0].Type() != agui.EventTypeRunStarted || !res.Succeeded() {
		t.Fatalf("delivered %v, status %s", delivered, res.Status)
	}

	// Without the binding, a protobuf response is refused, never misread.
	sseOnly, _ := newAgent(srv.URL)
	sseOnly.DisableProtobuf = true
	if _, err := sseOnly.Run(t.Context(), client.RunInput{}, nil); err == nil || !strings.Contains(err.Error(), protobuf.ContentType) {
		t.Fatalf("got %v", err)
	}
}

// C#: tests/AGUI.Client.UnitTests/AGUIEventStreamHandlerTest.cs::AGUIEventStreamHandlerTest.ReadAGUIEventStreamAsync_WithoutHandler_DefaultsToSse
//
// The .NET reader decodes the frames without sequencing them; so does the Go
// transport layer (sse.Reader + agui.ParseEvent). The stream here carries a
// TEXT_MESSAGE_CONTENT with no TEXT_MESSAGE_START, so the full consumer
// pipeline must additionally reject it (text-messages.mdx sequencing) — that
// is asserted too.
func TestCsAGUIEventStreamHandlerTest_ReadAGUIEventStreamAsync_WithoutHandler_DefaultsToSse(t *testing.T) {
	events := []agui.Event{
		&agui.RunStartedEvent{ThreadID: "t1", RunID: "r1"},
		&agui.TextMessageContentEvent{MessageID: "m1", Delta: "Hi"},
		&agui.RunFinishedEvent{ThreadID: "t1", RunID: "r1"},
	}
	body := buildSse(t, events)

	r := sse.NewReader(strings.NewReader(body))
	var results []agui.Event
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
		results = append(results, e)
	}
	if len(results) != 3 {
		t.Fatalf("read %d events", len(results))
	}
	if _, ok := results[0].(*agui.RunStartedEvent); !ok {
		t.Errorf("results[0] is %T", results[0])
	}
	if c, ok := results[1].(*agui.TextMessageContentEvent); !ok || c.Delta != "Hi" {
		t.Errorf("results[1] = %#v", results[1])
	}
	if _, ok := results[2].(*agui.RunFinishedEvent); !ok {
		t.Errorf("results[2] is %T", results[2])
	}

	agent, _ := newAgent("unused")
	if _, err := agent.Consume(strings.NewReader(body), nil); err == nil {
		t.Error("the consumer pipeline accepted TEXT_MESSAGE_CONTENT without TEXT_MESSAGE_START")
	}
}

// buildSse frames events as the SSE binding does.
func buildSse(t *testing.T, events []agui.Event) string {
	t.Helper()
	var buf bytes.Buffer
	w := sse.NewWriter(&buf)
	for _, e := range events {
		b, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		if err := w.WriteData(b); err != nil {
			t.Fatal(err)
		}
	}
	return buf.String()
}
