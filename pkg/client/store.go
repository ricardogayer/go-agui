package client

import (
	"encoding/json"
	"fmt"

	"github.com/ricardogayer/go-agui/pkg/agui"
)

// Store is what a consumer keeps of a thread: its messages and its state. It
// applies verified events under the rules of the event families — streamed
// items assemble by id, metadata merges key by key (last write wins, no
// recursion) into each event's merge target, snapshots replace and deltas
// amend, MESSAGES_SNAPSHOT reconciles by id.
type Store struct {
	// Messages is the conversation, in order.
	Messages agui.Messages
	// State is the shared state (any JSON value, in generic form).
	State any

	runToolCalls []string
	answered     map[string]bool
	// invented holds the ids of the assistant messages the store created to
	// carry tool calls that named no parent message. The producer never
	// named those messages, so their ids yield to any message it does name.
	invented map[string]bool
	warn     func(agui.Warning)
}

// NewStore returns a store holding messages and state.
func NewStore(messages agui.Messages, state any) *Store {
	return &Store{Messages: messages, State: state, answered: map[string]bool{}}
}

func (s *Store) warning(format string, args ...any) {
	if s.warn != nil {
		s.warn(agui.Warning{Message: fmt.Sprintf(format, args...)})
	}
}

// claim frees id for a message the producer is naming: an assistant message
// the store invented under that id moves to an id of its own, so the two
// never share one.
func (s *Store) claim(id string) {
	if !s.invented[id] {
		return
	}
	_, m := s.find(id)
	a, ok := m.(*agui.AssistantMessage)
	if !ok {
		delete(s.invented, id)
		return
	}
	delete(s.invented, id)
	a.ID = s.freshID(id + "-tool-calls")
	s.invented[a.ID] = true
}

// freshID returns base, or base with a numeric suffix, whichever no message
// holds yet.
func (s *Store) freshID(base string) string {
	id := base
	for n := 2; ; n++ {
		if _, m := s.find(id); m == nil {
			return id
		}
		id = fmt.Sprintf("%s-%d", base, n)
	}
}

func (s *Store) find(id string) (int, agui.Message) {
	for i, m := range s.Messages {
		if m.MessageID() == id {
			return i, m
		}
	}
	return -1, nil
}

// findToolCall returns the assistant message carrying a tool call, and the call.
func (s *Store) findToolCall(id string) (*agui.AssistantMessage, *agui.ToolCall) {
	for _, m := range s.Messages {
		if a, ok := m.(*agui.AssistantMessage); ok {
			for i := range a.ToolCalls {
				if a.ToolCalls[i].ID == id {
					return a, &a.ToolCalls[i]
				}
			}
		}
	}
	return nil, nil
}

// mergeMetadata merges md into *target key by key, last write winning,
// without recursion.
func mergeMetadata(target *agui.Metadata, md agui.Metadata) {
	if md == nil {
		return
	}
	if *target == nil {
		*target = agui.Metadata{}
	}
	for k, v := range md {
		(*target)[k] = cloneJSON(v)
	}
}

func metadataOf(m agui.Message) *agui.Metadata {
	switch t := m.(type) {
	case *agui.DeveloperMessage:
		return &t.Metadata
	case *agui.SystemMessage:
		return &t.Metadata
	case *agui.AssistantMessage:
		return &t.Metadata
	case *agui.UserMessage:
		return &t.Metadata
	case *agui.ToolMessage:
		return &t.Metadata
	case *agui.ActivityMessage:
		return &t.Metadata
	case *agui.ReasoningMessage:
		return &t.Metadata
	}
	return nil
}

func encryptedValueOf(m agui.Message) **string {
	switch t := m.(type) {
	case *agui.DeveloperMessage:
		return &t.EncryptedValue
	case *agui.SystemMessage:
		return &t.EncryptedValue
	case *agui.AssistantMessage:
		return &t.EncryptedValue
	case *agui.UserMessage:
		return &t.EncryptedValue
	case *agui.ToolMessage:
		return &t.EncryptedValue
	case *agui.ReasoningMessage:
		return &t.EncryptedValue
	}
	return nil
}

// appendText appends a delta to a text-bearing message.
func appendText(m agui.Message, delta string) bool {
	switch t := m.(type) {
	case *agui.AssistantMessage:
		t.Content = agui.Ptr(agui.Deref(t.Content) + delta)
	case *agui.UserMessage:
		t.Content = agui.TextContent(t.Content.String() + delta)
	case *agui.SystemMessage:
		t.Content += delta
	case *agui.DeveloperMessage:
		t.Content += delta
	case *agui.ReasoningMessage:
		t.Content += delta
	default:
		return false
	}
	return true
}

