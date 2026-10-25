package agui

// AgentCapabilities is what an agent declares about itself before a run. Every
// group and every field is optional: an omitted field means undeclared, not
// unsupported. Declarations are informative; the event stream is
// authoritative. How a consumer obtains a declaration is left to the
// implementation (see server.CapabilitiesHandler for one way).
type AgentCapabilities struct {
	Identity       *IdentityCapabilities       `json:"identity,omitempty"`
	Transport      *TransportCapabilities      `json:"transport,omitempty"`
	Tools          *ToolsCapabilities          `json:"tools,omitempty"`
	Output         *OutputCapabilities         `json:"output,omitempty"`
	State          *StateCapabilities          `json:"state,omitempty"`
	MultiAgent     *MultiAgentCapabilities     `json:"multiAgent,omitempty"`
	Reasoning      *ReasoningCapabilities      `json:"reasoning,omitempty"`
	Multimodal     *MultimodalCapabilities     `json:"multimodal,omitempty"`
	Execution      *ExecutionCapabilities      `json:"execution,omitempty"`
	HumanInTheLoop *HumanInTheLoopCapabilities `json:"humanInTheLoop,omitempty"`
	// Custom is open by key, for capabilities no standard group covers.
	Custom map[string]any `json:"custom,omitzero"`
}

// IdentityCapabilities names the agent.
type IdentityCapabilities struct {
	Name             *string  `json:"name,omitempty"`
	Type             *string  `json:"type,omitempty"`
	Description      *string  `json:"description,omitempty"`
	Version          *string  `json:"version,omitempty"`
	Provider         *string  `json:"provider,omitempty"`
	DocumentationURL *string  `json:"documentationUrl,omitempty"`
	Metadata         Metadata `json:"metadata,omitzero"`
}

// TransportCapabilities declares the transport bindings the agent serves.
type TransportCapabilities struct {
	Streaming         *bool `json:"streaming,omitempty"`
	Websocket         *bool `json:"websocket,omitempty"`
	HTTPBinary        *bool `json:"httpBinary,omitempty"`
	PushNotifications *bool `json:"pushNotifications,omitempty"`
	Resumable         *bool `json:"resumable,omitempty"`
}

// ToolsCapabilities declares tool use. Items lists the tools the agent itself
// provides, distinct from RunAgentInput.Tools (the application's).
type ToolsCapabilities struct {
	Supported      *bool  `json:"supported,omitempty"`
	Items          []Tool `json:"items,omitzero"`
	ParallelCalls  *bool  `json:"parallelCalls,omitempty"`
	ClientProvided *bool  `json:"clientProvided,omitempty"`
}

// OutputCapabilities declares output shaping.
type OutputCapabilities struct {
	StructuredOutput   *bool    `json:"structuredOutput,omitempty"`
	SupportedMimeTypes []string `json:"supportedMimeTypes,omitzero"`
}

// StateCapabilities declares state handling.
type StateCapabilities struct {
	Snapshots       *bool `json:"snapshots,omitempty"`
	Deltas          *bool `json:"deltas,omitempty"`
	Memory          *bool `json:"memory,omitempty"`
	PersistentState *bool `json:"persistentState,omitempty"`
}

// SubagentInfo names a subagent an agent may invoke (a definition, not an
// invocation).
type SubagentInfo struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}

// MultiAgentCapabilities declares delegation.
type MultiAgentCapabilities struct {
	Supported  *bool          `json:"supported,omitempty"`
	Delegation *bool          `json:"delegation,omitempty"`
	Handoffs   *bool          `json:"handoffs,omitempty"`
	Subagents  []SubagentInfo `json:"subagents,omitzero"`
}

// ReasoningCapabilities declares reasoning output.
type ReasoningCapabilities struct {
	Supported *bool `json:"supported,omitempty"`
	Streaming *bool `json:"streaming,omitempty"`
	Encrypted *bool `json:"encrypted,omitempty"`
}

// MultimodalInputCapabilities declares accepted input modalities.
type MultimodalInputCapabilities struct {
	Image *bool `json:"image,omitempty"`
	Audio *bool `json:"audio,omitempty"`
	Video *bool `json:"video,omitempty"`
	PDF   *bool `json:"pdf,omitempty"`
	File  *bool `json:"file,omitempty"`
}

// MultimodalOutputCapabilities declares producible output modalities.
type MultimodalOutputCapabilities struct {
	Image *bool `json:"image,omitempty"`
	Audio *bool `json:"audio,omitempty"`
}

// MultimodalCapabilities groups input and output modalities.
type MultimodalCapabilities struct {
	Input  *MultimodalInputCapabilities  `json:"input,omitempty"`
	Output *MultimodalOutputCapabilities `json:"output,omitempty"`
}

// ExecutionCapabilities declares code execution and limits.
type ExecutionCapabilities struct {
	CodeExecution    *bool  `json:"codeExecution,omitempty"`
	Sandboxed        *bool  `json:"sandboxed,omitempty"`
	MaxIterations    *int64 `json:"maxIterations,omitempty"`
	MaxExecutionTime *int64 `json:"maxExecutionTime,omitempty"`
}

// HumanInTheLoopCapabilities declares approvals and interrupt–resume support.
type HumanInTheLoopCapabilities struct {
	Supported        *bool `json:"supported,omitempty"`
	Approvals        *bool `json:"approvals,omitempty"`
	Interventions    *bool `json:"interventions,omitempty"`
	Feedback         *bool `json:"feedback,omitempty"`
	Interrupts       *bool `json:"interrupts,omitempty"`
	ApproveWithEdits *bool `json:"approveWithEdits,omitempty"`
}
