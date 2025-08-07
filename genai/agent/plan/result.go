package plan

// Result represents the result of a tool invocation in a plan.
type Result struct {
	ID string `json:"id"`
	// Name is the name of the tool or step invoked
	Name string `json:"name"`
	// Args holds the original arguments passed to the tool
	Args map[string]interface{} `json:"args"`
	// Result is the string output from the tool invocation
	Result string `json:"result"`
	//Error tool call error
	Error string `json:"error,omitempty"`
	Seen  bool   `json:"seen"` // Seen indicates if the result was seen by LLM
}
