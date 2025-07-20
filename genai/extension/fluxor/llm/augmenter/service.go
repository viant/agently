package augmenter

import (
	"context"
	"fmt"
	"github.com/tmc/langchaingo/schema"
	"github.com/tmc/langchaingo/vectorstores"
	"github.com/viant/afs"
	"github.com/viant/agently/genai/embedder"
	"github.com/viant/agently/internal/shared"
	embedius "github.com/viant/embedius"
	"github.com/viant/fluxor/model/types"
	"path"
	"reflect"
	"strings"
)

const name = "llm/augmenter"

// Service extracts structured information from LLM responses
type Service struct {
	finder         embedder.Finder
	DocsAugmenters shared.Map[string, *DocsAugmenter]
	CodeAugmenters shared.Map[string, *CodeAugmenter]
}

// New creates a new extractor service
func New(finder embedder.Finder) *Service {
	return &Service{
		finder:         finder,
		DocsAugmenters: shared.NewMap[string, *DocsAugmenter](),
		CodeAugmenters: shared.NewMap[string, *CodeAugmenter](),
	}
}

// Name returns the service name
func (s *Service) Name() string {
	return name
}

const (
	augmentDocsMethod = "augmentDocs"
	augmentCodeMethod = "augmentCode"
)

// Methods returns the service methods
func (s *Service) Methods() types.Signatures {
	return []types.Signature{
		{
			Name:   augmentDocsMethod,
			Input:  reflect.TypeOf(&AugmentDocsInput{}),
			Output: reflect.TypeOf(&AugmentDocsOutput{}),
		},
		{
			Name:   augmentCodeMethod,
			Input:  reflect.TypeOf(&AugmentCodeInput{}),
			Output: reflect.TypeOf(&AugmentCodeOutput{}),
		},
	}
}

// Method returns the specified method
func (s *Service) Method(name string) (types.Executable, error) {
	switch name {
	case augmentDocsMethod:
		return s.augmentDocs, nil
	case augmentCodeMethod:

		return s.augmentCode, nil
	default:
		return nil, types.NewMethodNotFoundError(name)
	}
}

// augmentCode processes LLM responses to code with embedded context
func (s *Service) augmentCode(ctx context.Context, in, out interface{}) error {

	input, ok := in.(*AugmentCodeInput)
	if !ok {
		return types.NewInvalidInputError(in)
	}
	output, ok := out.(*AugmentCodeOutput)
	if !ok {
		return types.NewInvalidOutputError(output)
	}

	return s.AugmentCode(ctx, input, output)
}

func (s *Service) AugmentCode(ctx context.Context, input *AugmentCodeInput, output *AugmentCodeOutput) error {
	input.Init(ctx)
	err := input.Validate(ctx)
	if err != nil {
		return fmt.Errorf("failed to init input: %w", err)
	}

	augmenter, err := s.getCodeAugmenter(ctx, input)
	if err != nil {
		return err
	}
	service := embedius.NewService(augmenter.service)
	var searchDocuments []schema.Document

	var vectorOptions []vectorstores.Option
	if input.Match != nil {
		vectorOptions = append(vectorOptions, vectorstores.WithFilters(input.Match.Options()))
	}

	for _, location := range input.Locations {
		docs, err := service.Match(ctx, input.Query, input.MaxDocuments, location, vectorOptions...)
		if err != nil {
			return fmt.Errorf("failed to augmentDocs documents: %w", err)
		}
		searchDocuments = append(searchDocuments, docs...)
	}

	output.Documents = Documents(searchDocuments).ProjectDocuments()
	output.DocumentsSize = Documents(searchDocuments).Size()
	output.GroupedBy = output.Documents.GroupBy()

	// Ensure the set for the provided paths or kind
	content := strings.Builder{}
	documents := output.Documents
	if input.ShallUseGroup() {
		documents = output.GroupedBy
	}
	sizeSoFar := 0
	for _, doc := range documents {
		if sizeSoFar+doc.Size() > input.MaxResponseSize {
			break
		}
		size, _ := s.addDocumentContent(&content, doc.Path, doc.Content)
		sizeSoFar += size
	}
	output.Content = content.String()
	return nil
}

