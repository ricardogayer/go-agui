package agui

import (
	"sort"
	"strings"
)

// Verifier enforces the ordering rules of an AG-UI event stream: the run
// lifecycle (a stream begins with RUN_STARTED or RUN_ERROR, nothing follows a
// closed run but RUN_STARTED or a late RUN_ERROR, no nested runs), the
// open/content/close discipline of messages, tool calls, reasoning spans and
// reasoning messages, balanced steps, subagent lifecycle, and subagent
// ownership of everything opened.
//
// It checks the expanded form: chunk events must go through a ChunkExpander
// first (StreamChecker combines the two). A producer runs every event it
// emits through one, a consumer every event it receives. The zero value is
// ready to use.
type Verifier struct {
	started      bool
	first        bool
	runActive    bool
	runFinished  bool
	runErrored   bool
	messages     map[string]bool
	toolCalls    map[string]bool
	spans        map[string]bool
	reasoning    map[string]bool
	steps        map[laneKey]map[string]bool
	subagents    map[string]bool
	closedSubs   map[string]bool
	ownMessage   map[string]laneKey
	ownToolCall  map[string]laneKey
	ownActivity  map[string]laneKey
	ownReasoning map[string]laneKey

	// Producer enables the rules that bind what a producer emits but that a
	// consumer is not required to detect: RUN_STARTED declares the protocol
	// version, boundary events agree on the run's
	// identity and a runId is not reused, a subagent is announced before any
	// event attributed to it and not used after it finished, a reopened item
	// agrees with its opener, interrupt ids are unique, pendingToolCallIds is
	// exact, and an activity delta has a snapshot to amend.
	Producer bool
	// ThreadID, when set in producer mode, is the input's thread: every
	// run on the stream carries it on its boundary events.
	ThreadID string

	threadID, runID string
	usedRunIDs      map[string]bool       // stream-wide
	attributed      map[string]bool       // subagent ids seen on events, this run
	textOpeners     map[string]textOpener // first opener of each text message
	toolOpeners     map[string]toolOpener // first opener of each tool call
	callSeq         map[string]int        // tool calls started this run, in order
	answered        map[string]bool       // tool calls answered this run

	// journal records writes to the maps that only grow while a trial copy
	// (see CheckAtomic) is in use: those maps are shared with the original,
	// so a failed trial is undone by rollback instead of copying them per
	// event.
	journal *[]func()
}

type textOpener struct {
	role TextMessageRole
	name *string
}

type toolOpener struct {
	name   string
	parent *string
}

// jset writes m[k] = val, journaling the previous value when a trial is in
// progress.
func jset[K comparable, V any](v *Verifier, m map[K]V, k K, val V) {
	if v.journal != nil {
		prev, existed := m[k]
		*v.journal = append(*v.journal, func() {
			if existed {
				m[k] = prev
			} else {
				delete(m, k)
			}
		})
	}
	m[k] = val
}

func (v *Verifier) setOwner(m map[string]laneKey, k string, lane laneKey) { jset(v, m, k, lane) }

func (v *Verifier) reset() {
	v.messages = map[string]bool{}
	v.toolCalls = map[string]bool{}
	v.spans = map[string]bool{}
	v.reasoning = map[string]bool{}
	v.steps = map[laneKey]map[string]bool{}
	v.subagents = map[string]bool{}
	v.closedSubs = map[string]bool{}
	v.ownMessage = map[string]laneKey{}
	v.ownToolCall = map[string]laneKey{}
	v.ownActivity = map[string]laneKey{}
	v.ownReasoning = map[string]laneKey{}
	v.attributed = map[string]bool{}
	v.textOpeners = map[string]textOpener{}
	v.toolOpeners = map[string]toolOpener{}
	v.callSeq = map[string]int{}
	v.answered = map[string]bool{}
	if v.usedRunIDs == nil {
		v.usedRunIDs = map[string]bool{}
	}
	v.runFinished, v.runErrored = false, false
}

// RunActive reports whether a run is open (started and not yet closed).
func (v *Verifier) RunActive() bool { return v.runActive }

