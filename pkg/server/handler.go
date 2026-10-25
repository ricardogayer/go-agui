// Package server exposes AG-UI agents over the HTTP + SSE binding.
//
// A handler reads the RunAgentInput from the POST body under the processing
// model (unrecognised material stripped with a warning, malformed known
// values rejected with an HTTP error before any stream exists), negotiates the
// protocol version, and hands the agent a stream.SSE that only lets
// conformant events through. If the agent returns an error, or returns
// without closing its run, the handler ends the stream with RUN_ERROR, so a
// consumer always sees a terminal event.
package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/ricardogayer/go-agui/pkg/agui"
	"github.com/ricardogayer/go-agui/pkg/stream"
)

// AgentRunner runs an agent. Implement it to integrate LangchainGo, GenKit or
// any other framework.
//
// Run receives the parsed input and a stream. It opens the run with
// sse.RunStarted(input.ThreadID, input.RunID), emits its events, and closes
// the run with RunFinished (or one of its variants) — or returns an error,
// which the handler reports as RUN_ERROR.
type AgentRunner interface {
	Run(ctx context.Context, input *agui.RunAgentInput, sse *stream.SSE) error
}

// RunnerFunc adapts a function to AgentRunner.
type RunnerFunc func(ctx context.Context, input *agui.RunAgentInput, sse *stream.SSE) error

// Run implements AgentRunner.
func (f RunnerFunc) Run(ctx context.Context, input *agui.RunAgentInput, sse *stream.SSE) error {
	return f(ctx, input, sse)
}

// Logger receives the warnings the protocol says a producer should surface
// (stripped input material, a newer peer version, ...).
type Logger func(format string, args ...any)

// Options configures a handler.
type Options struct {
	// Logger receives warnings. Defaults to log.Printf.
	Logger Logger
	// RejectOtherMajorVersions rejects, before the run starts, an input
	// declaring a major protocol version this implementation does not speak
	// (the specification permits this; the default is to serve it).
	RejectOtherMajorVersions bool
	// MaxBodyBytes bounds the request body (default 32 MiB).
	MaxBodyBytes int64
	// StreamOptions are passed to every stream.
	StreamOptions []stream.Option
	// DisableProtobuf answers SSE whatever the client's Accept header says.
	// By default the handler serves the HTTP + Protobuf binding to clients
	// whose Accept admits application/vnd.ag-ui.event+proto, as the
	// specification recommends.
	DisableProtobuf bool
}

// AgentHandler serves one agent.
type AgentHandler struct {
	runner AgentRunner
	opts   Options
}

// NewAgentHandler returns a handler serving runner.
func NewAgentHandler(runner AgentRunner, opts ...Options) *AgentHandler {
	h := &AgentHandler{runner: runner}
	if len(opts) > 0 {
		h.opts = opts[0]
	}
	if h.opts.Logger == nil {
		h.opts.Logger = log.Printf
	}
	if h.opts.MaxBodyBytes <= 0 {
		h.opts.MaxBodyBytes = 32 << 20
	}
	return h
}

// ErrorResponse is the JSON body of a rejected request.
type ErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

// RequestError is an input rejected before the run started.
type RequestError struct {
	Status  int
	Message string
	Err     error
}

func (e *RequestError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *RequestError) Unwrap() error { return e.Err }

// ReadInput reads and parses a run input from an HTTP request under the
// processing model. Warnings are about material that was stripped or
// translated; a *RequestError means the input must be rejected before the
// run starts.
func (h *AgentHandler) ReadInput(r *http.Request) (*agui.RunAgentInput, []agui.Warning, error) {
	if r.Method != http.MethodPost {
		return nil, nil, &RequestError{Status: http.StatusMethodNotAllowed, Message: "the run input must be POSTed"}
	}
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.Contains(strings.ToLower(ct), "json") {
		return nil, nil, &RequestError{Status: http.StatusUnsupportedMediaType, Message: "the run input must be application/json"}
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, h.opts.MaxBodyBytes+1))
	if err != nil {
		return nil, nil, &RequestError{Status: http.StatusBadRequest, Message: "cannot read request body", Err: err}
	}
	if int64(len(body)) > h.opts.MaxBodyBytes {
		return nil, nil, &RequestError{Status: http.StatusRequestEntityTooLarge, Message: "request body too large"}
	}
	input, warnings, err := agui.ParseRunAgentInput(body)
	if err != nil {
		return nil, warnings, &RequestError{Status: http.StatusBadRequest, Message: "invalid run input", Err: err}
	}
	kind, w := agui.ClassifyPeerVersion(input.ProtocolVersion)
	if w != nil {
		warnings = append(warnings, *w)
	}
	if kind == agui.PeerOtherMajor && h.opts.RejectOtherMajorVersions {
		return nil, warnings, &RequestError{Status: http.StatusBadRequest, Message: fmt.Sprintf("unsupported protocol version %q (this agent speaks %s)", *input.ProtocolVersion, agui.ProtocolVersion)}
	}
	return input, warnings, nil
}