// augmentDocs processes LLM responses to augmentDocs with embedded context
func (s *Service) augmentDocs(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*AugmentDocsInput)
	if !ok {
		return types.NewInvalidInputError(in)
	}
	output, ok := out.(*AugmentDocsOutput)
	if !ok {
		return types.NewInvalidOutputError(output)
	}

	return s.AugmentDocs(ctx, input, output)
}

func (s *Service) AugmentDocs(ctx context.Context, input *AugmentDocsInput, output *AugmentDocsOutput) error {
	input.Init(ctx)
	err := input.Validate(ctx)
	if err != nil {
		return fmt.Errorf("failed to init input: %w", err)
	}
	augmenter, err := s.getDocAugmenter(ctx, input)
	if err != nil {
		return err
	}
	service := embedius.NewService(augmenter.service)
	var searchDocuments []schema.Document
	var vectorOptions []vectorstores.Option
	if input.Match != nil {
		vectorOptions = append(vectorOptions, vectorstores.WithFilters(input.Match.Options()))
	}

	for _, location := range input.Locations {
		docs, err := service.Match(ctx, input.Query, input.MaxDocuments, location, vectorOptions...)
		if err != nil {
			return fmt.Errorf("failed to augmentDocs documents: %w", err)
		}
		searchDocuments = append(searchDocuments, docs...)
	}
	output.Documents = searchDocuments
	output.DocumentsSize = Documents(searchDocuments).Size()

	// Ensure the set for the provided paths or kind
	responseContent := strings.Builder{}

	if input.IncludeFile {
		s.includeDocFileContent(ctx, searchDocuments, input, &responseContent)
	} else {
		s.includeDocuments(output, input, searchDocuments, &responseContent)
	}
	output.Content = responseContent.String()
	return nil
}

func (s *Service) includeDocuments(output *AugmentDocsOutput, input *AugmentDocsInput, searchDocuments []schema.Document, responseContent *strings.Builder) {
	documentSize := output.DocumentsSize
	if documentSize < input.MaxResponseSize {
		for _, doc := range searchDocuments {
			loc := input.Location(getStringFromMetadata(doc.Metadata, "path"))
			_, _ = s.addDocumentContent(responseContent, loc, doc.PageContent)
		}

		return
	}

	sizeSoFar := 0
	for _, doc := range searchDocuments {
		if sizeSoFar+Document(doc).Size() >= input.MaxResponseSize {
			break
		}
		sizeSoFar += Document(doc).Size()
		loc := input.Location(getStringFromMetadata(doc.Metadata, "path"))
		_, _ = s.addDocumentContent(responseContent, loc, doc.PageContent)
	}
}

func (s *Service) includeDocFileContent(ctx context.Context, searchResults []schema.Document, input *AugmentDocsInput, responseContent *strings.Builder) {
	fs := afs.New()
	documentSize := Documents(searchResults).Size()
	var unique = make(map[string]bool)

	sizeSoFar := 0
	if documentSize < input.MaxResponseSize {
		for _, doc := range searchResults {
			loc := input.Location(getStringFromMetadata(doc.Metadata, "path"))
			if loc != "" {
				if unique[loc] {
					continue
				}
				unique[loc] = true
			}
			data, err := fs.DownloadWithURL(ctx, loc)
			if err != nil {
				continue
			}
			//TODO add template based output
			if sizeSoFar+len(data) <= input.MaxResponseSize {
				_, _ = s.addDocumentContent(responseContent, loc, string(data))
			} else if sizeSoFar+Document(doc).Size() <= input.MaxResponseSize {
				_, _ = s.addDocumentContent(responseContent, loc, doc.PageContent)
			}

		}
	}
}

func (s *Service) addDocumentContent(response *strings.Builder, loc string, content string) (int, error) {
	return response.WriteString(fmt.Sprintf("file: %v\n```%v\n%v\n````\n\n", loc, strings.Trim(path.Ext(loc), "."), content))
}

// Helper function to safely extract a string from metadata
func getStringFromMetadata(metadata map[string]any, key string) string {
	if value, ok := metadata[key]; ok {
		text, _ := value.(string)
		return text
	}
	return ""
}
