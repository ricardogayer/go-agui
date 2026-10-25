package agui

import "fmt"

// ProtocolError reports a stream that breaks the protocol's sequencing rules:
// an event before RUN_STARTED, content for a message that is not open, a
// chunk with no referent, an attribution mismatch, and so on. A consumer that
// meets one rejects the stream; a producer that would emit one has a bug.
type ProtocolError struct {
	Message string
}

func (e *ProtocolError) Error() string { return "ag-ui: " + e.Message }

func protocolErr(format string, args ...any) *ProtocolError {
	return &ProtocolError{Message: fmt.Sprintf(format, args...)}
}

// laneKey identifies whose chunk lane an item belongs to: the parent agent or
// one subagent invocation.
type laneKey struct {
	subagent string
	isSub    bool
}

func laneOf(sub *string) laneKey {
	if sub == nil {
		return laneKey{}
	}
	return laneKey{subagent: *sub, isSub: true}
}

func (k laneKey) ptr() *string {
	if !k.isSub {
		return nil
	}
	return Ptr(k.subagent)
}

func (k laneKey) describe() string {
	if !k.isSub {
		return "(the parent agent)"
	}
	return k.subagent
}

type chunkKind int

const (
	chunkText chunkKind = iota
	chunkTool
	chunkReasoning
)

type pendingStream struct {
	kind chunkKind
	id   string
	// text
	role TextMessageRole
	name *string
	// tool
	toolName string
	parentID *string
	// attribution of the opener
	subagent *string
}

// ChunkExpander turns the compact chunked spelling (TEXT_MESSAGE_CHUNK,
// TOOL_CALL_CHUNK, REASONING_MESSAGE_CHUNK) into the start/content/end triad,
// synthesizing the END when a chunk stream can no longer continue, as the
// streaming pattern specifies. Every other event passes through, preceded by
// whatever END events it implies. The zero value is ready to use.
type ChunkExpander struct {
	lanes map[laneKey]*pendingStream
	order []laneKey
}

// Expand returns the events e stands for. A *ProtocolError means the chunk
// breaks the chunked form's rules (a first chunk missing its id, a
// conflicting repeat, an ambiguous continuation).
func (x *ChunkExpander) Expand(e Event) ([]Event, error) {
	if x.lanes == nil {
		x.lanes = map[laneKey]*pendingStream{}
	}
	switch t := e.(type) {
	case *TextMessageStartEvent, *TextMessageContentEvent, *TextMessageEndEvent,
		*ToolCallStartEvent, *ToolCallArgsEvent, *ToolCallEndEvent, *ToolCallResultEvent,
		*StateSnapshotEvent, *StateDeltaEvent, *CustomEvent, *StepStartedEvent, *StepFinishedEvent,
		*ReasoningStartEvent, *ReasoningMessageStartEvent, *ReasoningMessageContentEvent,
		*ReasoningMessageEndEvent, *ReasoningEndEvent:
		out := x.closeLane(laneOf(subagentPtr(e)))
		// An explicit event naming an item a chunk stream is assembling is
		// not its continuation: it ends the shorthand, whoever's lane holds
		// it, and is then judged by the triad rules.
		if kind, id, ok := explicitItem(e); ok {
			for _, k := range append([]laneKey(nil), x.order...) {
				if p := x.lanes[k]; p.kind == kind && p.id == id {
					out = append(out, x.closeLane(k)...)
				}
			}
		}
		return append(out, e), nil
	case *RunErrorEvent:
		// RUN_ERROR ends whatever was still open along with the run: nothing
		// is synthesized, and nothing can stand in its way.
		x.lanes, x.order = map[laneKey]*pendingStream{}, nil
		return []Event{e}, nil
	case *RunStartedEvent, *RunFinishedEvent, *MessagesSnapshotEvent:
		return append(x.CloseAll(), e), nil
	case *SubagentFinishedEvent:
		return append(x.closeLane(laneOf(&t.SubagentRunID)), e), nil
	case *SubagentErrorEvent:
		return append(x.closeLane(laneOf(&t.SubagentRunID)), e), nil
	case *TextMessageChunkEvent:
		return x.text(t)
	case *ToolCallChunkEvent:
		return x.tool(t)
	case *ReasoningMessageChunkEvent:
		return x.reasoning(t)
	default:
		// RAW, ACTIVITY_*, REASONING_ENCRYPTED_VALUE and SUBAGENT_STARTED stand
		// aside from assembly.
		return []Event{e}, nil
	}
}

