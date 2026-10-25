// Package sse implements the framing of the AG-UI HTTP + Server-Sent Events
// binding (https://docs.ag-ui.com/spec/1.0/basic/transports/http-sse): each
// SSE event's data payload is exactly one protocol event as a JSON object,
// lines end in LF, and consumers ignore every field but data and tolerate
// comment lines.
package sse

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ContentType is the media type of the SSE binding's response.
const ContentType = "text/event-stream"

// ProtobufContentType is the media type of the optional HTTP + Protobuf
// binding. This module does not implement that binding: producers answer SSE
// whatever the client's Accept header says, which the specification permits.
const ProtobufContentType = "application/vnd.ag-ui.event+proto"

// SetHeaders writes the response headers of the SSE binding.
func SetHeaders(h http.Header) {
	h.Set("Content-Type", ContentType)
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no") // disable proxy buffering (nginx)
}

// Writer writes SSE frames. It is not safe for concurrent use.
type Writer struct {
	w io.Writer
}

// NewWriter returns a Writer on w. When w is an http.Flusher every frame is
// flushed as soon as it is written.
func NewWriter(w io.Writer) *Writer { return &Writer{w: w} }

// WriteData writes one SSE event whose data is payload. A payload containing
// line breaks is split over several data lines, which a consumer joins back
// with LF, as SSE specifies.
func (w *Writer) WriteData(payload []byte) error {
	var buf bytes.Buffer
	payload = bytes.ReplaceAll(payload, []byte("\r\n"), []byte("\n"))
	payload = bytes.ReplaceAll(payload, []byte("\r"), []byte("\n"))
	for _, line := range bytes.Split(payload, []byte("\n")) {
		buf.WriteString("data: ")
		buf.Write(line)
		buf.WriteByte('\n')
	}
	buf.WriteByte('\n')
	return w.write(buf.Bytes())
}

// WriteComment writes an SSE comment line (": text"), such as a keep-alive.
func (w *Writer) WriteComment(text string) error {
	text = strings.NewReplacer("\r", " ", "\n", " ").Replace(text)
	return w.write([]byte(": " + text + "\n\n"))
}

func (w *Writer) write(b []byte) error {
	if _, err := w.w.Write(b); err != nil {
		return fmt.Errorf("sse: write: %w", err)
	}
	if f, ok := w.w.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}

// Reader reads the data payloads of an SSE stream. It accepts the full SSE
// line grammar (LF, CRLF and CR line endings), ignores fields other than data
// (event, id, retry) and comment lines, and joins multi-line data with LF.
type Reader struct {
	r   *bufio.Reader
	err error
}

// NewReader returns a Reader on r.
func NewReader(r io.Reader) *Reader {
	return &Reader{r: bufio.NewReaderSize(r, 64*1024)}
}

// ErrTruncated is returned when the stream ends in the middle of an event.
var ErrTruncated = errors.New("sse: stream ended in the middle of an event")

// Next returns the data of the next event. It returns io.EOF at a clean end
// of stream.
func (r *Reader) Next() ([]byte, error) {
	if r.err != nil {
		return nil, r.err
	}
	var data []byte
	hasData := false
	for {
		line, err := r.readLine()
		if err != nil {
			// SSE dispatches an event only on a blank line: data pending at
			// the end of the stream belongs to an event that never finished.
			if err == io.EOF && hasData {
				err = ErrTruncated
			}
			r.err = err
			return nil, err
		}
		if line == "" {
			if hasData {
				return data, nil
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, found := strings.Cut(line, ":")
		if found {
			value = strings.TrimPrefix(value, " ")
		}
		if field != "data" {
			continue
		}
		if hasData {
			data = append(data, '\n')
		}
		data = append(data, value...)
		hasData = true
	}
}

func (r *Reader) readLine() (string, error) {
	var sb strings.Builder
	for {
		b, err := r.r.ReadByte()
		if err != nil {
			if err == io.EOF && sb.Len() > 0 {
				return sb.String(), nil
			}
			return "", err
		}
		switch b {
		case '\n':
			return sb.String(), nil
		case '\r':
			if next, err := r.r.Peek(1); err == nil && next[0] == '\n' {
				_, _ = r.r.ReadByte()
			}
			return sb.String(), nil
		default:
			sb.WriteByte(b)
		}
	}
}
