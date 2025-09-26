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
	TenantID string     `parameter:",kind=path,in=tenantId" predicate:"in,group=0,p,tenant_id"`
	Id       string     `parameter:",kind=query,in=id" predicate:"in,group=0,p,id"`
	Ids      []string   `parameter:",kind=query,in=ids" predicate:"in,group=0,p,id"`
	Kind     string     `parameter:",kind=query,in=kind" predicate:"in,group=0,p,kind"`
	Digest   string     `parameter:",kind=query,in=digest" predicate:"in,group=0,p,digest"`
	Storage  string     `parameter:",kind=query,in=storage" predicate:"in,group=0,p,storage"`
	MimeType string     `parameter:",kind=query,in=mime_type" predicate:"in,group=0,p,mime_type"`
	Since    *time.Time `parameter:",kind=query,in=since" predicate:"greater_or_equal,group=0,p,created_at"`
	Has      *Has       `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type Has struct {
	TenantID bool
	Id       bool
	Ids      bool
	Kind     bool
	Digest   bool
	Storage  bool
	MimeType bool
	Since    bool
}

type Output struct {
	response.Status `parameter:",kind=output,in=status" json:",omitempty"`
	Data            []*PayloadView   `parameter:",kind=output,in=view" view:"payload,batch=10000,relationalConcurrency=1" sql:"uri=sql/payload.sql"`
	Metrics         response.Metrics `parameter:",kind=output,in=metrics"`
}

type PayloadView struct {
	Id                     string     `sqlx:"id"`
	TenantID               *string    `sqlx:"tenant_id"`
	Kind                   string     `sqlx:"kind"`
	Subtype                *string    `sqlx:"subtype"`
	MimeType               string     `sqlx:"mime_type"`
	SizeBytes              int        `sqlx:"size_bytes"`
	Digest                 *string    `sqlx:"digest"`
	Storage                string     `sqlx:"storage"`
	InlineBody             *[]byte    `sqlx:"inline_body"`
	URI                    *string    `sqlx:"uri"`
	Compression            string     `sqlx:"compression"`
	EncryptionKMSKeyID     *string    `sqlx:"encryption_kms_key_id"`
	RedactionPolicyVersion *string    `sqlx:"redaction_policy_version"`
	Redacted               *int       `sqlx:"redacted"`
	CreatedAt              *time.Time `sqlx:"created_at"`
	SchemaRef              *string    `sqlx:"schema_ref"`
	Preview                *string    `sqlx:"preview"`
	Tags                   *string    `sqlx:"tags"`
}

var PayloadURI = "/v2/api/agently/payload"

func DefineComponent(ctx context.Context, srv *datly.Service) error {
	base, err := repository.NewComponent(
		contract.NewPath("GET", PayloadURI),
		repository.WithResource(srv.Resource()),
		repository.WithContract(reflect.TypeOf(Input{}), reflect.TypeOf(Output{}), &FS, view.WithConnectorRef("agently")),
	)
	if err != nil {
		return fmt.Errorf("failed to create payload base component: %w", err)
	}
	if err := srv.AddComponent(ctx, base); err != nil {
		return fmt.Errorf("failed to add payload base: %w", err)
	}
	return nil
}

func (i *Input) EmbedFS() *embed.FS { return &FS }
