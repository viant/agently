package read

import (
	"context"
	"embed"
	"fmt"
	"reflect"
	"time"

	"github.com/viant/datly"
	"github.com/viant/datly/repository"
	"github.com/viant/datly/repository/contract"
	"github.com/viant/datly/view"
	"github.com/viant/xdatly/handler/response"
)

//go:embed sql/*.sql
var FS embed.FS

type Input struct {
	ConversationID string     `parameter:",kind=query,in=conversationId" predicate:"equal,group=0,gf,conversation_id"`
	TurnID         string     `parameter:",kind=query,in=turnId" predicate:"equal,group=0,gf,turn_id"`
	MessageID      string     `parameter:",kind=query,in=messageId" predicate:"equal,group=0,gf,message_id"`
	ID             string     `parameter:",kind=query,in=id" predicate:"equal,group=0,gf,id"`
	Provider       string     `parameter:",kind=query,in=provider" predicate:"equal,group=0,gf,provider"`
	Status         string     `parameter:",kind=query,in=status" predicate:"equal,group=0,gf,status"`
	Since          *time.Time `parameter:",kind=query,in=since" predicate:"greater_or_equal,group=0,gf,created_at"`
	Has            *Has       `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type Has struct {
	ConversationID bool
	TurnID         bool
	MessageID      bool
	ID             bool
	Provider       bool
	Status         bool
	Since          bool
}

type Output struct {
	response.Status `parameter:",kind=output,in=status" json:",omitempty"`
	Data            []*GeneratedFileView `parameter:",kind=output,in=view" view:"generated_file,batch=10000,relationalConcurrency=1" sql:"uri=sql/generated_file.sql"`
	Metrics         response.Metrics     `parameter:",kind=output,in=metrics"`
}

type GeneratedFileView struct {
	ID             string     `sqlx:"id"`
	ConversationID string     `sqlx:"conversation_id"`
	TurnID         *string    `sqlx:"turn_id"`
	MessageID      *string    `sqlx:"message_id"`
	Provider       string     `sqlx:"provider"`
	Mode           string     `sqlx:"mode"`
	CopyMode       string     `sqlx:"copy_mode"`
	Status         string     `sqlx:"status"`
	PayloadID      *string    `sqlx:"payload_id"`
	ContainerID    *string    `sqlx:"container_id"`
	ProviderFileID *string    `sqlx:"provider_file_id"`
	Filename       *string    `sqlx:"filename"`
	MimeType       *string    `sqlx:"mime_type"`
	SizeBytes      *int       `sqlx:"size_bytes"`
	Checksum       *string    `sqlx:"checksum"`
	ErrorMessage   *string    `sqlx:"error_message"`
	ExpiresAt      *time.Time `sqlx:"expires_at"`
	CreatedAt      *time.Time `sqlx:"created_at"`
	UpdatedAt      *time.Time `sqlx:"updated_at"`
}

var URI = "/v2/api/agently/generated-file"

func DefineComponent(ctx context.Context, srv *datly.Service) error {
	base, err := repository.NewComponent(
		contract.NewPath("GET", URI),
		repository.WithResource(srv.Resource()),
		repository.WithContract(reflect.TypeOf(Input{}), reflect.TypeOf(Output{}), &FS, view.WithConnectorRef("agently")),
	)
	if err != nil {
		return fmt.Errorf("failed to create generated file base component: %w", err)
	}
	if err := srv.AddComponent(ctx, base); err != nil {
		return fmt.Errorf("failed to add generated file base: %w", err)
	}
	return nil
}

func (i *Input) EmbedFS() *embed.FS { return &FS }
