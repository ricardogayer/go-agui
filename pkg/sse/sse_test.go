package sse

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func readAll(t *testing.T, in string) ([]string, error) {
	t.Helper()
	r := NewReader(strings.NewReader(in))
	var out []string
	for {
		d, err := r.Next()
		if err != nil {
			if err == io.EOF {
				return out, nil
			}
			return out, err
		}
		out = append(out, string(d))
	}
}

func TestReaderGrammar(t *testing.T) {
	in := ": keep-alive\n\n" +
		"event: ignored\nid: 7\nretry: 100\ndata: {\"a\":1}\n\n" +
		"data:{\"b\":2}\r\n\r\n" +
		"data: line1\rdata: line2\r\r" +
		"data: x\n\n"
	got, err := readAll(t, in)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{`{"a":1}`, `{"b":2}`, "line1\nline2", "x"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q", got)
	}
}

func TestReaderTruncated(t *testing.T) {
	got, err := readAll(t, "data: {\"a\":1}\n\ndata: {\"b\"")
	if !errors.Is(err, ErrTruncated) || len(got) != 1 {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestWriterRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	_ = w.WriteComment("ping")
	_ = w.WriteData([]byte(`{"type":"RAW","event":"a"}`))
	_ = w.WriteData([]byte("multi\nline"))
	if strings.Contains(buf.String(), "\r") {
		t.Fatal("writer must use LF")
	}
	got, err := readAll(t, buf.String())
	if err != nil || len(got) != 2 || got[1] != "multi\nline" {
		t.Fatalf("got %q, %v", got, err)
	}
}
