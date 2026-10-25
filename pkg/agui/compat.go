package agui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// This file holds the compatibility shims for shapes the protocol retired in
// 1.0 (DEPRECATIONS.md in the AG-UI repository; every shim expires
// 2027-09-17). They run on generic JSON before Sanitize, because once the
// processing model judges a document against 1.0 a retired shape is simply
// unrecognised material and would be stripped.

// UpgradeLegacyContent converts retired {type:"binary"} content parts inside
// a list of messages (generic JSON) into the 1.0 media parts — image, audio,
// video or document by MIME type — with a data or url source. A legacy part
// that mirrors a modern part already present is removed. A part carrying only
// an id cannot be converted and is left for the processing model to strip.
//
// announce reports each conversion as a warning. The conversion loses
// nothing, so a consumer upgrading its own outgoing input may stay silent;
// a producer receiving a retired shape announces it so the sender can be
// upgraded. A part that cannot be converted is always reported.
func UpgradeLegacyContent(messages any, loc string, announce bool) []Warning {
	list, ok := messages.([]any)
	if !ok {
		return nil
	}
	var warnings []Warning
	for i, m := range list {
		msg, ok := m.(map[string]any)
		if !ok {
			continue
		}
		parts, ok := msg["content"].([]any)
		if !ok {
			continue
		}
		out := make([]any, 0, len(parts))
		changed := false
		for j, p := range parts {
			part, ok := p.(map[string]any)
			mime, hasMime := part["mimeType"].(string)
			if !ok || part["type"] != "binary" || !hasMime {
				out = append(out, p)
				continue
			}
			at := fmt.Sprintf("%s/%d/content/%d", loc, i, j)
			converted := convertBinary(part, mime)
			if converted == nil {
				warnings = append(warnings, Warning{Path: at, Message: fmt.Sprintf("a binary content part carrying only an id (%q) cannot be converted to a 1.0 media part; provide data or a url", part["id"])})
				out = append(out, p)
				continue
			}
			changed = true
			filename, _ := part["filename"].(string)
			if mirrorsModernPart(parts, converted, filename) {
				continue
			}
			if announce {
				warnings = append(warnings, Warning{Path: at, Message: fmt.Sprintf("retired {type:\"binary\"} content part upgraded to a %q part with a %s source", converted["type"], converted["source"].(map[string]any)["type"])})
			}
			out = append(out, converted)
		}
		if changed {
			msg["content"] = out
		}
	}
	return warnings
}

func convertBinary(part map[string]any, mime string) map[string]any {
	kind := "document"
	switch {
	case strings.HasPrefix(mime, "image/"):
		kind = "image"
	case strings.HasPrefix(mime, "audio/"):
		kind = "audio"
	case strings.HasPrefix(mime, "video/"):
		kind = "video"
	}
	var source map[string]any
	if data, _ := part["data"].(string); data != "" {
		source = map[string]any{"type": "data", "value": data, "mimeType": mime}
	} else if url, _ := part["url"].(string); url != "" {
		source = map[string]any{"type": "url", "value": url, "mimeType": mime}
	} else {
		return nil
	}
	out := map[string]any{"type": kind, "source": source}
	if filename, _ := part["filename"].(string); filename != "" {
		out["metadata"] = map[string]any{"filename": filename}
	}
	return out
}

func mirrorsModernPart(parts []any, converted map[string]any, filename string) bool {
	cs := converted["source"].(map[string]any)
	for _, p := range parts {
		part, ok := p.(map[string]any)
		if !ok || part["type"] != converted["type"] {
			continue
		}
		src, ok := part["source"].(map[string]any)
		if !ok || src["type"] != cs["type"] || src["value"] != cs["value"] || src["mimeType"] != cs["mimeType"] {
			continue
		}
		if filename == "" {
			return true
		}
		if md, ok := part["metadata"].(map[string]any); ok && md["filename"] == filename {
			return true
		}
	}
	return false
}

