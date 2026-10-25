package protobuf_test

// Port of tests/AGUI.Protobuf.UnitTests/FramingTest.cs.
//
// AGUIProtobuf.WriteFramed is protobuf.FrameWriter; AGUIProtobuf.ReadFramedAsync
// splits into protobuf.FrameReader (frames) + protobuf.Decode (events), and
// the "skip an unknown event, keep reading" policy lives in the client's
// protobuf consumer (client.Agent.ConsumeProtobuf), which is what those
// tests drive.

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/client"
	"github.com/ricardogayer/go-agui/pkg/protobuf"
)

// readFramed reads every frame of data and decodes it (ReadFramedAsync
// without the unknown-event policy, which client.ConsumeProtobuf owns).
func readFramed(t *testing.T, data []byte) ([]agui.Event, error) {
	t.Helper()
	r := protobuf.NewFrameReader(bytes.NewReader(data))
	var out []agui.Event
	for {
		frame, err := r.Next()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		e, err := protobuf.Decode(frame)
		if err != nil {
			return out, err
		}
		out = append(out, e)
	}
}

// consume runs framed bytes through the client's protobuf consumer.
func consume(t *testing.T, data []byte) (*client.RunResult, []string, error) {
	t.Helper()
	var warnings []string
	a := &client.Agent{Warn: func(w agui.Warning) { warnings = append(warnings, w.Message) }}
	res, err := a.ConsumeProtobuf(bytes.NewReader(data), nil)
	return res, warnings, err
}