// RunClosed reports whether the last run on the stream has closed, by
// RUN_FINISHED or RUN_ERROR.
func (v *Verifier) RunClosed() bool { return v.runFinished || v.runErrored }

func keys(m map[string]bool) string {
	var ks []string
	for k, open := range m {
		if open {
			ks = append(ks, k)
		}
	}
	sort.Strings(ks)
	return strings.Join(ks, ", ")
}

func ownerMismatch(et EventType, tag *string, owner laneKey, known bool, kind, id string) error {
	if tag == nil || !known {
		return nil
	}
	if laneOf(tag) != owner {
		return protocolErr("Cannot send '%s': subagentRunId '%s' does not match the %s '%s' opener's subagent '%s'.", et, *tag, kind, id, owner.describe())
	}
	return nil
}

func (v *Verifier) seed(msgs Messages, authoritative bool) {
	for _, m := range msgs {
		var sub *string
		var calls []ToolCall
		bucket := v.ownMessage
		switch t := m.(type) {
		case *AssistantMessage:
			sub, calls = t.SubagentRunID, t.ToolCalls
		case *UserMessage:
			sub = t.SubagentRunID
		case *SystemMessage:
			sub = t.SubagentRunID
		case *DeveloperMessage:
			sub = t.SubagentRunID
		case *ToolMessage:
			sub = t.SubagentRunID
		case *ActivityMessage:
			sub, bucket = t.SubagentRunID, v.ownActivity
		case *ReasoningMessage:
			sub, bucket = t.SubagentRunID, v.ownReasoning
		}
		if _, ok := bucket[m.MessageID()]; authoritative || !ok {
			v.setOwner(bucket, m.MessageID(), laneOf(sub))
		}
		for _, tc := range calls {
			if _, ok := v.ownToolCall[tc.ID]; authoritative || !ok {
				v.setOwner(v.ownToolCall, tc.ID, laneOf(sub))
			}
		}
	}
}

