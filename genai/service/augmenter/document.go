package augmenter

import (
	"context"
	"errors"
	"strings"

	"github.com/viant/afs"
	"github.com/viant/embedius/matching/option"
	embSchema "github.com/viant/embedius/schema"
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
	Documents     []embSchema.Document
	DocumentsSize int
}

func (o *AugmentDocsOutput) LoadDocuments(ctx context.Context, fs afs.Service) []embSchema.Document {
	var result = make([]embSchema.Document, 0, len(o.Documents))
	var unique = make(map[string]bool)
	for _, doc := range o.Documents {
		key, ok := doc.Metadata["path"]
		if !ok {
			key = doc.Metadata["docId"]
		}
		uri, ok := key.(string)
		if !ok {
			continue
		}
		if unique[uri] {
			continue
		}
		content, err := fs.DownloadWithURL(ctx, uri)
		if err != nil {
			continue
		}
		unique[uri] = true
		result = append(result, embSchema.Document{Metadata: doc.Metadata, PageContent: string(content)})
	}
	return result
}
