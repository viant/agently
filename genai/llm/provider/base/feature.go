package base

const (
	CanUseTools            string = "can-use-tools"
	CanStream              string = "can-stream"
	IsMultimodal           string = "is-multimodal"
	CanExecToolsInParallel string = "can-exec-tools-in-parallel"
	// SupportsContextContinuation indicates the provider can continue
	// a conversation by passing a prior response identifier (e.g., OpenAI
	// /v1/responses via previous_response_id).
	SupportsContextContinuation string = "supports-context-continuation"
	// SupportsInstructions indicates the provider supports top-level
	// instructions/system guidance outside the message list.
	SupportsInstructions string = "supports-instructions"
)
