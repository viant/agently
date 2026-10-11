package agently

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	identity "github.com/viant/agently-core/protocol/resource"
	"github.com/viant/agently-core/runtime/requestctx"
	"github.com/viant/agently-core/sdk/api"
	forge "github.com/viant/agently-core/service/primitiveprovider"
	"github.com/viant/agently-core/service/ui/permittedview"
	"github.com/viant/forge/backend/types"
)

func (d *nativeWindowDelivery) emitSelection(ctx context.Context, window *types.Window, parameters map[string]any) (*identity.SelectionBinding, error) {
	binding, err := d.emit(ctx, window, parameters)
	if errors.Is(err, permittedview.ErrUnavailable) {
		return nil, forge.ErrWindowSelectionUnavailable
	}
	return binding, err
}

type nativeBootstrapKey struct{}
type nativeBootstrapRead struct {
	owner      *nativeWindowHost
	resource   identity.ResolvedResource
	target     string
	datasource string
	inputs     string
	parameters string
}

func nativeDigest(value map[string]any) (string, error) {
	return identity.SelectionParametersDigest(value)
}

// Only trusted original OPEN bootstrap constructs this wrapper.
// It still invokes the same protected datasource stack and managing MCP tool.
type nativeBootstrapDatasource struct {
	host   *nativeWindowHost
	source WindowBootstrapDatasource
}

func (s *nativeBootstrapDatasource) FetchDatasource(ctx context.Context, input *api.FetchDatasourceInput) (*api.FetchDatasourceOutput, error) {
	if s == nil || s.host == nil || s.source == nil || ctx == nil || ctx.Err() != nil || input == nil || input.Resource == nil || input.Target == nil || input.SelectionBinding != nil {
		return nil, identity.ErrResourceDenied
	}
	var declared *WindowBootstrapBinding
	for i := range s.host.config.Bootstrap {
		binding := &s.host.config.Bootstrap[i]
		if binding.URI == input.Resource.URI && binding.DataSource == input.ID {
			declared = binding
			break
		}
	}
	if declared == nil {
		return nil, identity.ErrResourceDenied
	}
	// Preserve all original parameters through the service's immutable handoff.
	copy := *input
	copy.WindowParameters = requestctx.WindowParametersFromContext(ctx)
	copy.Cache = &api.DatasourceCacheHints{BypassCache: true}
	raw, err := json.Marshal(copy)
	if err != nil {
		return nil, identity.ErrResourceDenied
	}
	var detached api.FetchDatasourceInput
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&detached) != nil {
		return nil, identity.ErrResourceDenied
	}
	inputs, err := nativeDigest(detached.Inputs)
	if err != nil {
		return nil, err
	}
	parameters, err := nativeDigest(detached.WindowParameters)
	if err != nil {
		return nil, err
	}
	target, err := json.Marshal(detached.Target)
	if err != nil {
		return nil, identity.ErrResourceDenied
	}
	marker := &nativeBootstrapRead{owner: s.host, resource: *detached.Resource, target: string(target), datasource: detached.ID, inputs: inputs, parameters: parameters}
	return s.source.FetchDatasource(context.WithValue(ctx, nativeBootstrapKey{}, marker), &detached)
}

func (h *nativeWindowHost) verifyDeliveryContext(ctx context.Context, pin identity.ResolvedResource, target *types.WindowTarget, id string, inputs map[string]any) error {
	if ctx == nil || ctx.Err() != nil || h == nil || h.delivery == nil || !pin.ValidUntil.After(time.Now()) {
		return identity.ErrResourceDenied
	}
	if marker, _ := ctx.Value(nativeBootstrapKey{}).(*nativeBootstrapRead); marker != nil && marker.owner == h {
		actualInputs, err := nativeDigest(inputs)
		if err != nil {
			return identity.ErrResourceDenied
		}
		actualParameters, err := nativeDigest(requestctx.WindowParametersFromContext(ctx))
		if err != nil {
			return identity.ErrResourceDenied
		}
		actualTarget, err := json.Marshal(target)
		if err != nil || marker.datasource != id || !nativeJSONEqual(marker.resource, pin) || marker.target != string(actualTarget) || marker.inputs != actualInputs || marker.parameters != actualParameters {
			return identity.ErrResourceDenied
		}
		_, _, err = h.delivery.actor(ctx)
		return err
	}
	grant, _ := ctx.Value(nativeDeliveryKey{}).(*nativeDeliveryGrant)
	if grant == nil || grant.owner != h.delivery || grant.prepared == nil || !nativeJSONEqual(pin, grant.prepared.Resource) || !nativeJSONEqual(target, grant.prepared.Target) {
		return identity.ErrResourceDenied
	}
	if _, err := (&nativeDeliveryPolicy{delivery: h.delivery}).grant(ctx); err != nil {
		return err
	}
	parameters, err := nativeDigest(requestctx.WindowParametersFromContext(ctx))
	if err != nil || parameters != grant.parameters {
		return identity.ErrResourceDenied
	}
	return nil
}