// Check verifies the next event of the stream. It returns a *ProtocolError
// when the event breaks a rule; the verifier's state is then unspecified and
// the stream must be abandoned.
func (v *Verifier) Check(e Event) error {
	if v.messages == nil {
		v.reset()
	}
	et := e.Type()
	if _, isChunk := e.(interface{ chunk() }); isChunk {
		return protocolErr("'%s' must be expanded before verification", et)
	}
	if v.runErrored && et != EventTypeRunStarted {
		return protocolErr("Cannot send event type '%s': The run has already errored with 'RUN_ERROR'. No further events can be sent.", et)
	}
	if v.runFinished && et != EventTypeRunError && et != EventTypeRunStarted {
		return protocolErr("Cannot send event type '%s': The run has already finished with 'RUN_FINISHED'. Start a new run with 'RUN_STARTED'.", et)
	}
	if !v.first {
		v.first = true
		if et != EventTypeRunStarted && et != EventTypeRunError {
			return protocolErr("First event must be 'RUN_STARTED'")
		}
	} else if et == EventTypeRunStarted {
		if v.runActive {
			return protocolErr("Cannot send 'RUN_STARTED' while a run is still active. The previous run must be finished with 'RUN_FINISHED' before starting a new run.")
		}
	}
	if et == EventTypeRunStarted && (v.runFinished || v.runErrored || !v.started) {
		v.reset()
	}
	if v.Producer {
		if err := v.checkProducer(e); err != nil {
			return err
		}
	}

	switch t := e.(type) {
	case *RunStartedEvent:
		v.started, v.runActive = true, true
		if t.Input != nil {
			v.seed(t.Input.Messages, false)
		}
	case *RunFinishedEvent:
		if len(v.stepsOpen()) > 0 {
			return protocolErr("Cannot send 'RUN_FINISHED' while steps are still active: %s", strings.Join(v.stepsOpen(), ", "))
		}
		if s := keys(v.messages); s != "" {
			return protocolErr("Cannot send 'RUN_FINISHED' while text messages are still active: %s", s)
		}
		if s := keys(v.reasoning); s != "" {
			return protocolErr("Cannot send 'RUN_FINISHED' while reasoning messages are still active: %s", s)
		}
		if s := keys(v.spans); s != "" {
			return protocolErr("Cannot send 'RUN_FINISHED' while reasoning spans are still active: %s", s)
		}
		if s := keys(v.toolCalls); s != "" {
			return protocolErr("Cannot send 'RUN_FINISHED' while tool calls are still active: %s", s)
		}
		if s := keys(v.subagents); s != "" {
			return protocolErr("Cannot send 'RUN_FINISHED' while subagents are still active: %s", s)
		}
		v.runFinished, v.runActive = true, false
	case *RunErrorEvent:
		v.runErrored, v.runActive = true, false

	case *TextMessageStartEvent:
		if v.messages[t.MessageID] {
			return protocolErr("Cannot send 'TEXT_MESSAGE_START' event: A text message with ID '%s' is already in progress. Complete it with 'TEXT_MESSAGE_END' first.", t.MessageID)
		}
		owner, known := v.ownMessage[t.MessageID]
		if err := ownerMismatch(et, t.SubagentRunID, owner, known, "message", t.MessageID); err != nil {
			return err
		}
		v.messages[t.MessageID] = true
		if !known {
			v.setOwner(v.ownMessage, t.MessageID, laneOf(t.SubagentRunID))
		}
	case *TextMessageContentEvent:
		if !v.messages[t.MessageID] {
			return protocolErr("Cannot send 'TEXT_MESSAGE_CONTENT' event: No active text message found with ID '%s'. Start a text message with 'TEXT_MESSAGE_START' first.", t.MessageID)
		}
		owner, known := v.ownMessage[t.MessageID]
		return ownerMismatch(et, t.SubagentRunID, owner, known, "message", t.MessageID)
	case *TextMessageEndEvent:
		if !v.messages[t.MessageID] {
			return protocolErr("Cannot send 'TEXT_MESSAGE_END' event: No active text message found with ID '%s'. A 'TEXT_MESSAGE_START' event must be sent first.", t.MessageID)
		}
		owner, known := v.ownMessage[t.MessageID]
		if err := ownerMismatch(et, t.SubagentRunID, owner, known, "message", t.MessageID); err != nil {
			return err
		}
		delete(v.messages, t.MessageID)

	case *ToolCallStartEvent:
		if v.toolCalls[t.ToolCallID] {
			return protocolErr("Cannot send 'TOOL_CALL_START' event: A tool call with ID '%s' is already in progress. Complete it with 'TOOL_CALL_END' first.", t.ToolCallID)
		}
		var inherited *laneKey
		if t.ParentMessageID != nil {
			if parent, ok := v.ownMessage[*t.ParentMessageID]; ok {
				if t.SubagentRunID != nil && laneOf(t.SubagentRunID) != parent {
					return protocolErr("Cannot send 'TOOL_CALL_START': subagentRunId '%s' does not match its parent message '%s' owner '%s'. A tool call belongs to the message that carries it.", *t.SubagentRunID, *t.ParentMessageID, parent.describe())
				}
				inherited = &parent
			}
		}
		existing, known := v.ownToolCall[t.ToolCallID]
		if known {
			if err := ownerMismatch(et, t.SubagentRunID, existing, true, "tool call", t.ToolCallID); err != nil {
				return err
			}
			if t.SubagentRunID == nil && inherited != nil && *inherited != existing {
				return protocolErr("Cannot send 'TOOL_CALL_START': tool call '%s' is owned by '%s' but its parent message '%s' is owned by '%s'. A tool call belongs to the message that carries it.", t.ToolCallID, existing.describe(), *t.ParentMessageID, inherited.describe())
			}
		}
		v.toolCalls[t.ToolCallID] = true
		if !known {
			switch {
			case t.SubagentRunID != nil:
				v.setOwner(v.ownToolCall, t.ToolCallID, laneOf(t.SubagentRunID))
			case inherited != nil:
				v.setOwner(v.ownToolCall, t.ToolCallID, *inherited)
			default:
				v.setOwner(v.ownToolCall, t.ToolCallID, laneKey{})
			}
		}
	case *ToolCallArgsEvent:
		if !v.toolCalls[t.ToolCallID] {
			return protocolErr("Cannot send 'TOOL_CALL_ARGS' event: No active tool call found with ID '%s'. Start a tool call with 'TOOL_CALL_START' first.", t.ToolCallID)
		}
		owner, known := v.ownToolCall[t.ToolCallID]
		return ownerMismatch(et, t.SubagentRunID, owner, known, "tool call", t.ToolCallID)
	case *ToolCallEndEvent:
		if !v.toolCalls[t.ToolCallID] {
			return protocolErr("Cannot send 'TOOL_CALL_END' event: No active tool call found with ID '%s'. A 'TOOL_CALL_START' event must be sent first.", t.ToolCallID)
		}
		owner, known := v.ownToolCall[t.ToolCallID]
		if err := ownerMismatch(et, t.SubagentRunID, owner, known, "tool call", t.ToolCallID); err != nil {
			return err
		}
		delete(v.toolCalls, t.ToolCallID)
	case *ToolCallResultEvent:
		v.setOwner(v.ownMessage, t.MessageID, laneOf(t.SubagentRunID))

	case *StepStartedEvent:
		lane := laneOf(t.SubagentRunID)
		if v.steps[lane][t.StepName] {
			in := ""
			if lane.isSub {
				in = " in subagent '" + lane.subagent + "'"
			}
			return protocolErr("Step \"%s\" is already active for 'STEP_STARTED'%s", t.StepName, in)
		}
		if v.steps[lane] == nil {
			v.steps[lane] = map[string]bool{}
		}
		v.steps[lane][t.StepName] = true
	case *StepFinishedEvent:
		lane := laneOf(t.SubagentRunID)
		if !v.steps[lane][t.StepName] {
			for other, names := range v.steps {
				if other != lane && names[t.StepName] {
					return protocolErr("Cannot send 'STEP_FINISHED' for step \"%s\" attributed to %s: that step is open under %s. A step must be finished by whoever started it.", t.StepName, stepOwner(lane), stepOwner(other))
				}
			}
			return protocolErr("Cannot send 'STEP_FINISHED' for step \"%s\" that was not started", t.StepName)
		}
		delete(v.steps[lane], t.StepName)

	case *ActivitySnapshotEvent:
		if _, known := v.ownActivity[t.MessageID]; !known || t.Replace == nil || *t.Replace {
			v.setOwner(v.ownActivity, t.MessageID, laneOf(t.SubagentRunID))
		}
	case *ActivityDeltaEvent:
		owner, known := v.ownActivity[t.MessageID]
		return ownerMismatch(et, t.SubagentRunID, owner, known, "activity", t.MessageID)

	case *ReasoningStartEvent:
		return v.reasoningOpen(et, t.MessageID, t.SubagentRunID, true)
	case *ReasoningMessageStartEvent:
		return v.reasoningOpen(et, t.MessageID, t.SubagentRunID, false)
	case *ReasoningEndEvent:
		return v.reasoningContinue(et, t.MessageID, t.SubagentRunID, true, true)
	case *ReasoningMessageContentEvent:
		return v.reasoningContinue(et, t.MessageID, t.SubagentRunID, false, false)
	case *ReasoningMessageEndEvent:
		return v.reasoningContinue(et, t.MessageID, t.SubagentRunID, false, true)
	case *ReasoningEncryptedValueEvent:
		var owner laneKey
		var known bool
		kind := "message"
		if t.Subtype == EncryptedValueSubtypeToolCall {
			owner, known = v.ownToolCall[t.EntityID]
			kind = "tool call"
		} else {
			owner, known = v.ownMessage[t.EntityID]
			if !known {
				owner, known = v.ownReasoning[t.EntityID]
			}
		}
		return ownerMismatch(et, t.SubagentRunID, owner, known, kind, t.EntityID)

	case *SubagentStartedEvent:
		id := t.SubagentRunID
		if v.subagents[id] {
			return protocolErr("Cannot send 'SUBAGENT_STARTED': subagent '%s' is already active. Finish it with 'SUBAGENT_FINISHED' first.", id)
		}
		if v.closedSubs[id] {
			return protocolErr("Cannot send 'SUBAGENT_STARTED': subagent '%s' has already finished in this run. Subagent IDs are per-invocation and cannot be reused.", id)
		}
		if p := t.ParentSubagentRunID; p != nil && !v.subagents[*p] && !v.closedSubs[*p] {
			return protocolErr("Cannot send 'SUBAGENT_STARTED': parentSubagentRunId '%s' has not been started in this run.", *p)
		}
		v.subagents[id] = true
	case *SubagentFinishedEvent:
		return v.subagentEnd(et, t.SubagentRunID)
	case *SubagentErrorEvent:
		return v.subagentEnd(et, t.SubagentRunID)

	case *MessagesSnapshotEvent:
		v.seed(t.Messages, true)
	}
	return nil
}

