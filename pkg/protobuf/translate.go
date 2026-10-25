package protobuf

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// The JSON event and the wire messages differ where the wire flattens a JSON
// union: the seven message roles share one Message, a tool result's content
// is a string field or a parts list, RUN_FINISHED and SUBAGENT_FINISHED carry
// their outcome as a string beside its payload, patch operations are an enum.
// These translations follow the reference implementations, so the bytes are
// the bytes the shared corpus pins.

// =============================================================================
// JSON → wire
// =============================================================================

func asObject(v any) *object {
	o, _ := v.(*object)
	return o
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

// copyObject returns a shallow copy of o.
func copyObject(o *object) *object {
	c := newObject()
	for _, k := range o.keys {
		c.set(k, o.vals[k])
	}
	return c
}

// keepObject sets dst[k] = src[k] when that value is an object (the wire's
// Struct-typed metadata cannot carry anything else).
func keepObject(dst *object, k string) {
	if v, ok := dst.get(k); ok {
		if _, isObj := v.(*object); !isObj {
			dst.del(k)
		}
	}
}

func toWireSource(v any) (*object, bool) {
	src := asObject(v)
	if src == nil {
		return nil, false
	}
	arm := newObject()
	inner := newObject()
	copyKey := func(k string) {
		if x, ok := src.get(k); ok {
			inner.set(k, x)
		}
	}
	t, _ := src.vals["type"].(string)
	switch t {
	case "data", "url":
		copyKey("value")
		copyKey("mimeType")
	case "file":
		copyKey("value")
		copyKey("provider")
		copyKey("mimeType")
	default:
		return nil, false
	}
	arm.set(t, inner)
	return arm, true
}

func toWirePart(v any) (*object, bool) {
	part := asObject(v)
	if part == nil {
		return nil, false
	}
	t, _ := part.vals["type"].(string)
	inner := newObject()
	copyKey := func(k string) {
		if x, ok := part.get(k); ok {
			inner.set(k, x)
		}
	}
	switch t {
	case "text":
		copyKey("id")
		copyKey("text")
		copyKey("metadata")
	case "image", "audio", "video", "document":
		copyKey("id")
		if src, ok := part.get("source"); ok {
			wire, ok := toWireSource(src)
			if !ok {
				return nil, false
			}
			inner.set("source", wire)
		}
		copyKey("metadata")
	case "binary":
		// The retired 0.x part rides as a document with marker metadata, as
		// the reference encoders do.
		mime := part.vals["mimeType"]
		source := newObject()
		pick := newObject()
		switch {
		case nonEmpty(part.vals["data"]):
			source.set("value", part.vals["data"])
			source.set("mimeType", mime)
			pick.set("data", source)
		case nonEmpty(part.vals["url"]):
			source.set("value", part.vals["url"])
			source.set("mimeType", mime)
			pick.set("url", source)
		case nonEmpty(part.vals["id"]):
			source.set("value", part.vals["id"])
			source.set("mimeType", mime)
			pick.set("url", source)
		default:
			return nil, false
		}
		md := newObject()
		md.set("legacyBinary", true)
		if f, ok := part.get("filename"); ok {
			md.set("filename", f)
		}
		if id, ok := part.get("id"); ok {
			md.set("id", id)
		}
		inner.set("source", pick)
		inner.set("metadata", md)
		t = "document"
	default:
		return nil, false
	}
	arm := newObject()
	arm.set(t, inner)
	return arm, true
}

func nonEmpty(v any) bool {
	s, ok := v.(string)
	return ok && s != ""
}

// toWireParts converts a JSON content list; parts the wire cannot carry are
// dropped and reported.
func toWireParts(list []any, at string, warn func(string)) []any {
	out := []any{}
	for i, p := range list {
		wire, ok := toWirePart(p)
		if !ok {
			warn(fmt.Sprintf("%s/%d: content part cannot be encoded on the binary wire; dropped", at, i))
			continue
		}
		out = append(out, wire)
	}
	return out
}

func toWireMessage(v any, at string, warn func(string)) *object {
	msg := asObject(v)
	if msg == nil {
		msg = newObject()
	}
	wire := copyObject(msg)
	keepObject(wire, "metadata")
	if calls, ok := wire.get("toolCalls"); ok {
		var out []any
		for _, c := range asList(calls) {
			call := copyObject(asObject(c))
			keepObject(call, "metadata")
			out = append(out, call)
		}
		wire.set("toolCalls", nilToEmpty(out))
	}
	role, _ := msg.vals["role"].(string)
	if content, ok := msg.get("content"); ok {
		if list, isList := content.([]any); isList {
			wire.set("contentParts", toWireParts(list, at+"/content", warn))
			wire.del("content")
		} else if role == "activity" {
			if obj, isObj := content.(*object); isObj {
				wire.set("activityContent", obj)
			} else {
				wire.set("activityContent", newObject())
			}
			wire.del("content")
		}
	} else if role == "activity" {
		wire.set("activityContent", newObject())
	}
	return wire
}

func nilToEmpty(l []any) []any {
	if l == nil {
		return []any{}
	}
	return l
}

func toWireRunAgentInput(v any, warn func(string)) *object {
	in := asObject(v)
	if in == nil {
		return nil
	}
	wire := newObject()
	for _, k := range []string{"threadId", "runId", "protocolVersion", "parentRunId", "state"} {
		if x, ok := in.get(k); ok {
			wire.set(k, x)
		}
	}
	var msgs []any
	for i, m := range asList(in.vals["messages"]) {
		msgs = append(msgs, toWireMessage(m, fmt.Sprintf("/input/messages/%d", i), warn))
	}
	wire.set("messages", nilToEmpty(msgs))
	withMetadata := func(list []any) []any {
		out := []any{}
		for _, x := range list {
			o := copyObject(asObject(x))
			keepObject(o, "metadata")
			out = append(out, o)
		}
		return out
	}
	wire.set("tools", withMetadata(asList(in.vals["tools"])))
	wire.set("context", nilToEmpty(asList(in.vals["context"])))
	if x, ok := in.get("forwardedProps"); ok {
		wire.set("forwardedProps", x)
	}
	wire.set("resume", withMetadata(asList(in.vals["resume"])))
	return wire
}

func toWirePatch(list []any) ([]any, error) {
	out := []any{}
	for i, x := range list {
		op := copyObject(asObject(x))
		name, _ := op.vals["op"].(string)
		idx := -1
		for j, known := range patchOps {
			if known == name {
				idx = j
			}
		}
		if idx < 0 {
			return nil, fmt.Errorf("ag-ui/protobuf: patch operation %d: %q has no binary encoding", i, name)
		}
		op.set("op", json.Number(strconv.Itoa(idx)))
		out = append(out, op)
	}
	return out, nil
}

// toWire turns a JSON event into the wire Event envelope.
func toWire(event *object, warn func(string)) (*object, error) {
	typ, _ := event.vals["type"].(string)
	var armName, msgName string
	for _, a := range eventArms {
		if a.eventType == typ {
			armName, msgName = a.name, a.msg
		}
	}
	if armName == "" {
		return nil, fmt.Errorf("ag-ui/protobuf: event type %q has no binary encoding", typ)
	}
	_ = msgName
	base := newObject()
	base.set("type", json.Number(strconv.Itoa(int(eventTypeNumbers[typ]))))
	for _, k := range []string{"timestamp", "rawEvent", "metadata"} {
		if v, ok := event.get(k); ok {
			base.set(k, v)
		}
	}
	keepObject(base, "metadata")
	rest := newObject()
	rest.set("baseEvent", base)
	for _, k := range event.keys {
		switch k {
		case "type", "timestamp", "rawEvent", "metadata":
			continue
		}
		rest.set(k, event.vals[k])
	}

	switch typ {
	case "MESSAGES_SNAPSHOT":
		var msgs []any
		for i, m := range asList(rest.vals["messages"]) {
			msgs = append(msgs, toWireMessage(m, fmt.Sprintf("/messages/%d", i), warn))
		}
		if _, ok := rest.get("messages"); ok {
			rest.set("messages", nilToEmpty(msgs))
		}
	case "RUN_FINISHED":
		outcome, present := rest.get("outcome")
		o := asObject(outcome)
		rest.set("pendingToolCallIds", []any{})
		rest.set("interrupts", []any{})
		ot, _ := o.vals2("type")
		switch {
		case !present:
			rest.set("outcome", "")
		case ot == "success":
			rest.set("outcome", "success")
			rest.set("pendingToolCallIds", nilToEmpty(asList(o.vals["pendingToolCallIds"])))
		case ot == "interrupt":
			rest.set("outcome", "interrupt")
			rest.set("interrupts", nilToEmpty(asList(o.vals["interrupts"])))
		default:
			rest.set("outcome", ot)
		}
		rest.set("usage", nilToEmpty(asList(rest.vals["usage"])))
	case "SUBAGENT_FINISHED":
		outcome, present := rest.get("outcome")
		o := asObject(outcome)
		rest.set("interruptIds", []any{})
		ot, _ := o.vals2("type")
		switch {
		case !present:
			rest.set("outcome", "")
		case ot == "suspended":
			rest.set("outcome", "suspended")
			rest.set("interruptIds", nilToEmpty(asList(o.vals["interruptIds"])))
		default:
			rest.set("outcome", ot)
		}
	case "RUN_ERROR":
		rest.set("usage", nilToEmpty(asList(rest.vals["usage"])))
	case "STATE_DELTA", "ACTIVITY_DELTA":
		key := "delta"
		if typ == "ACTIVITY_DELTA" {
			key = "patch"
		}
		if list, ok := rest.get(key); ok {
			wire, err := toWirePatch(asList(list))
			if err != nil {
				return nil, err
			}
			rest.set(key, wire)
		}
	case "TOOL_CALL_RESULT":
		if list, isList := rest.vals["content"].([]any); isList {
			rest.set("contentParts", toWireParts(list, "/content", warn))
			rest.del("content")
		}
	case "RUN_STARTED":
		if in, ok := rest.get("input"); ok {
			if wire := toWireRunAgentInput(in, warn); wire != nil {
				rest.set("input", wire)
			}
		}
	}
	envelope := newObject()
	envelope.set(armName, rest)
	return envelope, nil
}

// vals2 reads a string member of a possibly nil object.
func (o *object) vals2(k string) (string, bool) {
	if o == nil {
		return "", false
	}
	s, ok := o.vals[k].(string)
	return s, ok
}

// =============================================================================
// wire → JSON
// =============================================================================

func fromWireSource(v any) (*object, bool, error) {
	src := asObject(v)
	if src == nil || len(src.keys) == 0 {
		return nil, false, nil
	}
	if len(src.keys) > 1 {
		return nil, false, malformed("a part source carries more than one arm")
	}
	arm := src.keys[0]
	inner := asObject(src.vals[arm])
	out := newObject()
	out.set("type", arm)
	for _, k := range inner.keys {
		out.set(k, inner.vals[k])
	}
	return out, true, nil
}

func fromWirePart(v any) (*object, bool, error) {
	part := asObject(v)
	if part == nil || len(part.keys) == 0 {
		return nil, false, nil
	}
	if len(part.keys) > 1 {
		return nil, false, malformed("a content part carries more than one arm")
	}
	kind := part.keys[0]
	inner := asObject(part.vals[kind])
	out := newObject()
	out.set("type", kind)
	if id, ok := inner.get("id"); ok {
		out.set("id", id)
	}
	if kind == "text" {
		out.set("text", inner.vals["text"])
	} else if src, ok := inner.get("source"); ok {
		s, ok, err := fromWireSource(src)
		if err != nil {
			return nil, false, err
		}
		if !ok {
			return nil, false, nil // a source arm this build does not know
		}
		out.set("source", s)
	}
	if md, ok := inner.get("metadata"); ok {
		out.set("metadata", md)
	}
	return out, true, nil
}

func fromWireParts(list []any, at string, warn func(string)) ([]any, error) {
	out := []any{}
	for i, p := range list {
		part, ok, err := fromWirePart(p)
		if err != nil {
			return nil, err
		}
		if !ok {
			warn(fmt.Sprintf("%s/%d: dropped a content part of a kind this build does not know", at, i))
			continue
		}
		out = append(out, part)
	}
	return out, nil
}

var knownRoles = map[string]bool{"developer": true, "system": true, "assistant": true, "user": true, "tool": true, "activity": true, "reasoning": true}

func fromWireMessage(v any, at string, warn func(string)) (*object, error) {
	wire := asObject(v)
	if wire == nil {
		wire = newObject()
	}
	role, _ := wire.vals["role"].(string)
	known := knownRoles[role]
	parts := asList(wire.vals["contentParts"])
	_, hasContent := wire.get("content")
	_, hasActivity := wire.get("activityContent")
	usesParts := role == "user" || role == "tool"
	usesMap := role == "activity"
	if !known {
		usesParts = len(parts) > 0
		usesMap = !usesParts && hasActivity
		forms := 0
		for _, b := range []bool{hasContent, len(parts) > 0, hasActivity} {
			if b {
				forms++
			}
		}
		if forms > 1 {
			return nil, malformed("message carries more than one content form")
		}
	} else {
		switch {
		case !usesParts && len(parts) > 0:
			return nil, malformed("message carries content parts for a role that has none")
		case hasActivity && !usesMap:
			return nil, malformed("message carries activity content for a non-activity role")
		case usesMap && hasContent:
			return nil, malformed("activity content cannot ride with other content forms")
		case usesParts && hasContent && len(parts) > 0:
			return nil, malformed("message carries both string content and content parts")
		}
	}
	msg := newObject()
	for _, k := range wire.keys {
		switch k {
		case "contentParts", "activityContent":
			continue
		case "toolCalls":
			if len(asList(wire.vals[k])) == 0 {
				continue
			}
		}
		msg.set(k, wire.vals[k])
	}
	if usesParts && !hasContent {
		list, err := fromWireParts(parts, at+"/content", warn)
		if err != nil {
			return nil, err
		}
		msg.set("content", list)
	}
	if usesMap && hasActivity {
		msg.set("content", wire.vals["activityContent"])
	}
	return msg, nil
}

func fromWirePatch(list []any) []any {
	out := []any{}
	for _, x := range list {
		op := asObject(x)
		n, _ := op.vals["op"].(json.Number)
		i, err := strconv.Atoi(string(n))
		if err == nil && i >= 0 && i < len(patchOps) {
			op.set("op", patchOps[i])
		} else {
			// An operation this build cannot name rides on as its decimal
			// spelling, for enforcement to strip and name — never invented,
			// never dropped here.
			op.set("op", string(n))
		}
		out = append(out, op)
	}
	return out
}

func fromWireRunAgentInput(in *object, warn func(string)) (*object, error) {
	out := newObject()
	for _, k := range []string{"threadId", "runId", "protocolVersion", "parentRunId", "state"} {
		if v, ok := in.get(k); ok {
			out.set(k, v)
		}
	}
	msgs := []any{}
	for i, m := range asList(in.vals["messages"]) {
		msg, err := fromWireMessage(m, fmt.Sprintf("/input/messages/%d", i), warn)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, msg)
	}
	out.set("messages", msgs)
	out.set("tools", nilToEmpty(asList(in.vals["tools"])))
	out.set("context", nilToEmpty(asList(in.vals["context"])))
	if v, ok := in.get("forwardedProps"); ok {
		out.set("forwardedProps", v)
	}
	out.set("resume", nilToEmpty(asList(in.vals["resume"])))
	return out, nil
}

