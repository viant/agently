package model

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/viant/agently/genai/llm/provider"
	"github.com/viant/agently/internal/workspace"
	fs2 "github.com/viant/agently/internal/workspace/loader/fs"
)

const (
	defaultExtension = ".yaml"
)

// Service provides model data access operations
type Service struct {
	*fs2.Service[provider.Config]
}

// New creates a new model service instance
func New(options ...fs2.Option[provider.Config]) *Service {
	ret := &Service{
		Service: fs2.New[provider.Config](decodeYaml, options...),
	}
	return ret
}

// Load resolves bare model names against the standard workspace folder before
// delegating to the generic FS loader so that callers can simply refer to
// "o3" instead of "models/o4-mini.yaml".
func (s *Service) Load(ctx context.Context, URL string) (*provider.Config, error) {
	if !strings.Contains(URL, "/") && filepath.Ext(URL) == "" {
		URL = filepath.Join(workspace.KindModel, URL)
	}
	return s.Service.Load(ctx, URL)
}
