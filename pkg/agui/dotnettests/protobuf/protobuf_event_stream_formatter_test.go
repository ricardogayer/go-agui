package protobuf_test

// Port of tests/AGUI.Protobuf.UnitTests/ProtobufEventStreamFormatterTest.cs.
//
// ProtobufEventStreamFormatter is the .NET stream formatter: MediaType and
// CanRead select it by Content-Type, ReadAsync is the consumer side and
// WriteAsync the producer side. In Go those are protobuf.ContentType, the
// client's choice of parser by response Content-Type (client.Agent.Run),
// client.Agent.ConsumeProtobuf and stream.NewProtobufWriter.

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
	"github.com/ricardogayer/go-agui/pkg/protobuf"
	"github.com/ricardogayer/go-agui/pkg/stream"
)

// C#: tests/AGUI.Protobuf.UnitTests/ProtobufEventStreamFormatterTest.cs::ProtobufEventStreamFormatterTest.MediaType_IsProtobufMediaType
func TestCsProtobufEventStreamFormatterTest_MediaType_IsProtobufMediaType(t *testing.T) {
	eq(t, "ContentType", protobuf.ContentType, "application/vnd.ag-ui.event+proto")
	// The producer side announces exactly that media type.
	rec := httptest.NewRecorder()
	s := stream.NewProtobuf(rec)
	if !s.Protobuf() {
		t.Fatal("NewProtobuf returned a stream that does not speak protobuf")
	}
	eq(t, "Content-Type", rec.Header().Get("Content-Type"), protobuf.ContentType)
}

// C#: tests/AGUI.Protobuf.UnitTests/ProtobufEventStreamFormatterTest.cs::ProtobufEventStreamFormatterTest.CanRead_MatchesProtobufMediaTypeOnly
func TestCsProtobufEventStreamFormatterTest_CanRead_MatchesProtobufMediaTypeOnly(t *testing.T) {
	// The consumer selects its parser by the response Content-Type. A body of
	// protobuf frames is read as protobuf only under the protobuf media type.
	var payloads [][]byte
	for _, e := range fiveEvents("t1", "r1") {
		payloads = append(payloads, encode(t, e))
	}
	body := framed(t, payloads...)
	rows := []struct {
		name        string
		contentType *string
		expected    bool
	}{
		{"application/vnd.ag-ui.event+proto", agui.Ptr("application/vnd.ag-ui.event+proto"), true},
		{"text/event-stream", agui.Ptr("text/event-stream"), false},
		{"empty", agui.Ptr(""), false},
		{"null", nil, false},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if row.contentType == nil {
					w.Header()["Content-Type"] = nil // no header, and no sniffing
				} else {
					w.Header()["Content-Type"] = []string{*row.contentType}
				}
				_, _ = w.Write(body)
			}))
			defer srv.Close()
			a := client.New(srv.URL)
			res, err := a.Run(context.Background(), client.RunInput{}, nil)
			readAsProtobuf := err == nil && res != nil && len(res.Events) == 5
			eq(t, "read as protobuf", readAsProtobuf, row.expected)
		})
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/ProtobufEventStreamFormatterTest.cs::ProtobufEventStreamFormatterTest.ReadAsync_RoundTripsFramedEvents
func TestCsProtobufEventStreamFormatterTest_ReadAsync_RoundTripsFramedEvents(t *testing.T) {
	var payloads [][]byte
	for _, e := range fiveEvents("t1", "r1") {
		payloads = append(payloads, encode(t, e))
	}
	res, _, err := consume(t, framed(t, payloads...))
	if err != nil {
		t.Fatal(err)
	}
	decoded := res.Events
	eq(t, "count", len(decoded), 5)
	asType[*agui.RunStartedEvent](t, decoded[0])
	eq(t, "Delta", asType[*agui.TextMessageContentEvent](t, decoded[2]).Delta, "hello")
	asType[*agui.RunFinishedEvent](t, decoded[4])
}

// C#: tests/AGUI.Protobuf.UnitTests/ProtobufEventStreamFormatterTest.cs::ProtobufEventStreamFormatterTest.WriteAsync_ProducesFramedBytesThatRoundTrip
func TestCsProtobufEventStreamFormatterTest_WriteAsync_ProducesFramedBytesThatRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	s := stream.NewProtobufWriter(&buf)
	for _, e := range fiveEvents("t1", "r1") {
		if err := s.Send(e); err != nil {
			t.Fatalf("Send %s: %v", e.Type(), err)
		}
	}
	decoded, err := readFramed(t, buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "count", len(decoded), 5)
	asType[*agui.RunStartedEvent](t, decoded[0])
	eq(t, "Delta", asType[*agui.TextMessageContentEvent](t, decoded[2]).Delta, "hello")
	asType[*agui.RunFinishedEvent](t, decoded[4])
}