// fromWire turns a decoded envelope into the JSON event it carries.
func fromWire(envelope *object, warn func(string)) (*object, error) {
	if len(envelope.keys) != 1 {
		return nil, malformed("envelope carries %d events", len(envelope.keys))
	}
	armName := envelope.keys[0]
	var typ string
	for _, a := range eventArms {
		if a.name == armName {
			typ = a.eventType
		}
	}
	payload := asObject(envelope.vals[armName])
	base := asObject(payload.vals["baseEvent"])
	if base == nil {
		return nil, malformed("%s carries no base event", typ)
	}
	declared, _ := base.vals["type"].(json.Number)
	if declared != json.Number(strconv.Itoa(int(eventTypeNumbers[typ]))) {
		return nil, malformed("envelope carries %s but the base event declares %s", typ, declared)
	}

	ev := newObject()
	ev.set("type", typ)
	for _, k := range payload.keys {
		if k != "baseEvent" {
			ev.set(k, payload.vals[k])
		}
	}
	for _, k := range []string{"timestamp", "rawEvent", "metadata"} {
		if v, ok := base.get(k); ok {
			ev.set(k, v)
		}
	}

	switch typ {
	case "MESSAGES_SNAPSHOT":
		msgs := []any{}
		for i, m := range asList(ev.vals["messages"]) {
			msg, err := fromWireMessage(m, fmt.Sprintf("/messages/%d", i), warn)
			if err != nil {
				return nil, err
			}
			msgs = append(msgs, msg)
		}
		ev.set("messages", msgs)
	case "RUN_FINISHED":
		wireOutcome, _ := ev.vals["outcome"].(string)
		pending := asList(ev.vals["pendingToolCallIds"])
		interrupts := asList(ev.vals["interrupts"])
		ev.del("outcome")
		ev.del("pendingToolCallIds")
		ev.del("interrupts")
		if wireOutcome == "" {
			// No outcome: a payload that rode anyway surfaces as it arrived,
			// for enforcement to judge.
			if len(pending) > 0 {
				ev.set("pendingToolCallIds", pending)
			}
			if len(interrupts) > 0 {
				ev.set("interrupts", interrupts)
			}
		} else {
			o := newObject()
			o.set("type", wireOutcome)
			if wireOutcome == "interrupt" {
				o.set("interrupts", interrupts)
			} else if len(interrupts) > 0 {
				o.set("interrupts", interrupts)
			}
			if len(pending) > 0 {
				o.set("pendingToolCallIds", pending)
			}
			ev.set("outcome", o)
		}
		if len(asList(ev.vals["usage"])) == 0 {
			ev.del("usage")
		}
	case "SUBAGENT_FINISHED":
		wireOutcome, _ := ev.vals["outcome"].(string)
		ids := asList(ev.vals["interruptIds"])
		ev.del("outcome")
		ev.del("interruptIds")
		if wireOutcome == "" {
			if len(ids) > 0 {
				ev.set("interruptIds", ids)
			}
		} else {
			o := newObject()
			o.set("type", wireOutcome)
			if len(ids) > 0 {
				o.set("interruptIds", ids)
			}
			ev.set("outcome", o)
		}
	case "RUN_ERROR":
		if len(asList(ev.vals["usage"])) == 0 {
			ev.del("usage")
		}
	case "STATE_DELTA":
		ev.set("delta", fromWirePatch(asList(ev.vals["delta"])))
	case "ACTIVITY_DELTA":
		ev.set("patch", fromWirePatch(asList(ev.vals["patch"])))
	case "TOOL_CALL_RESULT":
		parts := asList(ev.vals["contentParts"])
		ev.del("contentParts")
		if _, hasContent := ev.get("content"); hasContent {
			if len(parts) > 0 {
				return nil, malformed("TOOL_CALL_RESULT carries both string content and content parts")
			}
		} else {
			list, err := fromWireParts(parts, "/content", warn)
			if err != nil {
				return nil, err
			}
			ev.set("content", list)
		}
	case "RUN_STARTED":
		if in := asObject(ev.vals["input"]); in != nil {
			out, err := fromWireRunAgentInput(in, warn)
			if err != nil {
				return nil, err
			}
			ev.set("input", out)
		}
	}
	return ev, nil
}

// typeOf names an event type in an error.
func typeOf(event *object) string {
	t, _ := event.vals2("type")
	return strings.TrimSpace(t)
}
