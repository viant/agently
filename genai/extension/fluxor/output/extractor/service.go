package extractor

import (
	"context"
	"github.com/tmc/langchaingo/llms"
	"github.com/viant/afs"
	"github.com/viant/afs/file"
	"github.com/viant/afs/url"
	extractor2 "github.com/viant/agently/genai/io/extractor"
	"github.com/viant/fluxor/model/types"
	"reflect"
	"strings"
)

const name = "output/extractor"

// Service extracts structured information from LLM responses
type Service struct{ fs afs.Service }

// Input represents input for extraction
type Input struct {
	// Response from the language model
	Response *llms.ContentResponse `json:"response"`

	Content string `json:"content,omitempty"`

	// Optional section to extract
	Section string `json:"section,omitempty"`

	CodeDestURL    string `json:"codeDestURL,omitempty"`
	CodeRepository string `json:"codeRepository,omitempty"`
}

func (i *Input) Init(ctx context.Context) {
	if i.Content != "" {
		return
	}
	// Build full response text
	responseBuilder := strings.Builder{}
	for _, choice := range i.Response.Choices {
		responseBuilder.WriteString(choice.Content)
	}
	i.Content = responseBuilder.String()
}

// Output represents output from extraction
type Output struct {
	// Extracted section
	Section *extractor2.Section `json:"section"`

	// If a specific section was requested, this contains its content
	Content string `json:"content,omitempty"`

	// Extracted code blocks
	CodeBlocks []extractor2.CodeBlock `json:"codeBlocks,omitempty"`

	UploadPaths []string `json:"uploadedURL,omitempty"`
}

// New creates a new extractor service
func New() *Service {
	return &Service{fs: afs.New()}
}

// Name returns the service name
func (s *Service) Name() string {
	return name
}

// Methods returns the service methods
func (s *Service) Methods() types.Signatures {
	return []types.Signature{
		{
			Name:   "extract",
			Input:  reflect.TypeOf(&Input{}),
			Output: reflect.TypeOf(&Output{}),
		},
	}
}

// Method returns the specified method
func (s *Service) Method(name string) (types.Executable, error) {
	switch strings.ToLower(name) {
	case "extract":
		return s.extract, nil
	default:
		return nil, types.NewMethodNotFoundError(name)
	}
}

// extract processes LLM responses to extract structured data
func (s *Service) extract(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*Input)
	if !ok {
		return types.NewInvalidInputError(in)
	}
	output, ok := out.(*Output)
	if !ok {
		return types.NewInvalidOutputError(out)
	}
	input.Init(ctx)
	// Parse the response
	var err error
	output.Section, err = extractor2.ParseLLMResponse([]byte(input.Content))
	if err != nil {
		return err
	}
	// Extract code blocks
	output.CodeBlocks = output.Section.CollectCodeBlocks()
	// Extract specific section if requested
	if input.Section != "" {
		output.Content = output.Section.Match(input.Section)
	}
	if input.CodeDestURL == "" {
		return nil
	}
	for _, codeBlock := range output.CodeBlocks {
		if codeBlock.Location == "" {
			continue
		}
		if input.CodeRepository != "" {
			codeBlock.Location = RelativeRepoPath(input.CodeRepository, codeBlock.Location)
		}
		destURL := url.Join(input.CodeDestURL, codeBlock.Location)
		_ = s.fs.Upload(ctx, destURL, file.DefaultFileOsMode, strings.NewReader(codeBlock.Content))
		output.UploadPaths = append(output.UploadPaths, destURL)
	}
	return nil
}

func RelativeRepoPath(repoRoot, absolutePath string) string {
	idx := strings.Index(absolutePath, repoRoot)
	if idx == -1 {
		return absolutePath
	}
	return absolutePath[idx:]
}
