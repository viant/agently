package agently

import (
	"context"
	"fmt"
	"github.com/viant/mcp-protocol/schema"

	"github.com/viant/agently-core/app/executor"
	identity "github.com/viant/agently-core/protocol/resource"
	"github.com/viant/agently-core/runtime/requestctx"
	"github.com/viant/agently-core/sdk"
	resources "github.com/viant/agently-core/service/resource"
)

func configureHostedAppReader(bindings []resources.HostedAppBinding, prepared *executor.PreparedAuthorization) func(*executor.Runtime, sdk.Backend) (sdk.MCPUIResourceReader, sdk.MCPUIResourceContextBinder, error) {
	return func(rt *executor.Runtime, backend sdk.Backend) (sdk.MCPUIResourceReader, sdk.MCPUIResourceContextBinder, error) {
		if len(bindings) == 0 {
			return nil, nil, nil
		}
		if prepared == nil || rt == nil || rt.PrimitiveWindows == nil {
			return nil, nil, fmt.Errorf("hosted apps require configured primitive windows")
		}
		actor := resources.ActorResolver(prepared.ResourceActor)
		reader, err := resources.NewHostedAppReader(bindings, rt.PrimitiveProviders, rt.MCPManager, rt.PrimitiveWindows, actor, verifyActor(actor), func(ctx context.Context, pin identity.ResolvedResource) error {
			return prepared.AuthorizeResolvedWindow(requestctx.WithResolvedResource(ctx, pin), pin, "ui.window.open", nil)
		})
		if err != nil {
			return nil, nil, err
		}
		fallback, _ := backend.(interface {
			ReadMCPUIResource(context.Context, string) (*schema.ReadResourceResult, error)
		})
		return func(ctx context.Context, uri string) (*schema.ReadResourceResult, error) {
				if reader.Handles(uri) {
					return reader.Read(ctx, uri)
				}
				if fallback == nil {
					return nil, identity.ErrResourceDenied
				}
				return fallback.ReadMCPUIResource(ctx, uri)
			}, func(ctx context.Context, in *sdk.MCPUIResourceReadInput) (context.Context, error) {
				return reader.BindContext(ctx, in.URI, in.Resource, in.ResourceTarget)
			}, nil
	}
}
