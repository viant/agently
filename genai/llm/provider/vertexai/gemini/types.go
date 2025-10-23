package gemini

// Request represents the request structure for Gemini API
type Request struct {
	GenerationConfig  *GenerationConfig  `json:"generationConfig,omitempty"`
	ToolConfig        *ToolConfig        `json:"toolConfig,omitempty"`
	Tools             []Tool             `json:"tools,omitempty"`
	SystemInstruction *SystemInstruction `json:"system_instruction,omitempty"`
	Contents          []Content          `json:"contents"`
	Stream            bool               `json:"stream,omitempty"`
	CachedContent     string             `json:"cachedContent,omitempty"`
	SafetySettings    []SafetySetting    `json:"safetySettings,omitempty"`
	Labels            map[string]string  `json:"labels,omitempty"`
}

// Content represents a content in the Gemini API request
type Content struct {
	Role  string `json:"role,omitempty"`
	Parts []Part `json:"parts"`
}

// SystemInstruction represents a system instruction in the Gemini API request
type SystemInstruction struct {
	Role  string `json:"role"`
	Parts []Part `json:"parts"`
}

// Part represents a part in a content for the Gemini API
type Part struct {
	Text             string            `json:"text,omitempty"`
	InlineData       *InlineData       `json:"inline_data,omitempty"`
	FileData         *FileData         `json:"file_data,omitempty"`
	VideoMetadata    *VideoMetadata    `json:"video_metadata,omitempty"`
	FunctionCall     *FunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *FunctionResponse `json:"functionResponse,omitempty"`
}

// FileData represents file data in the Gemini API
type FileData struct {
	MimeType string `json:"mime_type"`
	FileURI  string `json:"file_uri"`
}

// VideoMetadata represents video metadata in the Gemini API
type VideoMetadata struct {
	StartOffset *Offset `json:"start_offset,omitempty"`
	EndOffset   *Offset `json:"end_offset,omitempty"`
}

// Offset represents a time offset in the Gemini API
type Offset struct {
	Seconds int `json:"seconds"`
	Nanos   int `json:"nanos"`
}

// InlineData represents inline data (like images) in the Gemini API
type InlineData struct {
	MimeType string `json:"mime_type"`
	Data     string `json:"data"`
}

// GenerationConfig represents generation configuration for the Gemini API
type GenerationConfig struct {
	Temperature      float64         `json:"temperature,omitempty"`
	MaxOutputTokens  int             `json:"maxOutputTokens,omitempty"`
	TopP             float64         `json:"topP,omitempty"`
	TopK             int             `json:"topK,omitempty"`
	CandidateCount   int             `json:"candidateCount,omitempty"`
	StopSequences    []string        `json:"stopSequences,omitempty"`
	PresencePenalty  float64         `json:"presencePenalty,omitempty"`
	FrequencyPenalty float64         `json:"frequencyPenalty,omitempty"`
	ResponseMIMEType string          `json:"responseMimeType,omitempty"`
	ResponseSchema   interface{}     `json:"responseSchema,omitempty"`
	Seed             int             `json:"seed,omitempty"`
	ResponseLogprobs bool            `json:"responseLogprobs,omitempty"`
	Logprobs         int             `json:"logprobs,omitempty"`
	AudioTimestamp   bool            `json:"audioTimestamp,omitempty"`
	ThinkingConfig   *ThinkingConfig `json:"thinkingConfig,omitempty"`
}

// ThinkingConfig controls model "thinking" behaviour for Gemini 2.5 flash
type ThinkingConfig struct {
	ThinkingBudget int `json:"thinkingBudget,omitempty"`
}

// SafetySetting represents a safety setting for the Gemini API
type SafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

// Tool represents a tool in the Gemini API
type Tool struct {
	FunctionDeclarations []FunctionDeclaration `json:"functionDeclarations"`
}

// FunctionDeclaration represents a function declaration in the Gemini API
type FunctionDeclaration struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// ToolConfig represents tool configuration in the Gemini API
type ToolConfig struct {
	FunctionCallingConfig *FunctionCallingConfig `json:"functionCallingConfig,omitempty"`
}