// ServeHTTP implements http.Handler.
func (h *AgentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	input, warnings, err := h.ReadInput(r)
	for _, warn := range warnings {
		h.opts.Logger("ag-ui: run input: %s", warn)
	}
	if err != nil {
		writeRequestError(w, err)
		return
	}
	opts := append([]stream.Option{stream.ForThread(input.ThreadID)}, h.opts.StreamOptions...)
	var sse *stream.SSE
	if h.opts.DisableProtobuf {
		sse = stream.New(w, opts...)
	} else {
		sse = stream.Negotiate(w, r, opts...)
	}
	w.WriteHeader(http.StatusOK)
	h.run(r.Context(), input, sse)
}

func writeRequestError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	resp := ErrorResponse{Error: err.Error()}
	var re *RequestError
	if errors.As(err, &re) {
		status = re.Status
		resp.Error = re.Message
		if re.Err != nil {
			resp.Details = re.Err.Error()
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = jsonEncode(w, resp)
}

// run executes the agent and makes sure the stream ends with a terminal event.
func (h *AgentHandler) run(ctx context.Context, input *agui.RunAgentInput, sse *stream.SSE) {
	err := safeRun(ctx, h.runner, input, sse)
	if sse.Failed() != nil || ctx.Err() != nil {
		// The connection is gone: nothing more can reach the consumer.
		if err != nil {
			h.opts.Logger("ag-ui: run %s ended after the connection closed: %v", input.RunID, err)
		}
		return
	}
	switch {
	case err != nil:
		if sse.RunActive() || !sse.Started() {
			if sendErr := sse.RunError(err.Error(), nil); sendErr != nil {
				h.opts.Logger("ag-ui: cannot report run error: %v (original error: %v)", sendErr, err)
			}
		} else {
			h.opts.Logger("ag-ui: run %s failed after it closed: %v", input.RunID, err)
		}
	case sse.RunActive():
		h.opts.Logger("ag-ui: agent returned without closing run %s; reporting RUN_ERROR", input.RunID)
		_ = sse.RunError("the agent returned without finishing the run", agui.Ptr("AGENT_INCOMPLETE"))
	case !sse.Started():
		h.opts.Logger("ag-ui: agent returned without starting run %s; reporting RUN_ERROR", input.RunID)
		_ = sse.RunError("the agent returned without starting the run", agui.Ptr("AGENT_INCOMPLETE"))
	}
}

func safeRun(ctx context.Context, runner AgentRunner, input *agui.RunAgentInput, sse *stream.SSE) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("agent panicked: %v", p)
		}
	}()
	return runner.Run(ctx, input, sse)
}

// =============================================================================
// Convenience constructors
// =============================================================================

// FunctionAgentHandler returns an http.Handler running fn. Gin users wrap it
// with gin.WrapH (or use the ginx module).
func FunctionAgentHandler(fn func(ctx context.Context, input *agui.RunAgentInput, sse *stream.SSE) error, opts ...Options) http.Handler {
	return NewAgentHandler(RunnerFunc(fn), opts...)
}

// EchoRunner echoes the last user message. Useful for testing and as an
// example implementation.
type EchoRunner struct{}

// Run implements AgentRunner.
func (r *EchoRunner) Run(ctx context.Context, input *agui.RunAgentInput, sse *stream.SSE) error {
	if err := sse.RunStarted(input.ThreadID, input.RunID); err != nil {
		return err
	}
	text := LastUserText(input)
	if text == "" {
		text = "Hello! I'm an echo agent."
	}
	messageID := uuid.NewString()
	if err := sse.TextStart(messageID); err != nil {
		return err
	}
	if err := sse.TextContent(messageID, "Echo: "+text); err != nil {
		return err
	}
	if err := sse.TextEnd(messageID); err != nil {
		return err
	}
	return sse.RunFinished(input.ThreadID, input.RunID)
}

// LastUserText returns the text of the last user message in the input (text
// parts concatenated when the content is multimodal), or "".
func LastUserText(input *agui.RunAgentInput) string {
	for i := len(input.Messages) - 1; i >= 0; i-- {
		if m, ok := input.Messages[i].(*agui.UserMessage); ok {
			return m.Content.String()
		}
	}
	return ""
}
