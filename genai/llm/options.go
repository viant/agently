package llm

type Options struct {
	// Model is the model to use.
	Model string `json:"model" yaml:"model"`

	// CandidateCount is the number of response candidates to generate.
	CandidateCount int `json:"candidate_count" yaml:"candidate_count"`

	// MaxTokens is the maximum number of tokens to generate.
	MaxTokens int `json:"max_tokens" yaml:"max_tokens"`

	// Temperature is the temperature for sampling, between 0 and 1.
	Temperature float64 `json:"temperature" yaml:"temperature"`

	// StopWords is a list of words to stop on.
	StopWords []string `json:"stop_words" yaml:"stop_words"`

	// TopK is the number of tokens to consider for top-k sampling.
	TopK int `json:"top_k" yaml:"top_k"`

	// TopP is the cumulative probability for top-p sampling.
	TopP float64 `json:"top_p" yaml:"top_p"`

	// Seed is a seed for deterministic sampling.
	Seed int `json:"seed" yaml:"seed"`

	// MinLength is the minimum length of the generated text.
	MinLength int `json:"min_length" yaml:"min_length"`

	// MaxLength is the maximum length of the generated text.
	MaxLength int `json:"max_length" yaml:"max_length"`

	// N is how many chat completion choices to generate for each input message.
	N int `json:"n" yaml:"n"`

	// RepetitionPenalty is the repetition penalty for sampling.
	RepetitionPenalty float64 `json:"repetition_penalty" yaml:"repetition_penalty"`

	// FrequencyPenalty is the frequency penalty for sampling.
	FrequencyPenalty float64 `json:"frequency_penalty" yaml:"frequency_penalty"`

	// PresencePenalty is the presence penalty for sampling.
	PresencePenalty float64 `json:"presence_penalty" yaml:"presence_penalty"`

	// JSONMode is a flag to enable JSON mode.
	JSONMode bool `json:"json" yaml:"json"`

	// Tools is a list of tools to use. Each tool can be a specific tool or a function.
	Tools []Tool `json:"tools,omitempty" yaml:"tools,omitempty"`

	// ToolChoice is the choice of tool to use: "none", "auto" (default), or a specific tool.
	ToolChoice ToolChoice `json:"tool_choice,omitempty" yaml:"tool_choice,omitempty"`

	// Metadata is a map of metadata to include in the request.
	// The meaning of this field is specific to the backend in use.
	Metadata map[string]interface{} `json:"metadata,omitempty" yaml:"metadata,omitempty"`

	// ResponseMIMEType MIME type of the generated candidate text.
	// Supported MIME types: text/plain (default), application/json.
	ResponseMIMEType string `json:"response_mime_type,omitempty" yaml:"response_mime_type,omitempty"`

	Thinking *Thinking `json:"thinking,omitempty" yaml:"thinking,omitempty"`
	// Reasoning configures the model's reasoning behavior, e.g. summarization of chain-of-thought.
	Reasoning *Reasoning `json:"reasoning,omitempty" yaml:"reasoning,omitempty"`

	// Stream enables streaming responses.
	Stream bool `json:"stream,omitempty" yaml:"stream,omitempty"`

	Mode string `json:"mode,omitempty" yaml:"mode,omitempty"`

	// ParallelToolCalls requests the provider to execute multiple tool calls
	// in parallel within a single reasoning step, when supported by the model.
	// This is honored by providers that implement base.CanExecToolsInParallel
	// (e.g., OpenAI). Others will ignore it.
	ParallelToolCalls bool `json:"parallel_tool_calls,omitempty" yaml:"parallelToolCalls,omitempty"`

	// ContinuationContext explicitly enables/disables provider continuation
	// via previous_response_id for models that support it.
	//
	// Semantics:
	//   - nil           
	//            //    default behaviour based on model capability
	//        //    (enabled when the model implements SupportsContinuationByResponseID).
	//   - true    
	//            //    explicitly enable continuation when the provider supports it.
	//   - false   
	//            //    explicitly disable continuation even when the model supports it.
	ContinuationContext *bool `json:"continuationContext,omitempty" yaml:"continuationContext,omitempty"`
}

type Thinking struct {
	Type         string `json:"type" yaml:"type"`
	BudgetTokens int    `json:"budget_tokens,omitempty" yaml:"budget_tokens,omitempty"`
}

// Reasoning specifies options for the model's internal reasoning process.
// Summary may be set to "auto" to request an automatic summary of chain-of-thought.
type Reasoning struct {
	// Effort controls provider-native reasoning depth. For OpenAI o-series
	// valid values are: "low" | "medium" | "high".
	Effort  string `json:"effort,omitempty" yaml:"effort,omitempty"`
	Summary string `json:"summary,omitempty" yaml:"summary,omitempty"`
}