// CloseAll synthesizes the END of every open chunk stream.
func (x *ChunkExpander) CloseAll() []Event {
	var out []Event
	for _, k := range append([]laneKey(nil), x.order...) {
		out = append(out, x.closeLane(k)...)
	}
	return out
}

// Open reports whether any chunk stream is open.
func (x *ChunkExpander) Open() bool { return len(x.lanes) > 0 }

func subagentPtr(e Event) *string {
	if a, ok := e.(interface{ attribution() *Attribution }); ok {
		return a.attribution().SubagentRunID
	}
	return nil
}

func (a *Attribution) attribution() *Attribution { return a }

func (x *ChunkExpander) set(k laneKey, p *pendingStream) {
	if _, ok := x.lanes[k]; !ok {
		x.order = append(x.order, k)
	}
	x.lanes[k] = p
}

func (x *ChunkExpander) closeLane(k laneKey) []Event {
	p, ok := x.lanes[k]
	if !ok {
		return nil
	}
	delete(x.lanes, k)
	for i, o := range x.order {
		if o == k {
			x.order = append(x.order[:i], x.order[i+1:]...)
			break
		}
	}
	attr := Attribution{SubagentRunID: p.subagent}
	switch p.kind {
	case chunkText:
		return []Event{&TextMessageEndEvent{Attribution: attr, MessageID: p.id}}
	case chunkTool:
		return []Event{&ToolCallEndEvent{Attribution: attr, ToolCallID: p.id}}
	default:
		return []Event{&ReasoningMessageEndEvent{Attribution: attr, MessageID: p.id}}
	}
}

func kindNames(k chunkKind) (chunkType, entity, idField string) {
	switch k {
	case chunkText:
		return "TEXT_MESSAGE_CHUNK", "text message", "messageId"
	case chunkTool:
		return "TOOL_CALL_CHUNK", "tool call", "toolCallId"
	default:
		return "REASONING_MESSAGE_CHUNK", "reasoning message", "messageId"
	}
}

// resolveLane decides which lane a chunk continues, per the attribution and
// resolution rules of the streaming pattern.
func (x *ChunkExpander) resolveLane(kind chunkKind, id *string, tag *string) (laneKey, error) {
	chunkType, entity, idField := kindNames(kind)
	if id != nil {
		for _, k := range x.order {
			p := x.lanes[k]
			if p.kind == kind && p.id == *id {
				if tag != nil && laneOf(tag) != k {
					return k, protocolErr("Cannot continue %s '%s': chunk subagentRunId '%s' does not match the open stream's subagent '%s'.", entity, *id, *tag, k.describe())
				}
				return k, nil
			}
		}
		return laneOf(tag), nil
	}
	if tag != nil {
		return laneOf(tag), nil
	}
	if p, ok := x.lanes[laneKey{}]; ok && p.kind == kind {
		return laneKey{}, nil
	}
	var candidates []laneKey
	for _, k := range x.order {
		if x.lanes[k].kind == kind {
			candidates = append(candidates, k)
		}
	}
	switch len(candidates) {
	case 0:
		return laneKey{}, nil
	case 1:
		return candidates[0], nil
	}
	return laneKey{}, protocolErr("Ambiguous %s: it carries neither a %s nor a subagentRunId, but %d lanes have an open %s. Attribute the chunk to the subagent it belongs to.", chunkType, idField, len(candidates), entity)
}

