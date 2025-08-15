package stream

// Event represents a partial or complete event in a streaming LLM response.
// It captures text chunks, function calls, and final finish reasons.
type Event struct {
	Type         string                 `json:"type"`                   // chunk, function_call or done or error
	Content      string                 `json:"content,omitempty"`      // text content for chunk events
	Name         string                 `json:"name,omitempty"`         // function name for function_call events
	Arguments    map[string]interface{} `json:"arguments,omitempty"`    // function arguments for function_call events
	FinishReason string                 `json:"finishReason,omitempty"` // finish_reason for done events
}
