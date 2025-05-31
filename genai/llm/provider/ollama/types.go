package ollama

// Request represents the request structure for Ollama API
type Request struct {
	Model    string   `json:"model"`
	Prompt   string   `json:"prompt"`
	System   string   `json:"system,omitempty"`
	Template string   `json:"template,omitempty"`
	Context  []int    `json:"context,omitempty"`
	Format   string   `json:"format,omitempty"`
	Stream   bool     `json:"stream"`
	Raw      bool     `json:"raw,omitempty"`
	Options  *Options `json:"options,omitempty"`
}

// Options represents the options for the Ollama API request
type Options struct {
	Temperature   float64  `json:"temperature,omitempty"`
	TopP          float64  `json:"top_p,omitempty"`
	TopK          int      `json:"top_k,omitempty"`
	RepeatPenalty float64  `json:"repeat_penalty,omitempty"`
	Seed          int      `json:"seed,omitempty"`
	NumPredict    int      `json:"num_predict,omitempty"`
	Stop          []string `json:"stop,omitempty"`
}

// Response represents the response structure from Ollama API
type Response struct {
	Model              string `json:"model"`
	CreatedAt          string `json:"created_at"`
	Response           string `json:"response"`
	Done               bool   `json:"done"`
	Context            []int  `json:"context,omitempty"`
	TotalDuration      int64  `json:"total_duration,omitempty"`
	LoadDuration       int64  `json:"load_duration,omitempty"`
	PromptEvalCount    int    `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64  `json:"prompt_eval_duration,omitempty"`
	EvalCount          int    `json:"eval_count,omitempty"`
	EvalDuration       int64  `json:"eval_duration,omitempty"`
}

// PullRequest represents the request structure for pulling a model from Ollama API
type PullRequest struct {
	Stream bool   `json:"stream"`
	Name   string `json:"name"`
}

// PullResponse represents the response structure from pulling a model from Ollama API
type PullResponse struct {
	Status string `json:"status"`
	Digest string `json:"digest,omitempty"`
	Total  int64  `json:"total,omitempty"`
}
