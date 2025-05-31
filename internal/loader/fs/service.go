package fs

import (
	"context"
	"fmt"
	"github.com/viant/afs"
	"github.com/viant/fluxor/service/meta"
	"github.com/viant/fluxor/service/meta/yml"
	"gopkg.in/yaml.v3"
	"path/filepath"
)

const (
	defaultExtension = ".yaml"
)

// Service provides model data access operations
type Service[T any] struct {
	decoderFunc      DecodeFunc[T]
	metaService      *meta.Service
	defaultExtension string
}

func (s *Service[T]) List(ctx context.Context, URL string) ([]*T, error) {
	candidates, err := s.metaService.List(ctx, URL)
	if err != nil {
		return nil, fmt.Errorf("failed to list configs from %s: %w", URL, err)
	}
	var result = make([]*T, 0)
	for _, candidate := range candidates {
		config, err := s.Load(ctx, candidate)
		if err != nil {
			return nil, fmt.Errorf("failed to load configuration from %s: %w", candidate, err)
		}
		result = append(result, config)
	}
	return result, nil
}

// Load loads a model from the specified URL
func (s *Service[T]) Load(ctx context.Context, URL string) (*T, error) {
	ext := filepath.Ext(URL)
	if ext == "" {
		URL += s.defaultExtension
	}

	var node yaml.Node
	if err := s.metaService.Load(ctx, URL, &node); err != nil {
		return nil, fmt.Errorf("failed to load configuration from %s: %w", URL, err)
	}

	var t T
	if err := s.decoderFunc((*yml.Node)(&node), &t); err != nil {
		return nil, fmt.Errorf("failed to decode configuration from %s: %w", URL, err)
	}

	ptrT := &t
	// Use type assertion with interface{} first to avoid compile-time type checking issues with generics
	if validator, ok := any(ptrT).(Validator); ok {
		// Validate model
		if err := validator.Validate(ctx); err != nil {
			return nil, fmt.Errorf("invalid model configuration from %s: %w", URL, err)
		}
	}

	return &t, nil
}

// New creates a new model service instance
func New[T any](decoderFunc DecodeFunc[T], options ...Option[T]) *Service[T] {
	ret := &Service[T]{
		metaService:      meta.New(afs.New(), ""),
		defaultExtension: defaultExtension,
		decoderFunc:      decoderFunc,
	}
	for _, opt := range options {
		opt(ret)
	}
	return ret
}
