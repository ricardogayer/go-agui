package agui

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

// writerQueFalha devolve erro depois de n escritas bem-sucedidas, imitando um
// cliente que desconecta no meio do stream.
type writerQueFalha struct {
	restantes int
	err       error
}

func (w *writerQueFalha) Write(p []byte) (int, error) {
	if w.restantes <= 0 {
		return 0, w.err
	}
	w.restantes--
	return len(p), nil
}

// TestIsTransportErrorReconheceFalhaDeEscritaReal é o teste-canário do
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
func TestIsTransportErrorReconheceFalhaDeEscritaReal(t *testing.T) {
	falha := errors.New("cano quebrado")
	w := &writerQueFalha{restantes: 0, err: falha}

	e := NewEmitter(w, WithEmitterIDs("t1", "r1"))
	e.RunStarted()

	err := e.Err()
	if err == nil {
		t.Fatal("uma escrita que falha tem de ser classificada como erro de transporte;\n" +
			"se este teste quebrou depois de atualizar o SDK AG-UI, confira as\n" +
			"mensagens de pkg/encoding/sse/writer.go contra isTransportError")
	}
	if !errors.Is(err, ErrClientGone) {
		t.Fatalf("erro = %v, esperava embrulhar ErrClientGone", err)
	}
	if !errors.Is(err, falha) {
		t.Errorf("o erro original se perdeu no embrulho: %v", err)
	}
}

func TestDepoisDeDesconectarTodaEscritaViraNoOp(t *testing.T) {
	w := &writerQueFalha{restantes: 0, err: errors.New("cano quebrado")}
	e := NewEmitter(w, WithEmitterIDs("t1", "r1"))

	e.RunStarted()
	primeiro := e.Err()
	e.TextStart("m1")
	e.TextContent("m1", "olá")
	e.RunFinished()

	if e.Err() != primeiro {
		t.Fatal("o primeiro erro de transporte tem de ser pegajoso")
	}
}

func TestPrimeiraFalhaDeEscritaCancelaORun(t *testing.T) {
	w := &writerQueFalha{restantes: 0, err: errors.New("cano quebrado")}
	cancelou := 0
	e := NewEmitter(w,
		WithEmitterIDs("t1", "r1"),
		WithEmitterCancel(func() { cancelou++ }))

	e.RunStarted()
	e.TextStart("m1")
	e.TextEnd("m1")

	if cancelou != 1 {
		t.Fatalf("cancel foi chamado %d vezes, esperava exatamente 1", cancelou)
	}
}

func TestEventoInvalidoCaiMasOStreamSegue(t *testing.T) {
	var buf bytes.Buffer
	var descartados int
	e := NewEmitter(&buf,
		WithEmitterIDs("t1", "r1"),
		WithEmitterDropped(func(_ context.Context, _ Event, err error) {
			if !errors.Is(err, ErrEventDropped) {
				t.Errorf("erro do descarte = %v, esperava ErrEventDropped", err)
			}
			descartados++
		}))

	// Um TOOL_CALL_START com toolCallId vazio é recusado pelo SDK.
	e.ToolStart("", "alguma", "")
	// ...e o stream tem de continuar vivo para o evento terminal chegar.
	e.RunFinished()

	if descartados != 1 {
		t.Fatalf("descartados = %d, esperava 1", descartados)
	}
	if e.Err() != nil {
		t.Fatalf("um evento inválido não pode bloquear o stream: %v", e.Err())
	}
	if !strings.Contains(buf.String(), "RUN_FINISHED") {
		t.Fatalf("o evento terminal não chegou ao cliente:\n%s", buf.String())
	}
}

func TestApenasUmEventoTerminalPorRun(t *testing.T) {
	var buf bytes.Buffer
	e := NewEmitter(&buf, WithEmitterIDs("t1", "r1"))

	e.RunFinished()
	if !e.Terminated() {
		t.Fatal("Terminated deveria ser true depois de RUN_FINISHED")
	}
	e.RunError("não deveria sair")
	e.RunFinished()

	if n := strings.Count(buf.String(), `"RUN_FINISHED"`); n != 1 {
		t.Fatalf("RUN_FINISHED apareceu %d vezes, esperava 1", n)
	}
	if strings.Contains(buf.String(), "RUN_ERROR") {
		t.Fatalf("saiu um segundo terminal:\n%s", buf.String())
	}
}

func TestDeltasVaziosNaoViramEventos(t *testing.T) {
	var buf bytes.Buffer
	e := NewEmitter(&buf, WithEmitterIDs("t1", "r1"))

	e.TextContent("m1", "") // o SDK recusa delta vazio
	e.ToolArgs("c1", "")
	e.StateDelta(nil)

	if buf.Len() != 0 {
		t.Fatalf("nada deveria ter sido escrito:\n%s", buf.String())
	}
}
