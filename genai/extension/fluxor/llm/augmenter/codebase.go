package augmenter

import (
	"context"
	"errors"
	"github.com/viant/embedius/matching/option"
	"github.com/viant/linager/inspector/graph"
)

type AugmentCodeInput struct {
	Query           string          `json:"query,omitempty"`
	Locations       []string        `json:"locations,omitempty"`
	Match           *option.Options `json:"match,omitempty"`
	Model           string          `json:"model,omitempty"`
	MaxResponseSize int             `json:"maxResponseSize,omitempty"` //size in byte
	MaxDocuments    int             `json:"maxDocuments,omitempty"`
	UseGroup        *bool           `json:"useGroup,omitempty"`
}

func (i *AugmentCodeInput) Init(ctx context.Context) {
	// Set default values if not provided
	if i.MaxResponseSize == 0 {
		i.MaxResponseSize = defaultResponseSize // Default to 10KB
	}
	if i.MaxDocuments == 0 {
		i.MaxDocuments = defaultMaxDocuments
	}

}

func (i *AugmentCodeInput) ShallUseGroup() bool {
	if i.UseGroup == nil {
		return true
	}

	return *i.UseGroup
}

func (i *AugmentCodeInput) Validate(ctx context.Context) error {
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

// AugmentCodeOutput represents output from extraction
type AugmentCodeOutput struct {
	Content       string
	Documents     graph.Documents
	GroupedBy     graph.Documents
	DocumentsSize int
}
