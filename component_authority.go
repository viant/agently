package agently

import (
	"context"
	"fmt"

	"github.com/viant/agently-core/app/executor"
	"github.com/viant/agently-core/service/policy"
	"github.com/viant/authz/gating"
	forgeservice "github.com/viant/agently-core/service/primitiveprovider"
)

// Component execution independently refreshes authority. Explicit metadata and
// decision scopes may batch pure checks, but cannot supply execution freshness.
func freshComponentAuthority(resolve policy.AuthoritySnapshotResolver, metadata forgeservice.MetadataReadScope, decision executor.AuthorizationDecisionScope) policy.AuthoritySnapshotResolver {
	return func(ctx context.Context) (gating.Principal, error) {
		if resolve == nil || ctx == nil {
			return gating.Principal{}, fmt.Errorf("component authority unavailable")
		}
		ctx = freshExecutionContext(ctx, metadata, decision)
		return resolve(ctx)
	}
}
func freshExecutionContext(ctx context.Context, metadata forgeservice.MetadataReadScope, decision executor.AuthorizationDecisionScope) context.Context {
	if metadata != nil {
		ctx = metadata.WithoutMetadataRead(ctx)
	}
	if decision != nil {
		ctx = decision.WithoutDecision(ctx)
	}
	return ctx
}
