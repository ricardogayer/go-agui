// Package protobuf implements the AG-UI 1.0 HTTP + Protobuf binding
// (https://docs.ag-ui.com/spec/1.0/basic/transports/http-protobuf): the same
// JSON POST as the SSE binding, answered — when the client's Accept admits
// application/vnd.ag-ui.event+proto — with a stream of frames, each a 4-byte
// big-endian length followed by one protobuf-encoded event.
//
// The wire schema is the generated one every first-party SDK shares (the
// proto/ directory of this package); its encoding reproduces the shared
// cross-implementation byte corpus byte for byte. Decoding turns bytes into
// the JSON event the producer sent and nothing more: like the SSE reader, it
// does not validate, so an event enters the consumer's processing pipeline
// identically whichever binding carried it.
package protobuf

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// ContentType is the media type of the binding's response.
const ContentType = "application/vnd.ag-ui.event+proto"

// ErrUnknownEventType is returned by Decode for a well-formed frame whose
// event is of a type this build was not compiled against (an envelope arm
// from a later protocol version). The binding drops such a frame with a
// warning — the answer enforcement gives a JSON event of an unknown type.
var ErrUnknownEventType = errors.New("ag-ui/protobuf: event of a type this build does not know")

// IsMalformed reports whether err is a decode failure: bytes that are not a
// well-formed event frame, which is fatal to the stream.
func IsMalformed(err error) bool { return errors.Is(err, errMalformed) }

// Options tunes encoding and decoding.
type Options struct {
	// Warn receives what translation dropped (a content part the wire cannot
	// carry, a part of a kind this build does not know) and, on encoding, a
	// document the 1.0 schema rejects.
	Warn func(string)
	// SkipValidation skips checking a document against the schema before
	// encoding it (for callers that already did, such as pkg/stream).
	SkipValidation bool
}

func (o Options) warn() func(string) {
	if o.Warn != nil {
		return o.Warn
	}
	return func(string) {}
}

// Encode encodes an event to the protobuf wire format.
func Encode(e agui.Event, opts ...Options) ([]byte, error) {
	data, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	return EncodeJSON(data, opts...)
}

// EncodeJSON encodes a JSON event document. The members of open objects
// (metadata, state, any JSON value) are written in the document's own order,
// which is what makes the bytes reproducible across implementations.
func EncodeJSON(event []byte, opts ...Options) ([]byte, error) {
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}
	doc, err := parseOrdered(event)
	if err != nil {
		return nil, err
	}
	if !o.SkipValidation {
		// As the reference encoders do: a document the schema rejects is
		// reported, and encoded as given — what the wire cannot carry (a
		// metadata that is not an object, say) is lost, so it must not be
		// lost in silence.
		if err := agui.Validate("Event", event); err != nil {
			o.warn()(fmt.Sprintf("encoding without validation: %v; encoded as given", err))
		}
	}
	obj, ok := doc.(*object)
	if !ok {
		return nil, fmt.Errorf("ag-ui/protobuf: an event must be a JSON object")
	}
	envelope, err := toWire(obj, o.warn())
	if err != nil {
		return nil, err
	}
	b, err := encodeMessage("Event", envelope)
	if err != nil {
		return nil, fmt.Errorf("ag-ui/protobuf: cannot encode %s: %w", typeOf(obj), err)
	}
	return b, nil
}

// DecodeJSON decodes one event from the protobuf wire format into the JSON
// document it carries. It returns ErrUnknownEventType for an event of a type
// this build does not know, and an error satisfying IsMalformed for bytes
// that are not a well-formed event.
func DecodeJSON(data []byte, opts ...Options) ([]byte, error) {
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}
	if err := scanMessage("Event", data, true); err != nil {
		return nil, err
	}
	envelope, err := decodeMessage("Event", data)
	if err != nil {
		return nil, err
	}
	if len(envelope.keys) == 0 {
		if namesOnlyFutureArms(data) {
			return nil, ErrUnknownEventType
		}
		return nil, malformed("the frame carries no event")
	}
	event, err := fromWire(envelope, o.warn())
	if err != nil {
		return nil, err
	}
	return marshalOrdered(event)
}

