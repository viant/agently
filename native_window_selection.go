package agently

import (
	"context"
	"errors"
	"time"

	identity "github.com/viant/agently-core/protocol/resource"
	"github.com/viant/agently-core/runtime/requestctx"
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

func nativeDigest(value map[string]any) (string, error) {
	return identity.SelectionParametersDigest(value)
}

func (h *nativeWindowHost) verifyDeliveryContext(ctx context.Context, pin identity.ResolvedResource, target *types.WindowTarget, id string, inputs map[string]any) error {
	if ctx == nil || ctx.Err() != nil || h == nil || h.delivery == nil || !pin.ValidUntil.After(time.Now()) {
		return identity.ErrResourceDenied
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
