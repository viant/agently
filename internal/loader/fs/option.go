package fs

import "github.com/viant/fluxor/service/meta"

type Option[T any] func(s *Service[T])

func WithMetaService[T any](meta *meta.Service) Option[T] {
	return func(s *Service[T]) {
		s.metaService = meta
	}
}