func agree(entity, id, field string, incoming, established *string) error {
	if incoming == nil {
		return nil
	}
	if established == nil || *incoming != *established {
		est := "(absent)"
		if established != nil {
			est = "'" + *established + "'"
		}
		return protocolErr("Cannot continue %s '%s': chunk %s '%s' does not match the open stream's %s %s.", entity, id, field, *incoming, field, est)
	}
	return nil
}

func chunkBase(c *BaseEvent, withRaw bool) BaseEvent {
	b := BaseEvent{Metadata: c.Metadata}
	if withRaw {
		b.RawEvent = c.RawEvent
	}
	return b
}

func ownerOf(chunkTag, opener *string) *string {
	if chunkTag != nil {
		return chunkTag
	}
	return opener
}

func (x *ChunkExpander) text(c *TextMessageChunkEvent) ([]Event, error) {
	lane, err := x.resolveLane(chunkText, c.MessageID, c.SubagentRunID)
	if err != nil {
		return nil, err
	}
	var out []Event
	open := x.lanes[lane]
	var p *pendingStream
	if open != nil && open.kind == chunkText && (c.MessageID == nil || *c.MessageID == open.id) {
		role := string(open.role)
		var incomingRole *string
		if c.Role != nil {
			incomingRole = Ptr(string(*c.Role))
		}
		if err := agree("text message", open.id, "role", incomingRole, &role); err != nil {
			return nil, err
		}
		if err := agree("text message", open.id, "name", c.Name, open.name); err != nil {
			return nil, err
		}
		p = open
	} else {
		out = append(out, x.closeLane(lane)...)
		if c.MessageID == nil {
			return nil, protocolErr("First TEXT_MESSAGE_CHUNK must have a messageId")
		}
		role := TextMessageRoleAssistant
		if c.Role != nil {
			role = *c.Role
		}
		p = &pendingStream{kind: chunkText, id: *c.MessageID, role: role, name: c.Name, subagent: c.SubagentRunID}
		x.set(lane, p)
		out = append(out, &TextMessageStartEvent{
			BaseEvent:   chunkBase(&c.BaseEvent, false),
			Attribution: Attribution{SubagentRunID: c.SubagentRunID},
			MessageID:   p.id, Role: Ptr(role), Name: c.Name,
		})
	}
	if c.Delta != nil || c.RawEvent != nil || (len(out) == 0 && c.Metadata != nil) {
		out = append(out, &TextMessageContentEvent{
			BaseEvent:   chunkBase(&c.BaseEvent, true),
			Attribution: Attribution{SubagentRunID: ownerOf(c.SubagentRunID, p.subagent)},
			MessageID:   p.id, Delta: Deref(c.Delta),
		})
	}
	return out, nil
}

func (x *ChunkExpander) tool(c *ToolCallChunkEvent) ([]Event, error) {
	lane, err := x.resolveLane(chunkTool, c.ToolCallID, c.SubagentRunID)
	if err != nil {
		return nil, err
	}
	var out []Event
	open := x.lanes[lane]
	var p *pendingStream
	if open != nil && open.kind == chunkTool && (c.ToolCallID == nil || *c.ToolCallID == open.id) {
		if err := agree("tool call", open.id, "toolCallName", c.ToolCallName, &open.toolName); err != nil {
			return nil, err
		}
		if err := agree("tool call", open.id, "parentMessageId", c.ParentMessageID, open.parentID); err != nil {
			return nil, err
		}
		p = open
	} else {
		out = append(out, x.closeLane(lane)...)
		if c.ToolCallID == nil {
			return nil, protocolErr("First TOOL_CALL_CHUNK must have a toolCallId")
		}
		if c.ToolCallName == nil {
			return nil, protocolErr("First TOOL_CALL_CHUNK must have a toolCallName")
		}
		p = &pendingStream{kind: chunkTool, id: *c.ToolCallID, toolName: *c.ToolCallName, parentID: c.ParentMessageID, subagent: c.SubagentRunID}
		x.set(lane, p)
		out = append(out, &ToolCallStartEvent{
			BaseEvent:   chunkBase(&c.BaseEvent, false),
			Attribution: Attribution{SubagentRunID: c.SubagentRunID},
			ToolCallID:  p.id, ToolCallName: p.toolName, ParentMessageID: p.parentID,
		})
	}
	if c.Delta != nil || c.RawEvent != nil || (len(out) == 0 && c.Metadata != nil) {
		out = append(out, &ToolCallArgsEvent{
			BaseEvent:   chunkBase(&c.BaseEvent, true),
			Attribution: Attribution{SubagentRunID: ownerOf(c.SubagentRunID, p.subagent)},
			ToolCallID:  p.id, Delta: Deref(c.Delta),
		})
	}
	return out, nil
}