func fiveEvents(thread, run string) []agui.Event {
	return []agui.Event{
		// A producer declares the protocol version it speaks (versioning.mdx).
		&agui.RunStartedEvent{ThreadID: thread, RunID: run, ProtocolVersion: agui.Ptr(agui.ProtocolVersion)},
		&agui.TextMessageStartEvent{MessageID: "m1", Role: agui.Ptr(agui.TextMessageRoleAssistant)},
		&agui.TextMessageContentEvent{MessageID: "m1", Delta: "hello"},
		&agui.TextMessageEndEvent{MessageID: "m1"},
		&agui.RunFinishedEvent{ThreadID: thread, RunID: run},
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/FramingTest.cs::FramingTest.EncodeToBufferWriter_MatchesEncodeToArray
func TestCsFramingTest_EncodeToBufferWriter_MatchesEncodeToArray(t *testing.T) {
	// Go has one Encode (no IBufferWriter overload); its two entry points —
	// the Go event and the JSON document it marshals to — must agree.
	evt := &agui.TextMessageContentEvent{MessageID: "m1", Delta: "hi"}
	expected := encode(t, evt)
	doc, err := json.Marshal(evt)
	if err != nil {
		t.Fatal(err)
	}
	got := encodeDoc(t, string(doc))
	if !bytes.Equal(expected, got) {
		t.Fatalf("Encode %x != EncodeJSON %x", expected, got)
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/FramingTest.cs::FramingTest.WriteFramed_PrependsBigEndianLengthPrefix
func TestCsFramingTest_WriteFramed_PrependsBigEndianLengthPrefix(t *testing.T) {
	message := encode(t, &agui.TextMessageContentEvent{MessageID: "m1", Delta: "hi"})
	f := framed(t, message)
	eq(t, "len(framed)", len(f), len(message)+4)
	eq(t, "length prefix", binary.BigEndian.Uint32(f[:4]), uint32(len(message)))
	if !bytes.Equal(f[4:], message) {
		t.Fatalf("payload %x != %x", f[4:], message)
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/FramingTest.cs::FramingTest.ReadFramedAsync_ReadsAllWrittenEvents
func TestCsFramingTest_ReadFramedAsync_ReadsAllWrittenEvents(t *testing.T) {
	var payloads [][]byte
	for _, e := range fiveEvents("t", "r") {
		payloads = append(payloads, encode(t, e))
	}
	decoded, err := readFramed(t, framed(t, payloads...))
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "count", len(decoded), 5)
	asType[*agui.RunStartedEvent](t, decoded[0])
	eq(t, "Delta", asType[*agui.TextMessageContentEvent](t, decoded[2]).Delta, "hello")
	asType[*agui.RunFinishedEvent](t, decoded[4])
}

// C#: tests/AGUI.Protobuf.UnitTests/FramingTest.cs::FramingTest.ReadFramedAsync_SkipsAnUnknownEventArmAndKeepsReading
func TestCsFramingTest_ReadFramedAsync_SkipsAnUnknownEventArmAndKeepsReading(t *testing.T) {
	// The framed reader answers an unknown event the way the SSE reader does:
	// the frame is skipped (with a warning) and the stream carries on.
	data := framed(t,
		encode(t, &agui.RunStartedEvent{ThreadID: "t", RunID: "r"}),
		[]byte{0xE2, 0x38, 0x00}, // field 900, length-delimited, empty
		encode(t, &agui.RunFinishedEvent{ThreadID: "t", RunID: "r"}),
	)
	// The codec names the middle frame an unknown event, not malformed bytes.
	if _, err := protobuf.Decode([]byte{0xE2, 0x38, 0x00}); !errors.Is(err, protobuf.ErrUnknownEventType) {
		t.Fatalf("Decode(field 900) = %v, want ErrUnknownEventType", err)
	}
	res, warnings, err := consume(t, data)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "len(Events)", len(res.Events), 2)
	asType[*agui.RunStartedEvent](t, res.Events[0])
	asType[*agui.RunFinishedEvent](t, res.Events[1])
	eq(t, "Status", res.Status, client.StatusSucceeded)
	if len(warnings) == 0 {
		t.Fatal("dropping the unknown frame must warn")
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/FramingTest.cs::FramingTest.ReadFramedAsync_MalformedFrame_Throws
func TestCsFramingTest_ReadFramedAsync_MalformedFrame_Throws(t *testing.T) {
	// Broken bytes are not an event from the future, and the reader must not
	// swallow them: an envelope naming a known event it cannot read is fatal.
	data := framed(t,
		encode(t, &agui.RunStartedEvent{ThreadID: "t", RunID: "r"}),
		[]byte{0x08, 0x01}, // envelope field 1 as a varint
	)
	if _, err := readFramed(t, data); !protobuf.IsMalformed(err) {
		t.Fatalf("readFramed: got %v, want a malformed-frame error", err)
	}
	if _, _, err := consume(t, data); err == nil {
		t.Fatal("ConsumeProtobuf accepted a malformed frame")
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/FramingTest.cs::FramingTest.ReadFramedAsync_EmptyStream_YieldsNothing
func TestCsFramingTest_ReadFramedAsync_EmptyStream_YieldsNothing(t *testing.T) {
	if _, err := protobuf.NewFrameReader(bytes.NewReader(nil)).Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("Next on an empty stream = %v, want io.EOF", err)
	}
	decoded, err := readFramed(t, nil)
	if err != nil || len(decoded) != 0 {
		t.Fatalf("got %d events, %v", len(decoded), err)
	}
}

// C#: tests/AGUI.Protobuf.UnitTests/FramingTest.cs::FramingTest.ReadFramedAsync_TruncatedPayload_Throws
func TestCsFramingTest_ReadFramedAsync_TruncatedPayload_Throws(t *testing.T) {
	// EndOfStreamException in .NET; in Go the reader reports ErrTruncated, and
	// the spec makes a body ending mid-frame a truncated run (not malformed).
	f := framed(t, encode(t, &agui.TextMessageContentEvent{MessageID: "m1", Delta: "hello world"}))
	_, err := readFramed(t, f[:len(f)-2])
	if !errors.Is(err, protobuf.ErrTruncated) {
		t.Fatalf("got %v, want ErrTruncated", err)
	}
	if protobuf.IsMalformed(err) {
		t.Fatal("a truncated body is not a malformed frame")
	}
	if !strings.Contains(err.Error(), "middle of a frame") {
		t.Fatalf("unexpected error text %q", err)
	}
}
