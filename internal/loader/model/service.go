package model

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/viant/agently/genai/llm/provider"
	fs "github.com/viant/agently/internal/loader/fs"
	"github.com/viant/agently/internal/workspace"
)

const (
	defaultExtension = ".yaml"
)

// Service provides model data access operations
type Service struct {
	*fs.Service[provider.Config]
}

// New creates a new model service instance
func New(options ...fs.Option[provider.Config]) *Service {
	ret := &Service{
		Service: fs.New[provider.Config](decodeYaml, options...),
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
