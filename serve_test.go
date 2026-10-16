package agently

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/viant/agently-core/protocol/primitive/observe"
)

func TestForgeWindowMetadataRequiresAuth(t *testing.T) {
	for _, testCase := range []struct {
		path string
		want bool
	}{
		{path: "/navigation", want: false},
		{path: "/window/agent", want: true},
		{path: "/window/order", want: true},
		{path: "/window/order?applyPermission=false", want: true},
	} {
		req, err := http.NewRequest(http.MethodGet, testCase.path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := forgeWindowMetadataRequiresAuth(req); got != testCase.want {
			t.Fatalf("forgeWindowMetadataRequiresAuth(%q) = %v, want %v", testCase.path, got, testCase.want)
		}
	}
}

func TestPrimitiveAPITimingKeepsStreamingWriterAndOmitsResourceIDs(t *testing.T) {
	type sample struct {
		id     uint64
		method string
		route  string
		totals map[string]float64
	}
	var samples []sample
	handler := withPrimitiveAPITiming(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !observe.Enabled(r.Context()) {
			t.Fatal("request observer missing")
		}
		if _, ok := w.(http.Flusher); !ok {
			t.Fatal("streaming writer was replaced")
		}
		span := observe.Start(r.Context(), observe.GatewayMCPCall, observe.Labels{})
		span.End(observe.OK, observe.CacheUnknown)
		w.WriteHeader(http.StatusOK)
	}), func(id uint64, method, route string, totals map[string]float64) {
		samples = append(samples, sample{id: id, method: method, route: route, totals: totals})
	})
	for _, path := range []string{
		"/v1/api/datasources/platform/private-order-2695999/fetch?token=sensitive",
		"/v1/api/reports/private-report-2695999/run",
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d", recorder.Code)
		}
	}
	if len(samples) != 2 || samples[0].id != 1 || samples[1].id != 2 {
		t.Fatalf("request sequence = %+v", samples)
	}
	if samples[0].method != http.MethodPost || samples[0].route != "datasource" || samples[1].route != "report" {
		t.Fatalf("route classifications = %+v", samples)
	}
	for _, item := range samples {
		if item.totals["http.total.calls"] != 1 || item.totals["gateway.mcp.call.calls"] != 1 {
			t.Fatalf("missing correlated timings: %+v", item.totals)
		}
	}
}

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

func TestPrimitiveAPIRouteClassSeparatesHistoryFromLongLivedStreams(t *testing.T) {
	for path, expected := range map[string]string{
		"/v1/conversations/private-id":            "conversation",
		"/v1/conversations/private-id/transcript": "conversation.transcript",
		"/v1/conversations/private-id/live-state": "conversation.live-state",
		"/v1/application-events":                  "application.events",
		"/v1/ui/rpc":                              "ui.rpc",
		"/v1/ag-ui/run":                           "agui.run",
		"/v1/feeds/private-id/data":               "feed",
	} {
		if actual := primitiveAPIRouteClass(path); actual != expected {
			t.Errorf("route class = %q, want %q", actual, expected)
		}
	}
}
