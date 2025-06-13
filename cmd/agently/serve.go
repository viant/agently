package agently

import (
	"context"
	"log"

	httpchat "github.com/viant/agently/adapter/http"
)

// ServeCmd starts the lightweight HTTP chat server that exposes /v1/api
// endpoints.  The underlying logic relies on the shared service layer via the
// conversation manager already initialised in the executor singleton.
type ServeCmd struct {
	Addr string `short:"a" long:"addr" description:"listen address" default:":8080"`
}

func (s *ServeCmd) Execute(_ []string) error {
	exec := executorSingleton()
	if !exec.IsStarted() {
		exec.Start(context.Background())
	}

	mgr := exec.Conversation()
	log.Printf("HTTP chat server listening on %s", s.Addr)
	return httpchat.ListenAndServe(s.Addr, mgr)
}
