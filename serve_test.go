package agently

import (
	"context"
	"errors"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"
)

func TestApplyScratchpadRootURI(t *testing.T) {
	const envName = "AGENTLY_SCRATCHPAD_URI"

	t.Run("flag overrides environment", func(t *testing.T) {
		t.Setenv(envName, "mem://localhost/from-env/${userID}")

		applyScratchpadRootURI("  gs://scratchpad-bucket/from-flag/${userID}  ")

		if got, want := os.Getenv(envName), "gs://scratchpad-bucket/from-flag/${userID}"; got != want {
			t.Fatalf("AGENTLY_SCRATCHPAD_URI = %q, want %q", got, want)
		}
	})

	t.Run("omitted flag preserves environment", func(t *testing.T) {
		t.Setenv(envName, "mem://localhost/from-env/${userID}")

		applyScratchpadRootURI("")

		if got, want := os.Getenv(envName), "mem://localhost/from-env/${userID}"; got != want {
			t.Fatalf("AGENTLY_SCRATCHPAD_URI = %q, want %q", got, want)
		}
	})

	t.Run("whitespace-only flag preserves environment", func(t *testing.T) {
		t.Setenv(envName, "mem://localhost/from-env/${userID}")

		applyScratchpadRootURI("   ")

		if got, want := os.Getenv(envName), "mem://localhost/from-env/${userID}"; got != want {
			t.Fatalf("AGENTLY_SCRATCHPAD_URI = %q, want %q", got, want)
		}
	})
}

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