// Decode decodes one event into its Go type, without validation (use
// agui.ParseEvent on DecodeJSON's output for a schema-checked decode).
func Decode(data []byte, opts ...Options) (agui.Event, error) {
	doc, err := DecodeJSON(data, opts...)
	if err != nil {
		return nil, err
	}
	return agui.UnmarshalEvent(doc)
}

// =============================================================================
// Framing
// =============================================================================

// MaxFrameSize bounds the frames a FrameReader accepts (default 64 MiB).
var MaxFrameSize uint32 = 64 << 20

// FrameWriter writes length-prefixed frames. It flushes after every frame
// when the underlying writer is an http.Flusher.
type FrameWriter struct{ w io.Writer }

// NewFrameWriter returns a FrameWriter on w.
func NewFrameWriter(w io.Writer) *FrameWriter { return &FrameWriter{w: w} }

// WriteFrame writes one frame: a 4-byte big-endian length, then payload.
func (fw *FrameWriter) WriteFrame(payload []byte) error {
	buf := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(buf, uint32(len(payload)))
	copy(buf[4:], payload)
	if _, err := fw.w.Write(buf); err != nil {
		return fmt.Errorf("ag-ui/protobuf: write: %w", err)
	}
	if f, ok := fw.w.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}

// ErrTruncated is returned when a stream ends in the middle of a frame — a
// truncated run, not a malformed one.
var ErrTruncated = errors.New("ag-ui/protobuf: stream ended in the middle of a frame")

// FrameReader reads length-prefixed frames, tolerating frames split across
// transport chunks and several frames within one.
type FrameReader struct{ r *bufio.Reader }

// NewFrameReader returns a FrameReader on r.
func NewFrameReader(r io.Reader) *FrameReader {
	return &FrameReader{r: bufio.NewReaderSize(r, 64<<10)}
}

// Next returns the next frame's payload, io.EOF at a clean end of stream and
// ErrTruncated when the stream ends mid-frame.
func (fr *FrameReader) Next() ([]byte, error) {
	var header [4]byte
	n, err := io.ReadFull(fr.r, header[:])
	if err == io.EOF && n == 0 {
		return nil, io.EOF
	}
	if err != nil {
		return nil, ErrTruncated
	}
	size := binary.BigEndian.Uint32(header[:])
	if size > MaxFrameSize {
		return nil, fmt.Errorf("ag-ui/protobuf: frame of %d bytes exceeds the %d-byte limit", size, MaxFrameSize)
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(fr.r, payload); err != nil {
		return nil, ErrTruncated
	}
	return payload, nil
}

// =============================================================================
// Negotiation
// =============================================================================

// AdmitsProtobuf reports whether an Accept header admits the binding's media
// type with a positive quality — explicitly, or through application/* or
// */*. The most specific matching range decides; relative qualities between
// the two bindings are not consulted, as the specification prescribes.
func AdmitsProtobuf(accept string) bool {
	best, bestQ := -1, 0.0
	for _, part := range strings.Split(accept, ",") {
		mt, params, err := mime.ParseMediaType(strings.TrimSpace(part))
		if err != nil {
			continue
		}
		specificity := -1
		switch strings.ToLower(mt) {
		case ContentType:
			specificity = 2
		case "application/*":
			specificity = 1
		case "*/*":
			specificity = 0
		}
		if specificity < best || specificity < 0 {
			continue
		}
		q := 1.0
		if qs, ok := params["q"]; ok {
			if f, err := strconv.ParseFloat(qs, 64); err == nil {
				q = f
			}
		}
		if specificity > best {
			best, bestQ = specificity, q
		} else if q > bestQ {
			bestQ = q
		}
	}
	return best >= 0 && bestQ > 0
}
