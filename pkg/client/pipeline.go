package client

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// Middleware sees every event as generic JSON after the compatibility
// boundary and before enforcement, so it can translate or act on material the
// current protocol does not describe. It returns the events to pass on: the
// event itself, a rewritten one, several, or none to drop it.
type Middleware func(event map[string]any) ([]map[string]any, error)

// Pipeline is the consumer's processing pipeline for one stream:
//
//	producer → compatibility boundary → middleware → enforcement
//	         → chunk expansion → verification → application
//
// Feed it the data payload of each transport frame with Push; it returns the
// events application code may see, already applied to the Store.
type Pipeline struct {
	// Store receives every delivered event. Required.
	Store *Store
	// Middleware runs in order, before enforcement.
	Middleware []Middleware
	// Warn receives warnings (stripped material, translated retired shapes,
	// lossy downgrades, patches that did not apply, ...).
	Warn func(agui.Warning)
	// PeerVersion is the producer's protocol version when the consumer knows
	// it to be older ("0.0.57"); it selects the era downgrades. Empty means
	// current.
	PeerVersion string

	thinking agui.ThinkingTranslator
	checker  agui.StreamChecker
}

func (p *Pipeline) warn(w agui.Warning) {
	if p.Warn != nil {
		p.Warn(w)
	}
}

// Push processes one event payload. A returned error means the stream must be
// rejected: a malformed known value (*agui.ValidationError), a sequencing
// violation (*agui.ProtocolError) or invalid JSON.
func (p *Pipeline) Push(data []byte) ([]agui.Event, error) {
	if p.Store.warn == nil {
		p.Store.warn = p.Warn
	}
	var raw any
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("ag-ui: event is not JSON: %w", err)
	}
	ev, ok := raw.(map[string]any)
	if !ok {
		return nil, &agui.ValidationError{Keyword: "type", Message: "an event must be a JSON object"}
	}

	// Compatibility boundary.
	for _, w := range p.thinking.Translate(ev) {
		p.warn(w)
	}
	if t, _ := ev["type"].(string); t == string(agui.EventTypeMessagesSnapshot) {
		for _, w := range agui.UpgradeLegacyContent(ev["messages"], "/messages", true) {
			p.warn(w)
		}
	}
	if input, ok := ev["input"].(map[string]any); ok {
		for _, w := range agui.UpgradeLegacyContent(input["messages"], "/input/messages", true) {
			p.warn(w)
		}
	}
	events := []map[string]any{ev}
	if eraAtMost(p.PeerVersion, 0, 0, 57) {
		events = p.downgradeSubagents(events)
	}

	// Middleware.
	for _, mw := range p.Middleware {
		var next []map[string]any
		for _, e := range events {
			out, err := mw(e)
			if err != nil {
				return nil, err
			}
			next = append(next, out...)
		}
		events = next
	}

	var delivered []agui.Event
	for _, e := range events {
		// Enforcement.
		res, err := agui.SanitizeValue("Event", e, agui.SanitizeOptions{TolerateLegacyNulls: true})
		if res != nil {
			for _, w := range res.Warnings {
				p.warn(w)
			}
		}
		if err != nil {
			return delivered, err
		}
		if res.Dropped {
			continue
		}
		b, err := json.Marshal(res.Value)
		if err != nil {
			return delivered, err
		}
		typed, err := agui.UnmarshalEvent(b)
		if err != nil {
			return delivered, err
		}
		if rs, ok := typed.(*agui.RunStartedEvent); ok {
			p.checkProducerVersion(rs)
		}
		// Chunk expansion and verification.
		expanded, err := p.checker.Check(typed)
		if err != nil {
			return delivered, err
		}
		for _, x := range expanded {
			p.Store.Apply(x)
		}
		delivered = append(delivered, expanded...)
	}
	return delivered, nil
}

func (p *Pipeline) checkProducerVersion(rs *agui.RunStartedEvent) {
	if _, w := agui.ClassifyPeerVersion(rs.ProtocolVersion); w != nil {
		w.Path = "/protocolVersion"
		w.Message = "producer: " + w.Message
		p.warn(*w)
	}
}

// downgradeSubagents is the 0.0.57 era shim: a peer from before subagents
// existed gets no SUBAGENT_* events (a lossy removal, announced) and no
// attribution (lossless, silent).
func (p *Pipeline) downgradeSubagents(events []map[string]any) []map[string]any {
	var out []map[string]any
	for _, e := range events {
		t, _ := e["type"].(string)
		if strings.HasPrefix(t, "SUBAGENT_") {
			p.warn(agui.Warning{Message: fmt.Sprintf("dropping %s: the peer (protocol %s) predates subagents", t, p.PeerVersion)})
			continue
		}
		delete(e, "subagentRunId")
		out = append(out, e)
	}
	return out
}

// RunOpen reports whether the stream has an open run.
func (p *Pipeline) RunOpen() bool { return p.checker.Verifier.RunActive() }

// RunClosed reports whether the last run closed with a terminal event.
func (p *Pipeline) RunClosed() bool { return p.checker.Verifier.RunClosed() }

// eraAtMost reports whether a known peer version is at or below a.b.c.
// Versions are compared component by component; "" (unknown) means current.
func eraAtMost(version string, a, b, c int) bool {
	if version == "" {
		return false
	}
	parts := strings.Split(version, ".")
	want := []int{a, b, c}
	for i := 0; i < 3; i++ {
		n := 0
		if i < len(parts) {
			var err error
			if n, err = strconv.Atoi(parts[i]); err != nil {
				return false
			}
		}
		if n != want[i] {
			return n < want[i]
		}
	}
	return true
}

// peerPredatesVersioning reports whether a known peer version is older than
// 1.0, the first version carrying protocolVersion.
func peerPredatesVersioning(version string) bool {
	return version != "" && eraAtMost(version, 0, 999999, 999999)
}
