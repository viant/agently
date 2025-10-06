package useroauthtoken

import (
	"context"
	"embed"
	"reflect"

	"github.com/viant/datly"
	"github.com/viant/datly/repository"
	"github.com/viant/datly/repository/contract"
	"github.com/viant/datly/view"
)

//go:embed sql/*.sql
var FS embed.FS

var PathListURI = "/v1/api/internal/user-oauth-token"

type Input struct {
	UserID   string `parameter:"userId,kind=query" predicate:"equal,group=0,t,user_id"`
	Provider string `parameter:"provider,kind=query" predicate:"equal,group=0,t,provider"`
}

type TokenView struct {
	UserID    string  `sqlx:"user_id" json:"userId"`
	Provider  string  `sqlx:"provider" json:"provider"`
	UpdatedAt *string `sqlx:"updated_at" json:"updatedAt,omitempty"`
}

type Output struct {
	Data []*TokenView `parameter:",kind=output,in=view" view:"Token" sql:"uri=sql/list.sql"`
}

func (i *Input) EmbedFS() *embed.FS { return &FS }

func DefineComponent(ctx context.Context, srv *datly.Service) error {
	c, err := repository.NewComponent(
		contract.NewPath("GET", PathListURI),
		repository.WithResource(srv.Resource()),
		repository.WithContract(
			reflect.TypeOf(Input{}),
			reflect.TypeOf(Output{}), &FS, view.WithConnectorRef("agently")))
	if err != nil {
		return err
	}
	return srv.AddComponent(ctx, c)
}