// ParseRunAgentInput decodes a request body under the processing model, as a
// producer must: retired shapes are upgraded, optional nulls the deprecation
// registry lists read as absent, unrecognised material is stripped — each with
// a warning — and a malformed known field is an error (a *ValidationError),
// which the transport turns into a rejection before RUN_STARTED.
func ParseRunAgentInput(data []byte) (*RunAgentInput, []Warning, error) {
	doc, err := decodeGeneric(data)
	if err != nil {
		return nil, nil, err
	}
	var warnings []Warning
	if obj, ok := doc.(map[string]any); ok {
		warnings = append(warnings, UpgradeLegacyContent(obj["messages"], "/messages", true)...)
	}
	res, err := SanitizeValue("RunAgentInput", doc, SanitizeOptions{TolerateLegacyNulls: true})
	if res != nil {
		warnings = append(warnings, res.Warnings...)
	}
	if err != nil {
		return nil, warnings, err
	}
	if res.Dropped {
		return nil, warnings, &ValidationError{Keyword: "type", Message: "not a run input"}
	}
	clean, err := json.Marshal(res.Value)
	if err != nil {
		return nil, warnings, err
	}
	var in RunAgentInput
	if err := json.Unmarshal(clean, &in); err != nil {
		return nil, warnings, err
	}
	return &in, warnings, nil
}

// ThinkingTranslator translates the retired THINKING_* events (0.x) into the
// REASONING_* family on generic JSON events, as the compatibility boundary
// does. The zero value is ready to use; use one per stream.
type ThinkingTranslator struct {
	reasoningID *string
	messageID   *string
}

// Translate rewrites ev in place when it is a THINKING_* event and reports
// what it did.
func (t *ThinkingTranslator) Translate(ev map[string]any) []Warning {
	typ, _ := ev["type"].(string)
	retired := func(replacement EventType) Warning {
		return Warning{Message: fmt.Sprintf("received retired event %s; translated to %s (upgrade the producer to AG-UI %s)", typ, replacement, ProtocolVersion)}
	}
	minted := func(established *string) (string, []Warning) {
		if established != nil {
			return *established, nil
		}
		id := uuid.NewString()
		return id, []Warning{{Message: fmt.Sprintf("minting messageId %q for %s: no THINKING opener preceded it", id, typ)}}
	}
	var ws []Warning
	switch typ {
	case "THINKING_START":
		id := uuid.NewString()
		t.reasoningID = &id
		ws = append(ws, retired(EventTypeReasoningStart))
		if title, ok := ev["title"]; ok {
			ws = append(ws, Warning{Path: "/title", Message: fmt.Sprintf("dropping THINKING_START.title %s: REASONING_START has no title field", quote(title))})
			delete(ev, "title")
		}
		ev["type"], ev["messageId"] = string(EventTypeReasoningStart), id
	case "THINKING_TEXT_MESSAGE_START":
		id := uuid.NewString()
		t.messageID = &id
		ws = append(ws, retired(EventTypeReasoningMessageStart))
		ev["type"], ev["messageId"], ev["role"] = string(EventTypeReasoningMessageStart), id, string(RoleReasoning)
	case "THINKING_TEXT_MESSAGE_CONTENT":
		ws = append(ws, retired(EventTypeReasoningMessageContent))
		id, w := minted(t.messageID)
		ws = append(ws, w...)
		ev["type"], ev["messageId"] = string(EventTypeReasoningMessageContent), id
	case "THINKING_TEXT_MESSAGE_END":
		ws = append(ws, retired(EventTypeReasoningMessageEnd))
		id, w := minted(t.messageID)
		ws = append(ws, w...)
		t.messageID = nil
		ev["type"], ev["messageId"] = string(EventTypeReasoningMessageEnd), id
	case "THINKING_END":
		ws = append(ws, retired(EventTypeReasoningEnd))
		id, w := minted(t.reasoningID)
		ws = append(ws, w...)
		t.reasoningID = nil
		ev["type"], ev["messageId"] = string(EventTypeReasoningEnd), id
	case string(EventTypeRunStarted):
		t.reasoningID, t.messageID = nil, nil
	}
	return ws
}
