package agently

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestFinalizeServeResult_CancelsOnServeError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var shutdownWG sync.WaitGroup
	shutdownWG.Add(1)
	go func() {
		defer shutdownWG.Done()
		<-ctx.Done()
	}()

	done := make(chan error, 1)
	go func() {
		done <- finalizeServeResult(cancel, &shutdownWG, errors.New("listen tcp :8181: bind: address already in use"), &http.Server{})
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("expected wrapped serve error, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("finalizeServeResult hung waiting for shutdown")
	}
}
