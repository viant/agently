package internalread

import (
	"context"
	"embed"
	"github.com/viant/datly"
	"github.com/viant/datly/repository"
	"github.com/viant/datly/repository/contract"
	"github.com/viant/datly/view"
	"reflect"
)

//go:embed sql/*.sql
var FS embed.FS

var PathURI = "/v1/api/internal/agently/user-oauth-token/get"

type Input struct {
	UserID   string `parameter:"userId,kind=query" predicate:"equal,group=0,t,user_id"`
	Provider string `parameter:"provider,kind=query" predicate:"equal,group=0,t,provider"`
}

type Row struct {
	EncToken  string  `sqlx:"enc_token" json:"encToken"`
	UpdatedAt *string `sqlx:"updated_at" json:"updatedAt,omitempty"`
}

type Output struct {
	Data []*Row `parameter:",kind=output,in=view" view:"Token" sql:"uri=sql/get.sql"`
}

func (i *Input) EmbedFS() *embed.FS { return &FS }

func DefineComponent(ctx context.Context, srv *datly.Service) error {
	c, err := repository.NewComponent(
		contract.NewPath("GET", PathURI),
		repository.WithResource(srv.Resource()),
		repository.WithContract(
			reflect.TypeOf(Input{}),
			reflect.TypeOf(Output{}), &FS, view.WithConnectorRef("agently")))
	if err != nil {
		return err
	}
	return srv.AddComponent(ctx, c)
}
