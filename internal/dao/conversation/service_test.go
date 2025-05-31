package conversation

import (
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/viant/datly/view"
	"os"
	"path"
	"testing"
)

func TestService(t *testing.T) {

	goPath := os.Getenv("GOPATH")
	dbLocation := path.Join(goPath, "src/github.com/viant/agently/.db/agently.db")
	connector := view.NewConnector("agently", "sqlite", dbLocation)
	srv, err := New(context.Background(), connector)
	if !assert.Nil(t, err) {
		t.Error(err)
		return
	}
	assert.NotNil(t, srv)
	//
	//err = srv.AddMessage(context.Background(), "1", memory.Message{
	//	Role:    "abc",
	//	Content: "hello",
	//})
	if !assert.Nil(t, err) {
		t.Error("expected no error when adding message")
		return
	}

	messages, err := srv.GetMessages(context.Background(), "1")
	if !assert.Nil(t, err) {
		t.Error("expected no error when reading message")
		return
	}
	assert.NotNil(t, messages)
}
