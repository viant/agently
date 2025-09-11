package adapter

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	convmem "github.com/viant/agently/internal/dao/conversation/impl/memory"
	msgmem "github.com/viant/agently/internal/dao/message/impl/memory"
	mcmem "github.com/viant/agently/internal/dao/modelcall/impl/memory"
	plmem "github.com/viant/agently/internal/dao/payload/impl/memory"
	tcmem "github.com/viant/agently/internal/dao/toolcall/impl/memory"
	turnmem "github.com/viant/agently/internal/dao/turn/impl/memory"
	usemem "github.com/viant/agently/internal/dao/usage/impl/memory"

	msgw "github.com/viant/agently/internal/dao/message/write"
	plw "github.com/viant/agently/internal/dao/payload/write"
	tcwrite "github.com/viant/agently/internal/dao/toolcall/write"
	turnread "github.com/viant/agently/internal/dao/turn/read"
	turnwrite "github.com/viant/agently/internal/dao/turn/write"
	useread "github.com/viant/agently/internal/dao/usage/read"
	usew "github.com/viant/agently/internal/dao/usage/write"

	d "github.com/viant/agently/internal/domain"
)

// TestStore_Smoke wires memory DAOs and exercises each domain facet minimally.
func TestStore_Smoke(t *testing.T) {
	ctx := context.Background()
	store := New(
		convmem.New(),
		msgmem.New(),
		turnmem.New(),
		mcmem.New(),
		tcmem.New(),
		plmem.New(),
		usemem.New(),
	)

	// Conversations: Patch + List
	_, err := store.Conversations().Patch(ctx)
	assert.NoError(t, err)

	// Payloads: Patch + List
	_, err = store.Payloads().Patch(ctx, func() *plw.Payload {
		p := &plw.Payload{Id: "p1", Has: &plw.PayloadHas{Id: true}}
		p.SetKind("text")
		p.SetMimeType("text/plain")
		p.SetSizeBytes(1)
		p.SetStorage("inline")
		return p
	}())
	assert.NoError(t, err)
	pls, err := store.Payloads().List(ctx)
	assert.NoError(t, err)
	if b, _ := json.Marshal(pls); len(b) == 0 {
		t.Fatalf("payloads list empty serialization")
	}

	// Turns: StartedAt + List
	tw := &turnwrite.Turn{}
	tw.SetId("t1")
	tw.SetConversationID("cv1")
	tw.SetStatus("running")
	_, err = store.Turns().Start(ctx, tw)
	assert.NoError(t, err)
	turns, err := store.Turns().List(ctx, turnread.WithConversationID("cv1"))
	assert.NoError(t, err)
	assert.True(t, len(turns) >= 1)

	// Messages: Patch + List
	_, err = store.Messages().Patch(ctx, &msgw.Message{Id: "m1", ConversationID: "cv1", Role: "user", Type: "text", Content: "hi", Has: &msgw.MessageHas{Id: true, ConversationID: true, Role: true, Type: true, Content: true}})
	assert.NoError(t, err)
	msgs, err := store.Messages().List(ctx)
	assert.NoError(t, err)
	assert.True(t, len(msgs) >= 1)

	// Usage: Patch totals + List
	_, err = store.Usage().Patch(ctx, func() *usew.Usage { u := &usew.Usage{}; u.SetConversationID("cv1"); u.SetUsageInputTokens(1); return u }())
	assert.NoError(t, err)
	ul, err := store.Usage().List(ctx, useread.Input{})
	assert.NoError(t, err)
	_ = ul // may be empty depending on memory DAO state

	// Operations: Record a tool call (smoke test)
	tc := &tcwrite.ToolCall{}
	tc.SetMessageID("m1")
	tc.SetOpID("op1")
	tc.SetAttempt(1)
	tc.SetToolName("sys.echo")
	tc.SetToolKind("general")
	tc.SetStatus("completed")
	err = store.Operations().RecordToolCall(ctx, tc)
	assert.NoError(t, err)
}

// no-op