func (x *ChunkExpander) reasoning(c *ReasoningMessageChunkEvent) ([]Event, error) {
	lane, err := x.resolveLane(chunkReasoning, c.MessageID, c.SubagentRunID)
	if err != nil {
		return nil, err
	}
	var out []Event
	open := x.lanes[lane]
	var p *pendingStream
	if open != nil && open.kind == chunkReasoning && (c.MessageID == nil || *c.MessageID == open.id) {
		p = open
	} else {
		out = append(out, x.closeLane(lane)...)
		if c.MessageID == nil {
			return nil, protocolErr("First REASONING_MESSAGE_CHUNK must have a messageId")
		}
		p = &pendingStream{kind: chunkReasoning, id: *c.MessageID, subagent: c.SubagentRunID}
		x.set(lane, p)
		out = append(out, &ReasoningMessageStartEvent{
			BaseEvent:   chunkBase(&c.BaseEvent, false),
			Attribution: Attribution{SubagentRunID: c.SubagentRunID},
			MessageID:   p.id,
		})
	}
	if c.Delta != nil || c.RawEvent != nil || (len(out) == 0 && c.Metadata != nil) {
		out = append(out, &ReasoningMessageContentEvent{
			BaseEvent:   chunkBase(&c.BaseEvent, true),
			Attribution: Attribution{SubagentRunID: ownerOf(c.SubagentRunID, p.subagent)},
			MessageID:   p.id, Delta: Deref(c.Delta),
		})
	}
	return out, nil
}

// Clone returns an independent copy of the expander's state.
func (x ChunkExpander) Clone() ChunkExpander {
	c := ChunkExpander{order: append([]laneKey(nil), x.order...)}
	if x.lanes != nil {
		c.lanes = make(map[laneKey]*pendingStream, len(x.lanes))
		for k, p := range x.lanes {
			cp := *p
			c.lanes[k] = &cp
		}
	}
	return c
}

// explicitItem names the streamed item an explicit event addresses.
func explicitItem(e Event) (chunkKind, string, bool) {
	switch t := e.(type) {
	case *TextMessageStartEvent:
		return chunkText, t.MessageID, true
	case *TextMessageContentEvent:
		return chunkText, t.MessageID, true
	case *TextMessageEndEvent:
		return chunkText, t.MessageID, true
	case *ToolCallStartEvent:
		return chunkTool, t.ToolCallID, true
	case *ToolCallArgsEvent:
		return chunkTool, t.ToolCallID, true
	case *ToolCallEndEvent:
		return chunkTool, t.ToolCallID, true
	case *ReasoningMessageStartEvent:
		return chunkReasoning, t.MessageID, true
	case *ReasoningMessageContentEvent:
		return chunkReasoning, t.MessageID, true
	case *ReasoningMessageEndEvent:
		return chunkReasoning, t.MessageID, true
	}
	return 0, "", false
}
