package agently

import (
	"context"

	svcauth "github.com/viant/agently-core/service/auth"
	resources "github.com/viant/agently-core/service/resource"
)

type copyableDecisionScope interface {
	BeginDecision(context.Context) (context.Context, func() error, error)
	WithoutDecision(context.Context) context.Context
	CopyDecision(context.Context, context.Context) (context.Context, error)
}

type idTokenReadFactsScope struct{ source copyableDecisionScope }

func (s idTokenReadFactsScope) BeginDecision(ctx context.Context) (context.Context, func() error, error) {
	return s.source.BeginDecision(svcauth.AuthzIDTokenContext(ctx))
}
func (s idTokenReadFactsScope) WithoutDecision(ctx context.Context) context.Context {
	return s.source.WithoutDecision(ctx)
}
func (s idTokenReadFactsScope) CopyDecision(ctx, scoped context.Context) (context.Context, error) {
	return s.source.CopyDecision(svcauth.AuthzIDTokenContext(ctx), scoped)
}

func configuredDatasourceReadFacts(source interface{}) resources.ReadFactsScope {
	copyable, ok := source.(copyableDecisionScope)
	if !ok {
		return nil
	}
	return idTokenReadFactsScope{source: copyable}
}
