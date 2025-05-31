package model

import (
	"github.com/viant/agently/genai/embedder/provider"
	fs2 "github.com/viant/agently/internal/loader/fs"
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
