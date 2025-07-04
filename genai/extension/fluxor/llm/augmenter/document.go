package augmenter

import (
	"context"
	"errors"
	"github.com/tmc/langchaingo/schema"
	"github.com/viant/embedius/matching/option"
	"strings"
)

type AugmentDocsInput struct {
	Query           string
	Locations       []string
	Match           *option.Options
	Model           string
	MaxResponseSize int //size in byte
	MaxDocuments    int
	//based on meta['path'] include full path as long it does not go over //max response size
	IncludeFile bool
	TrimPath    string //trim path prefix
}

var (
	defaultResponseSize = 32 * 1024
	defaultMaxDocuments = 40
)

func (i *AugmentDocsInput) Init(ctx context.Context) {
	// Set default values if not provided
	if i.MaxResponseSize == 0 {
		i.MaxResponseSize = defaultResponseSize // Default to 10KB
	}
	if i.MaxDocuments == 0 {
		i.MaxDocuments = defaultMaxDocuments
	}

}

func (i *AugmentDocsInput) Validate(ctx context.Context) error {
	if i.Model == "" {
		return errors.New("embeddings is required")
	}
	if len(i.Locations) == 0 {
		return errors.New("locations is required")
	}
	if i.Query == "" {
		return errors.New("query is required")
	}
	return nil
}

func (i *AugmentDocsInput) Location(location string) string {
	if i.TrimPath == "" {
		return location
	}
	return strings.TrimPrefix(location, i.TrimPath)
}

// AugmentDocsOutput represents output from extraction
type AugmentDocsOutput struct {
	Content       string
	Documents     []schema.Document
	DocumentsSize int
}
