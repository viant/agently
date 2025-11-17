package augmenter

import (
	"context"
	"fmt"
	"path"
	"reflect"
	"strings"

	"github.com/viant/afs"
	"github.com/viant/agently/genai/embedder"
	mcpfs "github.com/viant/agently/genai/service/augmenter/mcpfs"
	svc "github.com/viant/agently/genai/tool/service"
	mcpmgr "github.com/viant/agently/internal/mcp/manager"
	mcpuri "github.com/viant/agently/internal/mcp/uri"
	"github.com/viant/agently/internal/shared"
	embedius "github.com/viant/embedius"
	embSchema "github.com/viant/embedius/schema"
	"github.com/viant/embedius/vectordb/mem"
	"sync"
)

const name = "llm/augmenter"

// Service extracts structured information from LLM responses
type Service struct {
	finder         embedder.Finder
	DocsAugmenters shared.Map[string, *DocsAugmenter]
	// Optional MCP client manager for resolving mcp: resources during indexing
	mcpMgr *mcpmgr.Manager
	// Global writer-capable mem store reused across all augmenters
	memStore     *mem.Store
	memStoreOnce sync.Once
}

// New creates a new extractor service
func New(finder embedder.Finder, opts ...func(*Service)) *Service {
	s := &Service{
		finder:         finder,
		DocsAugmenters: shared.NewMap[string, *DocsAugmenter](),
	}
	for _, o := range opts {
		if o != nil {
			o(s)
		}
	}
	return s
}

// WithMCPManager attaches an MCP manager so the augmenter can index mcp: resources.
func WithMCPManager(m *mcpmgr.Manager) func(*Service) { return func(s *Service) { s.mcpMgr = m } }

// Name returns the service name
func (s *Service) Name() string {
	return name
}

const (
	augmentDocsMethod = "augmentDocs"
)

// Methods returns the service methods
func (s *Service) Methods() svc.Signatures {
	return []svc.Signature{
		{
			Name:     augmentDocsMethod,
			Internal: true,
			Input:    reflect.TypeOf(&AugmentDocsInput{}),
			Output:   reflect.TypeOf(&AugmentDocsOutput{}),
		},
	}
}

// Method returns the specified method
func (s *Service) Method(name string) (svc.Executable, error) {
	switch name {
	case augmentDocsMethod:
		return s.augmentDocs, nil
	default:
		return nil, svc.NewMethodNotFoundError(name)
	}
}

// augmentDocs processes LLM responses to augmentDocs with embedded context
func (s *Service) augmentDocs(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*AugmentDocsInput)
	if !ok {
		return svc.NewInvalidInputError(in)
	}
	output, ok := out.(*AugmentDocsOutput)
	if !ok {
		return svc.NewInvalidOutputError(output)
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
	var searchDocuments []embSchema.Document

	for _, location := range input.Locations {

		docs, err := service.Match(ctx, input.Query, input.MaxDocuments, location)
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

func (s *Service) includeDocuments(output *AugmentDocsOutput, input *AugmentDocsInput, searchDocuments []embSchema.Document, responseContent *strings.Builder) {
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

func (s *Service) includeDocFileContent(ctx context.Context, searchResults []embSchema.Document, input *AugmentDocsInput, responseContent *strings.Builder) {
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
			var data []byte
			var err error
			if mcpuri.Is(loc) && s.mcpMgr != nil {
				// Read via MCP
				mfs := mcpfs.New(s.mcpMgr)
				data, err = mfs.Download(ctx, mcpfs.NewObjectFromURI(loc))
			} else {
				data, err = fs.DownloadWithURL(ctx, loc)
			}
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
