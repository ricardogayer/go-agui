package server

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

func jsonEncode(w io.Writer, v any) error { return json.NewEncoder(w).Encode(v) }

// CapabilitiesHandler serves an agent's capability declaration as JSON. The
// specification defines the declaration's shape but deliberately not how a
// consumer retrieves it; a GET endpoint next to the agent endpoint is this
// module's choice, not a protocol requirement.
func CapabilitiesHandler(caps agui.AgentCapabilities) http.Handler {
	body, err := json.Marshal(caps)
	if err == nil {
		err = agui.Validate("AgentCapabilities", body)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err != nil {
			http.Error(w, "invalid capabilities: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
}
