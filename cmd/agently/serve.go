package agently

import (
	"context"
	"log"
	"net/http"

	"github.com/viant/agently/adapter/http/router"
	"github.com/viant/agently/service"
)

// ServeCmd starts the embedded HTTP server.
// Usage: agently serve --addr :8080
type ServeCmd struct {
	Addr string `short:"a" long:"addr" description:"listen address" default:":8080"`
}

func (s *ServeCmd) Execute(_ []string) error {
	exec := executorSingleton()
	if !exec.IsStarted() {
		exec.Start(context.Background())
	}

	svc := service.New(exec, service.Options{})

	handler := router.New(exec, svc)

	log.Printf("Agently HTTP server listening on %s", s.Addr)
	return http.ListenAndServe(s.Addr, handler)
}
