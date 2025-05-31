package post

import (
	"github.com/viant/agently/pkg/dependency/checksum"
	"github.com/viant/xdatly/types/core"
	"reflect"
	"time"
)

var PackageName = "tool/post"

func init() {
	core.RegisterType(PackageName, "ToolCall", reflect.TypeOf(ToolCall{}), checksum.GeneratedTime)
}

type ToolCalls struct {
	ToolCall []*ToolCall
}

type ToolCall struct {
	Id             int        `sqlx:"id,primaryKey" `
	ConversationId *string    `sqlx:"conversation_id,refTable=conversation,refColumn=id" json:",omitempty"`
	ToolName       string     `sqlx:"tool_name" validate:"required"`
	Arguments      *string    `sqlx:"arguments" json:",omitempty"`
	Result         *string    `sqlx:"result" json:",omitempty"`
	Succeeded      *bool      `sqlx:"succeeded" json:",omitempty"`
	ErrorMsg       *string    `sqlx:"error_msg" json:",omitempty"`
	StartedAt      *time.Time `sqlx:"started_at" json:",omitempty"`
	FinishedAt     *time.Time `sqlx:"finished_at" json:",omitempty"`
	Has            *ToolHas   `setMarker:"true" format:"-" sqlx:"-" diff:"-" json:"-"`
}

type ToolHas struct {
	Id             bool
	ConversationId bool
	ToolName       bool
	Arguments      bool
	Result         bool
	Succeeded      bool
	ErrorMsg       bool
	StartedAt      bool
	FinishedAt     bool
}

func (t *ToolCall) SetId(value int) {
	t.Id = value
	t.Has.Id = true
}

func (t *ToolCall) SetConversationId(value string) {
	t.ConversationId = &value
	t.Has.ConversationId = true
}

func (t *ToolCall) SetToolName(value string) {
	t.ToolName = value
	t.Has.ToolName = true
}

func (t *ToolCall) SetArguments(value string) {
	t.Arguments = &value
	t.Has.Arguments = true
}

func (t *ToolCall) SetResult(value string) {
	t.Result = &value
	t.Has.Result = true
}

func (t *ToolCall) SetSucceeded(value bool) {
	t.Succeeded = &value
	t.Has.Succeeded = true
}

func (t *ToolCall) SetErrorMsg(value string) {
	t.ErrorMsg = &value
	t.Has.ErrorMsg = true
}

func (t *ToolCall) SetStartedAt(value time.Time) {
	t.StartedAt = &value
	t.Has.StartedAt = true
}

func (t *ToolCall) SetFinishedAt(value time.Time) {
	t.FinishedAt = &value
	t.Has.FinishedAt = true
}
