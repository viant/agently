package user

import (
	"context"
	"embed"
	"fmt"
	"github.com/viant/datly"
	"github.com/viant/datly/repository"
	"github.com/viant/datly/repository/contract"
	"github.com/viant/datly/view"
	"github.com/viant/xdatly/handler/response"
	"github.com/viant/xdatly/types/core"
	"github.com/viant/xdatly/types/custom/dependency/checksum"
	"reflect"
	"time"
)

func init() {
	core.RegisterType("user", "Input", reflect.TypeOf(Input{}), checksum.GeneratedTime)
	core.RegisterType("user", "Output", reflect.TypeOf(Output{}), checksum.GeneratedTime)
}

//go:embed user/*.sql
var FS embed.FS

type Input struct {
	Id       string    `parameter:",kind=path,in=id" predicate:"equal,group=0,t,id"`
	Username string    `parameter:",kind=query,in=username" predicate:"equal,group=0,t,username"`
	Provider string    `parameter:",kind=query,in=provider" predicate:"equal,group=0,t,provider"`
	Subject  string    `parameter:",kind=query,in=subject" predicate:"equal,group=0,t,subject"`
	Has      *InputHas `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type InputHas struct{ Id, Username, Provider, Subject bool }

type Output struct {
	response.Status `parameter:",kind=output,in=status" json:",omitempty"`
	Data            []*View          `parameter:",kind=output,in=view" view:"user,batch=10000,relationalConcurrency=1" sql:"uri=user/user.sql"`
	Metrics         response.Metrics `parameter:",kind=output,in=metrics"`
}

type View struct {
	Id                 string     `sqlx:"id"`
	Username           string     `sqlx:"username"`
	DisplayName        *string    `sqlx:"display_name"`
	Email              *string    `sqlx:"email"`
	Provider           string     `sqlx:"provider"`
	Subject            *string    `sqlx:"subject"`
	Timezone           string     `sqlx:"timezone"`
	DefaultAgentRef    *string    `sqlx:"default_agent_ref"`
	DefaultModelRef    *string    `sqlx:"default_model_ref"`
	DefaultEmbedderRef *string    `sqlx:"default_embedder_ref"`
	Settings           *string    `sqlx:"settings"`
	Disabled           int        `sqlx:"disabled"`
	CreatedAt          time.Time  `sqlx:"created_at"`
	UpdatedAt          *time.Time `sqlx:"updated_at"`
}

var PathURI = "/v1/api/agently/user/{id}"

func DefineComponent(ctx context.Context, srv *datly.Service) error {
	c, err := repository.NewComponent(
		contract.NewPath("GET", PathURI),
		repository.WithResource(srv.Resource()),
		repository.WithContract(
			reflect.TypeOf(Input{}),
			reflect.TypeOf(Output{}), &FS, view.WithConnectorRef("agently")))
	if err != nil {
		return fmt.Errorf("failed to create User component: %w", err)
	}
	if err := srv.AddComponent(ctx, c); err != nil {
		return fmt.Errorf("failed to add User component: %w", err)
	}
	return nil
}

func (i *Input) EmbedFS() *embed.FS { return &FS }
