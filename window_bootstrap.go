package agently

import (
	"context"
	"fmt"
	"sync"

	"github.com/viant/agently-core/app/executor"
	identity "github.com/viant/agently-core/protocol/resource"
	"github.com/viant/agently-core/sdk/api"
	"github.com/viant/agently-core/service/ui/permittedview"
	"github.com/viant/forge/backend/types"
)

// WindowBootstrapDatasource exposes only the ready protected datasource stack.
// The caller's identity, original resource pin and target accompany every fetch.
type WindowBootstrapDatasource interface {
	FetchDatasource(context.Context, *api.FetchDatasourceInput) (*api.FetchDatasourceOutput, error)
}
type WindowOpenBootstrapFactory func(context.Context, WindowBootstrapDatasource) (permittedview.OpenBootstrap, permittedview.OpenSelectionCheck, error)

type windowBootstrapState struct {
	sync.RWMutex
	load  permittedview.OpenBootstrap
	match permittedview.OpenSelectionCheck
}

// prepareWindowBootstrap installs fail-closed proxies before runtime build and
// binds the host factory after the SDK datasource service exists, before serving.
func prepareWindowBootstrap(options ServeOptions) (ServeOptions, func(context.Context, WindowBootstrapDatasource) error) {
	if options.WindowOpenBootstrapFactory == nil {
		return options, func(context.Context, WindowBootstrapDatasource) error { return nil }
	}
	state := &windowBootstrapState{}
	factory := options.WindowOpenBootstrapFactory
	previous := options.ConfigureBuilder
	options.ConfigureBuilder = func(ctx context.Context, b *executor.Builder) error {
		if previous != nil {
			if err := previous(ctx, b); err != nil {
				return err
			}
		}
		b.WithWindowOpenBootstrap(func(ctx context.Context, p identity.ResolvedResource, w *types.Window, args map[string]any) (map[string]any, error) {
			state.RLock()
			load := state.load
			state.RUnlock()
			if load == nil {
				return nil, identity.ErrResourceDenied
			}
			return load(ctx, p, w, args)
		}, func(ctx context.Context, w *types.Window, args, row map[string]any) error {
			state.RLock()
			match := state.match
			state.RUnlock()
			if match == nil {
				return identity.ErrResourceDenied
			}
			return match(ctx, w, args, row)
		})
		return nil
	}
	bind := func(ctx context.Context, source WindowBootstrapDatasource) error {
		if source == nil {
			return fmt.Errorf("window bootstrap datasource service is unavailable")
		}
		state.RLock()
		alreadyBound := state.load != nil
		state.RUnlock()
		if alreadyBound {
			return fmt.Errorf("window bootstrap runtime is already bound")
		}
		load, match, err := factory(ctx, source)
		if err != nil {
			return err
		}
		if load == nil || match == nil {
			return fmt.Errorf("window bootstrap factory returned incomplete admission")
		}
		state.Lock()
		defer state.Unlock()
		if state.load != nil {
			return fmt.Errorf("window bootstrap runtime is already bound")
		}
		state.load, state.match = load, match
		return nil
	}
	return options, bind
}
