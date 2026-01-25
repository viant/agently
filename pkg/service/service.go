package service

import (
	"context"
	"fmt"

	"github.com/viant/xdatly/handler"
	xhttp "github.com/viant/xdatly/handler/http"
	"github.com/viant/xdatly/handler/state"
)

type Service[I any, O any] struct {
	injector    state.Injector
	route       *xhttp.Route
	getInjector func(ctx context.Context, route xhttp.Route) (state.Injector, error)
}

type Option[I any] func(i *I)

func New[I any, O any](ctx context.Context, parent handler.Session, route *xhttp.Route) (*Service[I, O], error) {
	sess, err := parent.Session(ctx, route)
	if err != nil {
		return nil, err
	}
	return &Service[I, O]{injector: sess.Stater()}, nil
}

// NewWithInjector creates a service that resolves an injector via the supplied callback at execution time.
// It is useful when the caller only has an injector-provider function (instead of a handler.Session).
func NewWithInjector[I any, O any](route *xhttp.Route, getInjector func(ctx context.Context, route xhttp.Route) (state.Injector, error)) *Service[I, O] {
	return &Service[I, O]{route: route, getInjector: getInjector}
}

func Apply[I any](input *I, options ...Option[I]) {
	for _, opt := range options {
		opt(input)
	}
}

func (s *Service[I, O]) Bind(ctx context.Context, dest interface{}, opts ...state.Option) error {
	injector, err := s.ensureInjector(ctx)
	if err != nil {
		return err
	}
	return injector.Bind(ctx, dest, opts...)
}

func (s *Service[I, O]) RunWithSelector(ctx context.Context, input *I, selector *state.NamedQuerySelector, opts ...state.Option) (*O, error) {
	return s.Run(ctx, input, append(opts, state.WithQuerySelector(selector))...)
}

func (s *Service[I, O]) Run(ctx context.Context, input *I, opts ...state.Option) (*O, error) {
	return s.Execute(ctx, append(opts, state.WithInput(input))...)
}

func (s *Service[I, O]) Execute(ctx context.Context, opts ...state.Option) (*O, error) {
	var output O
	injector, err := s.ensureInjector(ctx)
	if err != nil {
		return nil, err
	}
	err = injector.Bind(ctx, &output, opts...)
	return &output, err
}

func (s *Service[I, O]) ensureInjector(ctx context.Context) (state.Injector, error) {
	injector := s.injector
	if injector == nil && s.getInjector != nil && s.route != nil {
		var err error
		injector, err = s.getInjector(ctx, *s.route)
		if err != nil {
			return nil, err
		}
	}
	if injector == nil {
		return nil, fmt.Errorf("service: injector not configured")
	}
	return injector, nil
}