func cloneJSON(v any) any {
	g, err := agui.ToGeneric(v)
	if err != nil {
		return v
	}
	return g
}

// cloneMessage deep-copies a message through its JSON form.
func cloneMessage(m agui.Message) agui.Message {
	b, err := json.Marshal(m)
	if err != nil {
		return m
	}
	c, err := agui.UnmarshalMessage(b)
	if err != nil {
		return m
	}
	return c
}

// Apply applies one verified, expanded event.
func (s *Store) Apply(e agui.Event) {
	if s.answered == nil {
		s.answered = map[string]bool{}
	}
	if s.invented == nil {
		s.invented = map[string]bool{}
	}
	md := e.Base().Metadata
	switch t := e.(type) {
	case *agui.RunStartedEvent:
		s.runToolCalls, s.answered = nil, map[string]bool{}
		if t.Input != nil {
			for _, m := range t.Input.Messages {
				if _, existing := s.find(m.MessageID()); existing == nil {
					s.Messages = append(s.Messages, cloneMessage(m))
				}
			}
		}

	case *agui.TextMessageStartEvent:
		s.claim(t.MessageID)
		_, existing := s.find(t.MessageID)
		if _, isActivity := existing.(*agui.ActivityMessage); isActivity {
			s.warning("TEXT_MESSAGE_START: message '%s' is an activity message; message ids must be unique", t.MessageID)
			return
		}
		if existing == nil {
			role := agui.TextMessageRoleAssistant
			if t.Role != nil {
				role = *t.Role
			}
			var m agui.Message
			switch role {
			case agui.TextMessageRoleUser:
				m = &agui.UserMessage{ID: t.MessageID, Content: agui.TextContent(""), Name: t.Name, SubagentRunID: t.SubagentRunID}
			case agui.TextMessageRoleSystem:
				m = &agui.SystemMessage{ID: t.MessageID, Name: t.Name, SubagentRunID: t.SubagentRunID}
			case agui.TextMessageRoleDeveloper:
				m = &agui.DeveloperMessage{ID: t.MessageID, Name: t.Name, SubagentRunID: t.SubagentRunID}
			default:
				m = &agui.AssistantMessage{ID: t.MessageID, Content: agui.Ptr(""), Name: t.Name, SubagentRunID: t.SubagentRunID}
			}
			s.Messages = append(s.Messages, m)
			existing = m
		}
		mergeMetadata(metadataOf(existing), md)
	case *agui.TextMessageContentEvent:
		s.appendTo("TEXT_MESSAGE_CONTENT", t.MessageID, t.Delta, md)
	case *agui.TextMessageEndEvent:
		s.appendTo("TEXT_MESSAGE_END", t.MessageID, "", md)

	case *agui.ReasoningMessageStartEvent:
		s.claim(t.MessageID)
		_, existing := s.find(t.MessageID)
		if _, isActivity := existing.(*agui.ActivityMessage); isActivity {
			s.warning("REASONING_MESSAGE_START: message '%s' is an activity message; message ids must be unique", t.MessageID)
			return
		}
		if existing == nil {
			existing = &agui.ReasoningMessage{ID: t.MessageID, SubagentRunID: t.SubagentRunID}
			s.Messages = append(s.Messages, existing)
		}
		mergeMetadata(metadataOf(existing), md)
	case *agui.ReasoningMessageContentEvent:
		s.appendTo("REASONING_MESSAGE_CONTENT", t.MessageID, t.Delta, md)
	case *agui.ReasoningMessageEndEvent:
		s.appendTo("REASONING_MESSAGE_END", t.MessageID, "", md)
	case *agui.ReasoningEncryptedValueEvent:
		if t.Subtype == agui.EncryptedValueSubtypeToolCall {
			if _, tc := s.findToolCall(t.EntityID); tc != nil {
				tc.EncryptedValue = agui.Ptr(t.EncryptedValue)
				return
			}
		} else if _, m := s.find(t.EntityID); m != nil {
			if ev := encryptedValueOf(m); ev != nil {
				*ev = agui.Ptr(t.EncryptedValue)
				return
			}
		}
		s.warning("REASONING_ENCRYPTED_VALUE: no %s '%s' to store the value with; dropped", t.Subtype, t.EntityID)

	case *agui.ToolCallStartEvent:
		if !contains(s.runToolCalls, t.ToolCallID) {
			s.runToolCalls = append(s.runToolCalls, t.ToolCallID)
		}
		if _, tc := s.findToolCall(t.ToolCallID); tc != nil {
			if tc.Function.Name != t.ToolCallName {
				s.warning("TOOL_CALL_START: tool call '%s' reopened with name '%s' (was '%s')", t.ToolCallID, t.ToolCallName, tc.Function.Name)
				tc.Function.Name = t.ToolCallName
			}
			mergeMetadata(&tc.Metadata, md)
			return
		}
		target, created := s.assistantFor(t.ParentMessageID, t.ToolCallID)
		if created && t.SubagentRunID != nil && target.SubagentRunID == nil {
			target.SubagentRunID = t.SubagentRunID
		}
		target.ToolCalls = append(target.ToolCalls, agui.NewToolCall(t.ToolCallID, t.ToolCallName, ""))
		mergeMetadata(&target.ToolCalls[len(target.ToolCalls)-1].Metadata, md)
	case *agui.ToolCallArgsEvent:
		if _, tc := s.findToolCall(t.ToolCallID); tc != nil {
			tc.Function.Arguments += t.Delta
			mergeMetadata(&tc.Metadata, md)
		} else {
			s.warning("TOOL_CALL_ARGS: no tool call '%s'", t.ToolCallID)
		}
	case *agui.ToolCallEndEvent:
		if _, tc := s.findToolCall(t.ToolCallID); tc != nil {
			mergeMetadata(&tc.Metadata, md)
		} else {
			s.warning("TOOL_CALL_END: no tool call '%s'", t.ToolCallID)
		}
	case *agui.ToolCallResultEvent:
		s.answered[t.ToolCallID] = true
		s.claim(t.MessageID)
		tm := &agui.ToolMessage{ID: t.MessageID, ToolCallID: t.ToolCallID, Content: t.Content, SubagentRunID: t.SubagentRunID}
		if i, m := s.find(t.MessageID); m != nil {
			if old, ok := m.(*agui.ToolMessage); ok && old.ToolCallID == t.ToolCallID {
				tm.Metadata, tm.EncryptedValue, tm.Error = old.Metadata, old.EncryptedValue, old.Error
				mergeMetadata(&tm.Metadata, md)
				s.Messages[i] = tm
				return
			}
		}
		mergeMetadata(&tm.Metadata, md)
		s.insertToolResult(tm)

	case *agui.StateSnapshotEvent:
		s.State = cloneJSON(t.Snapshot)
	case *agui.StateDeltaEvent:
		base := s.State
		if base == nil {
			base = map[string]any{}
		}
		next, err := agui.ApplyPatch(base, t.Delta)
		if err != nil {
			s.warning("Failed to apply state patch: %v; keeping the prior state", err)
			return
		}
		s.State = next

	case *agui.MessagesSnapshotEvent:
		s.reconcile(t.Messages)

	case *agui.ActivitySnapshotEvent:
		s.claim(t.MessageID)
		i, existing := s.find(t.MessageID)
		replace := t.Replace == nil || *t.Replace
		fresh := &agui.ActivityMessage{ID: t.MessageID, ActivityType: t.ActivityType, Content: cloneObject(t.Content), SubagentRunID: t.SubagentRunID}
		switch old := existing.(type) {
		case nil:
			s.Messages = append(s.Messages, fresh)
			mergeMetadata(&fresh.Metadata, md)
		case *agui.ActivityMessage:
			if replace {
				fresh.Metadata = old.Metadata // accumulated metadata survives
				s.Messages[i] = fresh
				old = fresh
			}
			mergeMetadata(&old.Metadata, md)
		default:
			if replace {
				s.Messages[i] = fresh
				mergeMetadata(&fresh.Metadata, md)
			}
		}
	case *agui.ActivityDeltaEvent:
		i, existing := s.find(t.MessageID)
		act, ok := existing.(*agui.ActivityMessage)
		if !ok {
			if existing == nil {
				s.warning("ACTIVITY_DELTA: no activity message '%s'; delta skipped", t.MessageID)
			} else {
				s.warning("ACTIVITY_DELTA: message '%s' is not an activity message; delta skipped", t.MessageID)
			}
			return
		}
		mergeMetadata(&act.Metadata, md)
		next, err := agui.ApplyPatch(map[string]any(cloneObject(act.Content)), t.Patch)
		if err != nil {
			s.warning("Failed to apply activity patch for '%s': %v", t.MessageID, err)
			return
		}
		obj, ok := next.(map[string]any)
		if !ok {
			s.warning("ACTIVITY_DELTA: patch for '%s' would make content a non-object; skipped", t.MessageID)
			return
		}
		updated := *act
		updated.Content, updated.ActivityType = obj, t.ActivityType
		s.Messages[i] = &updated
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func cloneObject(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	if c, ok := cloneJSON(m).(map[string]any); ok {
		return c
	}
	return m
}

func (s *Store) appendTo(et, id, delta string, md agui.Metadata) {
	_, m := s.find(id)
	if m == nil {
		s.warning("%s: no message found with ID '%s'", et, id)
		return
	}
	if _, isActivity := m.(*agui.ActivityMessage); isActivity {
		s.warning("%s: message '%s' is an activity message; message ids must be unique", et, id)
		return
	}
	if delta != "" && !appendText(m, delta) {
		s.warning("%s: message '%s' cannot hold text", et, id)
		return
	}
	mergeMetadata(metadataOf(m), md)
}

// assistantFor returns the assistant message a tool call attaches to: its
// parent message when that is an assistant message, otherwise a new one —
// under the parent id the producer named, or else under an id the store
// invents (the tool call's, when no message holds it).
func (s *Store) assistantFor(parentID *string, toolCallID string) (*agui.AssistantMessage, bool) {
	id, named := "", false
	if parentID != nil {
		_, existing := s.find(*parentID)
		if a, ok := existing.(*agui.AssistantMessage); ok {
			return a, false
		}
		if existing != nil {
			s.warning("TOOL_CALL_START: parentMessageId '%s' is a %s message, not assistant; attaching the call to a new message", *parentID, existing.MessageRole())
		} else {
			id, named = *parentID, true
		}
	}
	if !named {
		id = s.freshID(toolCallID)
		s.invented[id] = true
	}
	a := &agui.AssistantMessage{ID: id}
	s.Messages = append(s.Messages, a)
	return a, true
}

// insertToolResult places a tool message after the assistant message that
// issued its call (after any results already there), or at the end.
func (s *Store) insertToolResult(tm *agui.ToolMessage) {
	owner := -1
	for i := len(s.Messages) - 1; i >= 0; i-- {
		if a, ok := s.Messages[i].(*agui.AssistantMessage); ok {
			for _, tc := range a.ToolCalls {
				if tc.ID == tm.ToolCallID {
					owner = i
				}
			}
			if owner >= 0 {
				break
			}
		}
	}
	if owner < 0 {
		s.Messages = append(s.Messages, tm)
		return
	}
	at := owner + 1
	for at < len(s.Messages) {
		r := s.Messages[at].MessageRole()
		if r != agui.RoleTool && r != agui.RoleActivity {
			break
		}
		at++
	}
	s.Messages = append(s.Messages[:at], append(agui.Messages{tm}, s.Messages[at:]...)...)
}

// reconcile applies MESSAGES_SNAPSHOT: a message in the snapshot replaces the
// copy with its id in place; one the consumer has never seen is appended in
// snapshot order; one the snapshot omits is dropped — except activity and
// reasoning messages, unless the snapshot itself carries messages of that
// role.
func (s *Store) reconcile(snapshot agui.Messages) {
	byID := map[string]agui.Message{}
	hasActivity, hasReasoning := false, false
	for _, m := range snapshot {
		byID[m.MessageID()] = m
		switch m.MessageRole() {
		case agui.RoleActivity:
			hasActivity = true
		case agui.RoleReasoning:
			hasReasoning = true
		}
	}
	seen := map[string]bool{}
	var out agui.Messages
	for _, m := range s.Messages {
		if repl, ok := byID[m.MessageID()]; ok {
			out = append(out, cloneMessage(repl))
			seen[m.MessageID()] = true
			continue
		}
		switch m.MessageRole() {
		case agui.RoleActivity:
			if !hasActivity {
				out = append(out, m)
			}
		case agui.RoleReasoning:
			if !hasReasoning {
				out = append(out, m)
			}
		}
	}
	for _, m := range snapshot {
		if !seen[m.MessageID()] {
			out = append(out, cloneMessage(m))
			seen[m.MessageID()] = true
		}
	}
	s.Messages = out
	// The snapshot is the producer's account of the conversation: whatever
	// it holds, it named.
	s.invented = map[string]bool{}
}

// PendingToolCallIDs returns the tool calls the last run started and did not
// answer, in call order — what a consumer derives when RUN_FINISHED does not
// name them.
func (s *Store) PendingToolCallIDs() []string {
	var out []string
	for _, id := range s.runToolCalls {
		if !s.answered[id] {
			out = append(out, id)
		}
	}
	return out
}
