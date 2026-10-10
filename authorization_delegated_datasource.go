package agently

import (
	"context"

	"github.com/viant/agently-core/app/executor"
	dsproto "github.com/viant/agently-core/protocol/datasource"
	identity "github.com/viant/agently-core/protocol/resource"
)

func configuredRemoteDatasourceAdmission(prepared *executor.PreparedAuthorization) func(context.Context, identity.ResolvedResource, *dsproto.DataSource, map[string]interface{}) error {
	return func(ctx context.Context, pin identity.ResolvedResource, descriptor *dsproto.DataSource, inputs map[string]interface{}) error {
		if prepared == nil || descriptor == nil {
			return identity.ErrResourceDenied
		}
		return prepared.AuthorizeResolvedDatasource(ctx, pin, descriptor.ID, inputs)
	}
}
