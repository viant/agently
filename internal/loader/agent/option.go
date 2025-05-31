package agent

import "github.com/viant/fluxor/service/meta"

// Option represents a configuration option for the agent service
type Option func(*Service)

// WithMetaService sets the meta service for the agent service
func WithMetaService(metaService *meta.Service) Option {
	return func(s *Service) {
		s.metaService = metaService
	}
}
