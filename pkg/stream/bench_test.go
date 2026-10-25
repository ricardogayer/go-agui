package stream

import (
	"fmt"
	"io"
	"testing"
)

func BenchmarkLongRun(b *testing.B) {
	for i := 0; i < b.N; i++ {
		s := NewWriter(io.Discard)
		_ = s.RunStarted("t", "r")
		for m := 0; m < 2000; m++ {
			id := fmt.Sprint("m", m)
			_ = s.TextStart(id)
			_ = s.TextContent(id, "x")
			_ = s.TextEnd(id)
		}
		_ = s.RunFinished("t", "r")
	}
}