func stepOwner(k laneKey) string {
	if k.isSub {
		return "subagent '" + k.subagent + "'"
	}
	return "the parent agent"
}

func (v *Verifier) stepsOpen() []string {
	var out []string
	for owner, names := range v.steps {
		for n, open := range names {
			if !open {
				continue
			}
			if owner.isSub {
				n += " (subagent '" + owner.subagent + "')"
			}
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

func (v *Verifier) reasoningOpen(et EventType, id string, tag *string, span bool) error {
	open := v.reasoning
	if span {
		open = v.spans
	}
	if open[id] {
		if span {
			return protocolErr("Cannot send 'REASONING_START' event: A reasoning span with ID '%s' is already in progress. Complete it with 'REASONING_END' first.", id)
		}
		return protocolErr("Cannot send 'REASONING_MESSAGE_START' event: A reasoning message with ID '%s' is already in progress. Complete it with 'REASONING_MESSAGE_END' first.", id)
	}
	owner, known := v.ownReasoning[id]
	if err := ownerMismatch(et, tag, owner, known, "reasoning message", id); err != nil {
		return err
	}
	open[id] = true
	if !known {
		v.setOwner(v.ownReasoning, id, laneOf(tag))
	}
	return nil
}

func (v *Verifier) reasoningContinue(et EventType, id string, tag *string, span, closes bool) error {
	open := v.reasoning
	if span {
		open = v.spans
	}
	if !open[id] {
		if span {
			return protocolErr("Cannot send 'REASONING_END' event: No active reasoning span found with ID '%s'. A 'REASONING_START' event must be sent first.", id)
		}
		return protocolErr("Cannot send '%s' event: No active reasoning message found with ID '%s'. Start a reasoning message with 'REASONING_MESSAGE_START' first.", et, id)
	}
	owner, known := v.ownReasoning[id]
	if err := ownerMismatch(et, tag, owner, known, "reasoning message", id); err != nil {
		return err
	}
	if closes {
		delete(open, id)
	}
	return nil
}

func (v *Verifier) subagentEnd(et EventType, id string) error {
	if !v.subagents[id] {
		return protocolErr("Cannot send '%s': no active subagent found with ID '%s'. A 'SUBAGENT_STARTED' event must be sent first.", et, id)
	}
	delete(v.subagents, id)
	v.closedSubs[id] = true
	return nil
}

func (*TextMessageChunkEvent) chunk()      {}
func (*ToolCallChunkEvent) chunk()         {}
func (*ReasoningMessageChunkEvent) chunk() {}

// StreamChecker verifies a stream that may use the chunked form: it expands
// chunks and verifies the expanded events. The zero value is ready to use.
type StreamChecker struct {
	Expander ChunkExpander
	Verifier Verifier
}

// CheckAtomic is Check, except that when e is rejected the checker's state is
// left exactly as it was, so the stream can go on without the event. A
// producer uses it to refuse an event without corrupting its bookkeeping.
func (c *StreamChecker) CheckAtomic(e Event) ([]Event, error) {
	trial := StreamChecker{Expander: c.Expander.Clone(), Verifier: c.Verifier.trial()}
	expanded, err := trial.Check(e)
	if err != nil {
		trial.Verifier.rollback()
		return nil, err
	}
	trial.Verifier.commit()
	*c = trial
	return expanded, nil
}

// Check expands e and verifies what it stands for, returning the expanded
// events.
func (c *StreamChecker) Check(e Event) ([]Event, error) {
	expanded, err := c.Expander.Expand(e)
	if err != nil {
		return nil, err
	}
	for _, x := range expanded {
		if err := c.Verifier.Check(x); err != nil {
			return nil, err
		}
	}
	return expanded, nil
}

// Started reports whether the verifier has seen any event.
func (v *Verifier) Started() bool { return v.first }

// Clone returns an independent copy of the verifier's state.
func (v Verifier) Clone() Verifier {
	c := v.trial()
	c.journal = nil
	c.ownMessage = cloneAny(v.ownMessage)
	c.ownToolCall = cloneAny(v.ownToolCall)
	c.ownActivity = cloneAny(v.ownActivity)
	c.ownReasoning = cloneAny(v.ownReasoning)
	c.usedRunIDs = cloneAny(v.usedRunIDs)
	c.attributed = cloneAny(v.attributed)
	c.textOpeners = cloneAny(v.textOpeners)
	c.toolOpeners = cloneAny(v.toolOpeners)
	c.callSeq = cloneAny(v.callSeq)
	c.answered = cloneAny(v.answered)
	return c
}

func cloneAny[K comparable, V any](m map[K]V) map[K]V {
	if m == nil {
		return nil
	}
	c := make(map[K]V, len(m))
	for k, x := range m {
		c[k] = x
	}
	return c
}

// trial returns a copy for a trial check: the sets of open items are copied;
// the maps that only grow are shared and every write to them is journaled, so
// rollback undoes a failed trial and commit keeps it.
func (v Verifier) trial() Verifier {
	c := v
	c.messages = cloneSet(v.messages)
	c.toolCalls = cloneSet(v.toolCalls)
	c.spans = cloneSet(v.spans)
	c.reasoning = cloneSet(v.reasoning)
	c.subagents = cloneSet(v.subagents)
	c.closedSubs = cloneSet(v.closedSubs)
	journal := []func(){}
	c.journal = &journal
	if v.steps != nil {
		c.steps = make(map[laneKey]map[string]bool, len(v.steps))
		for k, s := range v.steps {
			c.steps[k] = cloneSet(s)
		}
	}
	return c
}

func cloneSet(m map[string]bool) map[string]bool {
	if m == nil {
		return nil
	}
	c := make(map[string]bool, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}

// rollback undoes the journaled writes of a trial copy.
func (v *Verifier) rollback() {
	if v.journal == nil {
		return
	}
	for i := len(*v.journal) - 1; i >= 0; i-- {
		(*v.journal)[i]()
	}
	v.journal = nil
}

// commit ends a trial copy's journal, keeping its writes.
func (v *Verifier) commit() { v.journal = nil }

// checkProducer applies the producer-only rules (see Verifier.Producer). It
// runs before the shared rules and mutates only through jset.
func (v *Verifier) checkProducer(e Event) error {
	if a, ok := e.(Attributable); ok {
		if tag := a.AttributedTo(); tag != "" || hasTag(e) {
			if v.closedSubs[tag] {
				return protocolErr("Cannot send '%s': subagent '%s' has already finished in this run; its id cannot be reused.", e.Type(), tag)
			}
			if !v.subagents[tag] && !v.attributed[tag] {
				jset(v, v.attributed, tag, true)
			}
		}
	}
	switch t := e.(type) {
	case *RunStartedEvent:
		if t.ProtocolVersion == nil {
			return protocolErr("Cannot send 'RUN_STARTED' without 'protocolVersion': a producer declares the protocol version it speaks (%s).", ProtocolVersion)
		}
		if v.ThreadID != "" && t.ThreadID != v.ThreadID {
			return protocolErr("Cannot send 'RUN_STARTED' for thread '%s': every run on this stream carries the input's threadId '%s'.", t.ThreadID, v.ThreadID)
		}
		if v.usedRunIDs[t.RunID] {
			return protocolErr("Cannot send 'RUN_STARTED': runId '%s' was already used on this stream; a runId must not be reused.", t.RunID)
		}
		jset(v, v.usedRunIDs, t.RunID, true)
		v.threadID, v.runID = t.ThreadID, t.RunID
	case *RunFinishedEvent:
		if t.RunID != v.runID || t.ThreadID != v.threadID {
			return protocolErr("Cannot send 'RUN_FINISHED' for thread '%s' run '%s': the run open on this stream is thread '%s' run '%s'.", t.ThreadID, t.RunID, v.threadID, v.runID)
		}
		switch o := t.Outcome.(type) {
		case *InterruptOutcome:
			seen := map[string]bool{}
			for _, in := range o.Interrupts {
				if seen[in.ID] {
					return protocolErr("Cannot send 'RUN_FINISHED': interrupt id '%s' is not unique within the run.", in.ID)
				}
				seen[in.ID] = true
			}
		case *SuccessOutcome:
			if len(o.PendingToolCallIDs) > 0 {
				want := v.unanswered()
				if strings.Join(o.PendingToolCallIDs, "\x00") != strings.Join(want, "\x00") {
					return protocolErr("Cannot send 'RUN_FINISHED': pendingToolCallIds %v must name exactly the tool calls the run started and did not answer, in call order: %v.", o.PendingToolCallIDs, want)
				}
			}
		}
	case *SubagentStartedEvent:
		if v.attributed[t.SubagentRunID] {
			return protocolErr("Cannot send 'SUBAGENT_STARTED': events attributed to subagent '%s' were already sent; an invocation must be announced before any event attributed to it.", t.SubagentRunID)
		}
	case *TextMessageStartEvent:
		role := TextMessageRoleAssistant
		if t.Role != nil {
			role = *t.Role
		}
		if prev, ok := v.textOpeners[t.MessageID]; ok {
			if prev.role != role || Deref(prev.name) != Deref(t.Name) || (prev.name == nil) != (t.Name == nil) {
				return protocolErr("Cannot send 'TEXT_MESSAGE_START': reopening message '%s' must agree with its opener (role '%s', name %q).", t.MessageID, prev.role, Deref(prev.name))
			}
		} else {
			jset(v, v.textOpeners, t.MessageID, textOpener{role: role, name: t.Name})
		}
	case *ToolCallStartEvent:
		if prev, ok := v.toolOpeners[t.ToolCallID]; ok {
			if prev.name != t.ToolCallName || Deref(prev.parent) != Deref(t.ParentMessageID) || (prev.parent == nil) != (t.ParentMessageID == nil) {
				return protocolErr("Cannot send 'TOOL_CALL_START': reopening tool call '%s' must agree with its opener (toolCallName '%s', parentMessageId %q).", t.ToolCallID, prev.name, Deref(prev.parent))
			}
		} else {
			jset(v, v.toolOpeners, t.ToolCallID, toolOpener{name: t.ToolCallName, parent: t.ParentMessageID})
		}
		if _, ok := v.callSeq[t.ToolCallID]; !ok {
			jset(v, v.callSeq, t.ToolCallID, len(v.callSeq))
		}
	case *ToolCallResultEvent:
		jset(v, v.answered, t.ToolCallID, true)
	case *ActivityDeltaEvent:
		if _, ok := v.ownActivity[t.MessageID]; !ok {
			return protocolErr("Cannot send 'ACTIVITY_DELTA': activity message '%s' was not created with ACTIVITY_SNAPSHOT.", t.MessageID)
		}
	}
	return nil
}

func hasTag(e Event) bool {
	if a, ok := e.(interface{ attribution() *Attribution }); ok {
		return a.attribution().SubagentRunID != nil
	}
	return false
}

// unanswered returns the tool calls this run started and did not answer, in
// call order.
func (v *Verifier) unanswered() []string {
	out := make([]string, len(v.callSeq))
	for id, i := range v.callSeq {
		out[i] = id
	}
	pending := out[:0]
	for _, id := range out {
		if !v.answered[id] {
			pending = append(pending, id)
		}
	}
	return pending
}
