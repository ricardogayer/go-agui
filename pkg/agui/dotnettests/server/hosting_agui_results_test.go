package server_test

// Port of sdks/dotnet/tests/AGUI.Hosting.AspNetCore.IntegrationTests/AGUIResultsTest.cs.
//
// AGUIResults.Events negotiates the response binding from Accept: SSE, or
// HTTP + Protobuf when a protobuf formatter is registered. In Go the handler
// negotiates (stream.Negotiate); "no protobuf formatter registered" is
// server.Options.DisableProtobuf.

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/protobuf"
	"github.com/ricardogayer/go-agui/pkg/server"
	"github.com/ricardogayer/go-agui/pkg/sse"
	"github.com/ricardogayer/go-agui/pkg/stream"
)

const protoMediaType = protobuf.ContentType

// negotiated POSTs the C# SampleEvents() run (RUN_STARTED, RUN_FINISHED) with
// accept (nil: no Accept header) and returns the response and its body.
func negotiated(t *testing.T, accept *string, registerProto bool) (*http.Response, []byte) {
	t.Helper()
	h := server.NewAgentHandler(agent(t, func(in *agui.RunAgentInput, s *stream.SSE, must func(error)) {
		must(s.RunStarted(in.ThreadID, in.RunID))
		must(s.RunFinished(in.ThreadID, in.RunID))
	}), server.Options{Logger: t.Logf, DisableProtobuf: !registerProto})
	srv := httptest.NewServer(h)
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(`{"threadId":"t1","runId":"r1","protocolVersion":"1.0","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	if accept != nil {
		req.Header.Set("Accept", *accept)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp, body
}

// protoEvents decodes a protobuf-binding body into validated events.
func protoEvents(t *testing.T, body []byte) []agui.Event {
	t.Helper()
	r := protobuf.NewFrameReader(bytes.NewReader(body))
	var out []agui.Event
	for {
		frame, err := r.Next()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		data, err := protobuf.DecodeJSON(frame)
		if err != nil {
			t.Fatal(err)
		}
		e, err := agui.ParseEvent(data)
		if err != nil {
			t.Fatalf("frame is not a valid 1.0 event: %v", err)
		}
		out = append(out, e)
	}
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/AGUIResultsTest.cs::AGUIResultsTest.Events_SelectsExpectedFormatter
func TestCsAGUIResultsTest_Events_SelectsExpectedFormatter(t *testing.T) {
	rows := []struct {
		accept        *string
		registerProto bool
		expected      string
	}{
		{agui.Ptr(protoMediaType), true, protoMediaType},
		{agui.Ptr(protoMediaType + ", text/event-stream"), true, protoMediaType},
		{agui.Ptr("text/event-stream, " + protoMediaType), true, protoMediaType},
		{agui.Ptr(protoMediaType + ", text/event-stream"), false, sse.ContentType},
		{agui.Ptr("text/event-stream"), true, sse.ContentType},
		{agui.Ptr("text/event-stream"), false, sse.ContentType},
		// Adapted: the C# row expects SSE. http-protobuf.mdx: "a wildcard
		// range (*/*, application/*) admits the media type too", and a
		// producer that supports the binding "SHOULD answer with it whenever
		// the client's Accept admits the media type with a positive quality".
		{agui.Ptr("*/*"), true, protoMediaType},
		{agui.Ptr("*/*"), false, sse.ContentType},
		{agui.Ptr("text/*"), true, sse.ContentType},
		{nil, true, sse.ContentType},
		{nil, false, sse.ContentType},
		{agui.Ptr(protoMediaType + ";q=0, text/event-stream"), true, sse.ContentType},
	}
	for i, row := range rows {
		t.Run(fmt.Sprintf("%02d_accept=%s_proto=%v", i+1, agui.Deref(row.accept), row.registerProto), func(t *testing.T) {
			resp, body := negotiated(t, row.accept, row.registerProto)
			if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != row.expected {
				t.Fatalf("status %d, Content-Type %q, want %q", resp.StatusCode, resp.Header.Get("Content-Type"), row.expected)
			}
			if row.expected == protoMediaType {
				if got := types(protoEvents(t, body)); strings.Join(got, ",") != "RUN_STARTED,RUN_FINISHED" {
					t.Fatalf("events %v", got)
				}
			}
		})
	}
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/AGUIResultsTest.cs::AGUIResultsTest.Events_Returns406_WhenProtoOnlyAndNoProtoFormatter
func TestCsAGUIResultsTest_Events_Returns406_WhenProtoOnlyAndNoProtoFormatter(t *testing.T) {
	for _, accept := range []string{protoMediaType, protoMediaType + ", application/json"} {
		t.Run(accept, func(t *testing.T) {
			// Adapted: 406 is the .NET result's choice. http-protobuf.mdx: a
			// producer that does not support the binding "ignores the media
			// type — which is why a client MUST always be prepared to receive
			// SSE".
			resp, _ := negotiated(t, agui.Ptr(accept), false)
			if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != sse.ContentType {
				t.Fatalf("status %d, Content-Type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
			}
		})
	}
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/AGUIResultsTest.cs::AGUIResultsTest.Events_Returns406_WhenNoAcceptableMediaType
func TestCsAGUIResultsTest_Events_Returns406_WhenNoAcceptableMediaType(t *testing.T) {
	// Adapted: the specification defines no rejection for an Accept that
	// admits neither binding (http-protobuf.mdx: "MUST answer SSE otherwise").
	resp, _ := negotiated(t, agui.Ptr("application/json"), true)
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != sse.ContentType {
		t.Fatalf("status %d, Content-Type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/AGUIResultsTest.cs::AGUIResultsTest.Events_WritesFramedProto_WhenProtoNegotiated
func TestCsAGUIResultsTest_Events_WritesFramedProto_WhenProtoNegotiated(t *testing.T) {
	resp, body := negotiated(t, agui.Ptr(protoMediaType), true)
	if resp.Header.Get("Content-Type") != protoMediaType {
		t.Fatalf("Content-Type %q", resp.Header.Get("Content-Type"))
	}
	// A 4-byte big-endian length prefix per event.
	if len(body) <= 4 {
		t.Fatalf("body %d bytes", len(body))
	}
	first := binary.BigEndian.Uint32(body[:4])
	if first == 0 || int(first) > len(body)-4 {
		t.Fatalf("first frame length %d", first)
	}
}

// C#: tests/AGUI.Hosting.AspNetCore.IntegrationTests/AGUIResultsTest.cs::AGUIResultsTest.Events_SseBody_IsByteIdenticalToDataJsonShape
func TestCsAGUIResultsTest_Events_SseBody_IsByteIdenticalToDataJsonShape(t *testing.T) {
	var buf bytes.Buffer
	s := stream.NewWriter(&buf)
	evt := &agui.RunStartedEvent{ThreadID: "t1", RunID: "r1", ProtocolVersion: agui.Ptr(agui.ProtocolVersion)}
	if err := s.Send(evt); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(evt)
	if got, want := buf.String(), "data: "+string(payload)+"\n\n"; got != want {
		t.Fatalf("SSE body\n got: %q\nwant: %q", got, want)
	}
}
