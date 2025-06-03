package gemini

// Request represents the request structure for Gemini API
type Request struct {
	Contents          []Content          `json:"contents"`
	Stream            bool               `json:"stream,omitempty"`
	CachedContent     string             `json:"cachedContent,omitempty"`
	SystemInstruction *SystemInstruction `json:"systemInstruction,omitempty"`
	GenerationConfig  *GenerationConfig  `json:"generationConfig,omitempty"`
	SafetySettings    []SafetySetting    `json:"safetySettings,omitempty"`
	Tools             []Tool             `json:"tools,omitempty"`
	ToolConfig        *ToolConfig        `json:"toolConfig,omitempty"`
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
	InlineData       *InlineData       `json:"inlineData,omitempty"`
	FileData         *FileData         `json:"fileData,omitempty"`
	VideoMetadata    *VideoMetadata    `json:"videoMetadata,omitempty"`
	FunctionCall     *FunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *FunctionResponse `json:"functionResponse,omitempty"`
}

// FileData represents file data in the Gemini API
type FileData struct {
	MimeType string `json:"mimeType"`
	FileURI  string `json:"fileUri"`
}

// VideoMetadata represents video metadata in the Gemini API
type VideoMetadata struct {
	StartOffset *Offset `json:"startOffset,omitempty"`
	EndOffset   *Offset `json:"endOffset,omitempty"`
}

// Offset represents a time offset in the Gemini API
type Offset struct {
	Seconds int `json:"seconds"`
	Nanos   int `json:"nanos"`
}

// InlineData represents inline data (like images) in the Gemini API
type InlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

// GenerationConfig represents generation configuration for the Gemini API
type GenerationConfig struct {
	Temperature      float64     `json:"temperature,omitempty"`
	MaxOutputTokens  int         `json:"maxOutputTokens,omitempty"`
	TopP             float64     `json:"topP,omitempty"`
	TopK             int         `json:"topK,omitempty"`
	CandidateCount   int         `json:"candidateCount,omitempty"`
	StopSequences    []string    `json:"stopSequences,omitempty"`
	PresencePenalty  float64     `json:"presencePenalty,omitempty"`
	FrequencyPenalty float64     `json:"frequencyPenalty,omitempty"`
	ResponseMIMEType string      `json:"responseMimeType,omitempty"`
	ResponseSchema   interface{} `json:"responseSchema,omitempty"`
	Seed             int         `json:"seed,omitempty"`
	ResponseLogprobs bool        `json:"responseLogprobs,omitempty"`
	Logprobs         int         `json:"logprobs,omitempty"`
	AudioTimestamp   bool        `json:"audioTimestamp,omitempty"`
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
}

// FunctionCall represents a function call in the Gemini API
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// FunctionResponse represents a function response in the Gemini API
type FunctionResponse struct {
	Name     string `json:"name"`
	Response string `json:"response"`
}

// Response represents the response structure from Gemini API
type Response struct {
	Candidates     []Candidate     `json:"candidates"`
	PromptFeedback *PromptFeedback `json:"promptFeedback,omitempty"`
	UsageMetadata  *UsageMetadata  `json:"usageMetadata,omitempty"`
	ModelVersion   string          `json:"modelVersion,omitempty"`
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
}
