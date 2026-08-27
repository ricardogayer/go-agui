package agui

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

// failingWriter returns an error after n successful writes, imitating a
// client that disconnects in the middle of the stream.
//
// PT: failingWriter devolve erro depois de n escritas bem-sucedidas, imitando um
// cliente que desconecta no meio do stream.
type failingWriter struct {
	remaining int
	err       error
}

func (w *failingWriter) Write(p []byte) (int, error) {
	if w.remaining <= 0 {
		return 0, w.err
	}
	w.remaining--
	return len(p), nil
}

// TestIsTransportErrorRecognizesRealWriteFailure is the canary test for the
// coupling to the SDK's strings.
//
// isTransportError tells "the client went away" apart from "the event is
// invalid" by matching the messages the SDK's pkg/encoding/sse produces. If an
// SDK update changes those texts, the classification silently flips: an
// invalid event would be read as a disconnect and the run would die without
// RUN_ERROR, with the symptom "the conversation simply stopped".
//
// This test forces a REAL write failure — it does not build the string by
// hand — precisely so that the breakage shows up here, in CI, and not in
// production.
//
// PT: TestIsTransportErrorRecognizesRealWriteFailure é o teste-canário do
// acoplamento a strings do SDK.
//
// isTransportError distingue "o cliente foi embora" de "o evento é inválido"
// casando as mensagens que o pkg/encoding/sse do SDK produz. Se uma atualização
// do SDK mudar esses textos, a classificação silenciosamente inverte: um evento
// inválido passaria a ser lido como desconexão e o run morreria sem RUN_ERROR,
// com sintoma de "a conversa simplesmente parou".
//
// Este teste força uma falha de escrita DE VERDADE — não constrói a string à
// mão — justamente para que a quebra apareça aqui, em CI, e não em produção.
func TestIsTransportErrorRecognizesRealWriteFailure(t *testing.T) {
	writeErr := errors.New("broken pipe")
	w := &failingWriter{remaining: 0, err: writeErr}

	e := NewEmitter(w, WithEmitterIDs("t1", "r1"))
	e.RunStarted()

	err := e.Err()
	if err == nil {
		t.Fatal("a failing write must be classified as a transport error;\n" +
			"if this test broke after updating the AG-UI SDK, check the\n" +
			"messages in pkg/encoding/sse/writer.go against isTransportError")
	}
	if !errors.Is(err, ErrClientGone) {
		t.Fatalf("error = %v, want it to wrap ErrClientGone", err)
	}
	if !errors.Is(err, writeErr) {
		t.Errorf("the original error was lost in the wrapping: %v", err)
	}
}

func TestAfterDisconnectEveryWriteIsANoOp(t *testing.T) {
	w := &failingWriter{remaining: 0, err: errors.New("broken pipe")}
	e := NewEmitter(w, WithEmitterIDs("t1", "r1"))

	e.RunStarted()
	first := e.Err()
	e.TextStart("m1")
	e.TextContent("m1", "hello")
	e.RunFinished()

	if e.Err() != first {
		t.Fatal("the first transport error must be sticky")
	}
}

func TestFirstWriteFailureCancelsTheRun(t *testing.T) {
	w := &failingWriter{remaining: 0, err: errors.New("broken pipe")}
	cancelled := 0
	e := NewEmitter(w,
		WithEmitterIDs("t1", "r1"),
		WithEmitterCancel(func() { cancelled++ }))

	e.RunStarted()
	e.TextStart("m1")
	e.TextEnd("m1")

	if cancelled != 1 {
		t.Fatalf("cancel was called %d times, want exactly 1", cancelled)
	}
}

func TestInvalidEventIsDroppedButTheStreamContinues(t *testing.T) {
	var buf bytes.Buffer
	var dropped int
	e := NewEmitter(&buf,
		WithEmitterIDs("t1", "r1"),
		WithEmitterDropped(func(_ context.Context, _ Event, err error) {
			if !errors.Is(err, ErrEventDropped) {
				t.Errorf("drop error = %v, want ErrEventDropped", err)
			}
			dropped++
		}))

	// A TOOL_CALL_START with an empty toolCallId is rejected by the SDK.
	//
	// PT: Um TOOL_CALL_START com toolCallId vazio é recusado pelo SDK.
	e.ToolStart("", "some", "")
	// ...and the stream must stay alive for the terminal event to arrive.
	//
	// PT: ...e o stream tem de continuar vivo para o evento terminal chegar.
	e.RunFinished()

	if dropped != 1 {
		t.Fatalf("dropped = %d, want 1", dropped)
	}
	if e.Err() != nil {
		t.Fatalf("an invalid event must not block the stream: %v", e.Err())
	}
	if !strings.Contains(buf.String(), "RUN_FINISHED") {
		t.Fatalf("the terminal event did not reach the client:\n%s", buf.String())
	}
}

func TestOnlyOneTerminalEventPerRun(t *testing.T) {
	var buf bytes.Buffer
	e := NewEmitter(&buf, WithEmitterIDs("t1", "r1"))

	e.RunFinished()
	if !e.Terminated() {
		t.Fatal("Terminated should be true after RUN_FINISHED")
	}
	e.RunError("should not go out")
	e.RunFinished()

	if n := strings.Count(buf.String(), `"RUN_FINISHED"`); n != 1 {
		t.Fatalf("RUN_FINISHED appeared %d times, want 1", n)
	}
	if strings.Contains(buf.String(), "RUN_ERROR") {
		t.Fatalf("a second terminal went out:\n%s", buf.String())
	}
}

func TestEmptyDeltasDoNotBecomeEvents(t *testing.T) {
	var buf bytes.Buffer
	e := NewEmitter(&buf, WithEmitterIDs("t1", "r1"))

	e.TextContent("m1", "") // the SDK rejects an empty delta (PT: o SDK recusa delta vazio)
	e.ToolArgs("c1", "")
	e.StateDelta(nil)

	if buf.Len() != 0 {
		t.Fatalf("nothing should have been written:\n%s", buf.String())
	}
}
