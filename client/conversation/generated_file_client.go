package conversation

import (
	"context"

	gfread "github.com/viant/agently/pkg/agently/generatedfile/read"
	gfwrite "github.com/viant/agently/pkg/agently/generatedfile/write"
)

// GeneratedFileClient is an optional extension implemented by concrete
// conversation clients that support generated-file persistence.
type GeneratedFileClient interface {
	GetGeneratedFiles(ctx context.Context, input *gfread.Input) ([]*gfread.GeneratedFileView, error)
	PatchGeneratedFile(ctx context.Context, generatedFile *gfwrite.GeneratedFile) error
}