// FunctionCallingConfig represents function calling configuration in the Gemini API
type FunctionCallingConfig struct {
	Mode string `json:"mode,omitempty"` // "AUTO" or "ANY" or "NONE"
	// AllowedFunctionNames enumerates the function names the model is permitted to call.
	// When empty the backend interprets it as "no restriction" and may call any declared function.
	AllowedFunctionNames []string `json:"allowed_function_names,omitempty"`
}

// FunctionCall represents a function call in the Gemini API
type FunctionCall struct {
	Name      string      `json:"name"`
	Args      interface{} `json:"args,omitempty"`      // v1beta responses
	Arguments string      `json:"arguments,omitempty"` // request side uses this string form
}

// FunctionResponse represents a function response in the Gemini API
type FunctionResponse struct {
	Name     string      `json:"name"`
	Response interface{} `json:"response"`
}

// Response represents the response structure from Gemini API
type Response struct {
	Candidates     []Candidate     `json:"candidates"`
	PromptFeedback *PromptFeedback `json:"promptFeedback,omitempty"`
	UsageMetadata  *UsageMetadata  `json:"usageMetadata,omitempty"`
	ModelVersion   string          `json:"modelVersion,omitempty"`
	ResponseID     string          `json:"responseId,omitempty"`
}

// Candidate represents a candidate in the Gemini API response
type Candidate struct {
	Content          Content           `json:"content"`
	FinishReason     string            `json:"finishReason,omitempty"`
	Index            int               `json:"index"`
	SafetyRatings    []SafetyRating    `json:"safetyRatings,omitempty"`
	CitationMetadata *CitationMetadata `json:"citationMetadata,omitempty"`
	AvgLogprobs      float64           `json:"avgLogprobs,omitempty"`
	LogprobsResult   *LogprobsResult   `json:"logprobsResult,omitempty"`
}

// CitationMetadata represents citation metadata in the Gemini API response
type CitationMetadata struct {
	Citations []Citation `json:"citations,omitempty"`
}

// Citation represents a citation in the Gemini API response
type Citation struct {
	StartIndex      int    `json:"startIndex"`
	EndIndex        int    `json:"endIndex"`
	URI             string `json:"uri,omitempty"`
	Title           string `json:"title,omitempty"`
	License         string `json:"license,omitempty"`
	PublicationDate *Date  `json:"publicationDate,omitempty"`
}

// Date represents a date in the Gemini API response
type Date struct {
	Year  int `json:"year"`
	Month int `json:"month,omitempty"`
	Day   int `json:"day,omitempty"`
}

// LogprobsResult represents logprobs result in the Gemini API response
type LogprobsResult struct {
	TopCandidates    []TokenCandidates `json:"topCandidates,omitempty"`
	ChosenCandidates []TokenLogprob    `json:"chosenCandidates,omitempty"`
}

// TokenCandidates represents token candidates in the Gemini API response
type TokenCandidates struct {
	Candidates []TokenLogprob `json:"candidates"`
}

// TokenLogprob represents a token logprob in the Gemini API response
type TokenLogprob struct {
	Token          string  `json:"token"`
	LogProbability float32 `json:"logProbability"`
}

// SafetyRating represents a safety rating in the Gemini API response
type SafetyRating struct {
	Category    string `json:"category"`
	Probability string `json:"probability"`
}

// PromptFeedback represents feedback about the prompt in the Gemini API response
type PromptFeedback struct {
	SafetyRatings []SafetyRating `json:"safetyRatings,omitempty"`
}

// UsageMetadata represents token usage information in the Gemini API response
type UsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`

	ThoughtsTokenCount  int           `json:"thoughtsTokenCount,omitempty"`
	PromptTokensDetails []TokenDetail `json:"promptTokensDetails,omitempty"`
	ResponseID          string        `json:"responseId,omitempty"` // top-level in response but convenient here
}

// TokenDetail represents details of tokens per modality
type TokenDetail struct {
	Modality   string `json:"modality"`
	TokenCount int    `json:"tokenCount"`
}
