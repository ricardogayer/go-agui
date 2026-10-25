package protobuf

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"google.golang.org/protobuf/encoding/protowire"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

func mustEncode(t *testing.T, doc string) []byte {
	t.Helper()
	b, err := EncodeJSON([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func lenField(num protowire.Number, payload []byte) []byte {
	b := protowire.AppendTag(nil, num, protowire.BytesType)
	return protowire.AppendBytes(b, payload)
}

// payloadOf returns the inner payload of a one-arm envelope.
func payloadOf(t *testing.T, frame []byte) (protowire.Number, []byte) {
	t.Helper()
	num, _, n := protowire.ConsumeTag(frame)
	v, _ := protowire.ConsumeBytes(frame[n:])
	return num, v
}

func TestTypedRoundTrip(t *testing.T) {
	events := []agui.Event{
		agui.NewRunStartedEvent("t", "r"),
		agui.NewTextMessageStartEvent("m"),
		&agui.ToolCallResultEvent{MessageID: "m2", ToolCallID: "c", Content: agui.PartsContent(agui.NewTextPart("ok"), agui.NewImagePart(&agui.URLSource{Value: "https://x"}))},
		&agui.RunFinishedEvent{ThreadID: "t", RunID: "r", Outcome: &agui.InterruptOutcome{Interrupts: []agui.Interrupt{{ID: "i", Reason: "approval"}}}},
		agui.NewStateDeltaEvent(agui.Add("/a", map[string]any{"b": []any{1, "x", nil, true}})),
	}
	for _, e := range events {
		b, err := Encode(e)
		if err != nil {
			t.Fatalf("%s: %v", e.Type(), err)
		}
		got, err := Decode(b)
		if err != nil {
			t.Fatalf("%s: %v", e.Type(), err)
		}
		if err := agui.ValidateEvent(got); err != nil {
			t.Fatalf("%s decoded to an invalid event: %v", e.Type(), err)
		}
		if got.Type() != e.Type() {
			t.Fatalf("got %s", got.Type())
		}
	}
}

func TestDecodeGuards(t *testing.T) {
	good := mustEncode(t, `{"type":"TEXT_MESSAGE_CONTENT","messageId":"m","delta":"hi"}`)
	other := mustEncode(t, `{"type":"TEXT_MESSAGE_END","messageId":"m"}`)

	t.Run("a frame naming only a future arm is an unknown event, not malformed", func(t *testing.T) {
		future := lenField(99, lenField(1, []byte("x")))
		if _, err := DecodeJSON(future); !errors.Is(err, ErrUnknownEventType) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("an empty frame is malformed", func(t *testing.T) {
		if _, err := DecodeJSON(nil); !IsMalformed(err) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("two different known events are malformed", func(t *testing.T) {
		if _, err := DecodeJSON(append(append([]byte{}, good...), other...)); !IsMalformed(err) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("field number zero is malformed", func(t *testing.T) {
		if _, err := DecodeJSON([]byte{0x02, 0x00}); err == nil {
			t.Fatal("accepted")
		}
	})
	t.Run("a length past the end is malformed", func(t *testing.T) {
		if _, err := DecodeJSON(good[:len(good)-1]); !IsMalformed(err) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("a future arm beside a known one is an unknown field and skipped", func(t *testing.T) {
		doc, err := DecodeJSON(append(append([]byte{}, good...), lenField(99, nil)...))
		if err != nil || !strings.Contains(string(doc), `"delta":"hi"`) {
			t.Fatalf("%s %v", doc, err)
		}
	})
	t.Run("the same event twice merges, last scalar wins", func(t *testing.T) {
		second := mustEncode(t, `{"type":"TEXT_MESSAGE_CONTENT","messageId":"m","delta":"bye"}`)
		doc, err := DecodeJSON(append(append([]byte{}, good...), second...))
		if err != nil || !strings.Contains(string(doc), `"delta":"bye"`) {
			t.Fatalf("%s %v", doc, err)
		}
	})
	t.Run("an envelope arm that disagrees with the base type is malformed", func(t *testing.T) {
		_, payload := payloadOf(t, good) // TEXT_MESSAGE_CONTENT payload under the TEXT_MESSAGE_END arm
		if _, err := DecodeJSON(lenField(3, payload)); !IsMalformed(err) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestDecodeLeavesJudgmentToEnforcement(t *testing.T) {
	t.Run("an unknown patch operation rides on as its number", func(t *testing.T) {
		b := mustEncode(t, `{"type":"STATE_DELTA","delta":[{"op":"remove","path":"/a"}]}`)
		// Rewrite op REMOVE (1) to 9, an operation this build cannot name.
		b = bytes.Replace(b, []byte{0x08, 0x01}, []byte{0x08, 0x09}, 1)
		doc, err := DecodeJSON(b)
		if err != nil || !strings.Contains(string(doc), `"op":"9"`) {
			t.Fatalf("%s %v", doc, err)
		}
		res, err := agui.Sanitize("Event", doc, agui.SanitizeOptions{})
		if err != nil || len(res.Warnings) == 0 {
			t.Fatalf("enforcement must strip the operation with a warning: %v %v", res, err)
		}
	})
	t.Run("an unknown outcome is rebuilt for enforcement to strip", func(t *testing.T) {
		b := mustEncode(t, `{"type":"RUN_FINISHED","threadId":"t","runId":"r","outcome":{"type":"cancelled"}}`)
		b = bytes.Replace(b, []byte("cancelled"), []byte("postponed"), 1)
		doc, err := DecodeJSON(b)
		if err != nil || !strings.Contains(string(doc), `"outcome":{"type":"postponed"}`) {
			t.Fatalf("%s %v", doc, err)
		}
	})
	t.Run("a content part of an unknown kind is dropped with a warning", func(t *testing.T) {
		b := mustEncode(t, `{"type":"TOOL_CALL_RESULT","messageId":"m","toolCallId":"c","content":[{"type":"text","text":"a"},{"type":"text","text":"b"}]}`)
		// Re-tag the second part's arm (text = 1) as 9.
		i := bytes.LastIndex(b, []byte{0x0a, 0x03, 0x0a, 0x01, 'b'})
		b[i] = 0x4a
		var warned []string
		doc, err := DecodeJSON(b, Options{Warn: func(s string) { warned = append(warned, s) }})
		if err != nil || strings.Contains(string(doc), `"b"`) || len(warned) != 1 {
			t.Fatalf("%s %v %v", doc, err, warned)
		}
	})
}

func TestFraming(t *testing.T) {
	var buf bytes.Buffer
	w := NewFrameWriter(&buf)
	frames := [][]byte{mustEncode(t, `{"type":"RUN_STARTED","threadId":"t","runId":"r"}`), {}, mustEncode(t, `{"type":"RUN_FINISHED","threadId":"t","runId":"r"}`)}
	for _, f := range frames {
		if err := w.WriteFrame(f); err != nil {
			t.Fatal(err)
		}
	}
	// One byte at a time: frames split across transport chunks.
	r := NewFrameReader(iotest.OneByteReader(bytes.NewReader(buf.Bytes())))
	for i, want := range frames {
		got, err := r.Next()
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("frame %d: %v", i, err)
		}
	}
	if _, err := r.Next(); err != io.EOF {
		t.Fatalf("want EOF, got %v", err)
	}
	r = NewFrameReader(bytes.NewReader(buf.Bytes()[:buf.Len()-2]))
	_, _ = r.Next()
	_, _ = r.Next()
	if _, err := r.Next(); !errors.Is(err, ErrTruncated) {
		t.Fatalf("a body ending mid-frame is a truncated run, got %v", err)
	}
}

func TestAdmitsProtobuf(t *testing.T) {
	cases := map[string]bool{
		"":                                  false,
		"text/event-stream":                 false,
		"application/vnd.ag-ui.event+proto": true,
		"text/event-stream, application/vnd.ag-ui.event+proto;q=0.1": true,
		"application/vnd.ag-ui.event+proto;q=0":                      false,
		"*/*":                                                        true,
		"application/*":                                              true,
		"text/*":                                                     false,
		"application/json, */*;q=0":                                  false,
		"*/*, application/vnd.ag-ui.event+proto;q=0":                 false,
		"APPLICATION/VND.AG-UI.EVENT+PROTO":                          true,
	}
	for accept, want := range cases {
		if got := AdmitsProtobuf(accept); got != want {
			t.Errorf("%q: got %v want %v", accept, got, want)
		}
	}
}
