package model

import (
	"github.com/viant/agently/genai/embedder/provider"
	"github.com/viant/agently/internal/workspace/loader/fs"
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
