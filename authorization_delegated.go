package agently

import (
	"context"
	"time"

	"github.com/viant/agently-core/app/executor"
	identity "github.com/viant/agently-core/protocol/resource"
	"github.com/viant/agently-core/runtime/requestctx"
	resources "github.com/viant/agently-core/service/resource"
	"github.com/viant/forge/backend/types"
)

// configuredRemoteWindowAdmission intersects provider-authorized metadata with
// the existing configured host open policy. It adds no provider-specific rules
// or transport metadata. The gateway independently validates actor and content.
func configuredRemoteWindowAdmission(prepared *executor.PreparedAuthorization) resources.WindowAdmission {
	return func(ctx context.Context, pin identity.ResolvedResource, window *types.Window) error {
		if ctx == nil || ctx.Err() != nil || prepared == nil || window == nil || !pin.ValidUntil.After(time.Now()) {
			return identity.ErrResourceDenied
		}
		ctx, cancel := context.WithDeadline(requestctx.WithResolvedResource(ctx, pin), pin.ValidUntil)
		defer cancel()
		if err := prepared.AuthorizeResolvedWindow(ctx, pin, "ui.window.open", nil); err != nil {
			return err
		}
		if ctx.Err() != nil || !pin.ValidUntil.After(time.Now()) {
			return identity.ErrResourceDenied
		}
		return nil
	}
}
