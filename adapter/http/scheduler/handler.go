package scheduler

import (
	"context"
	"fmt"
	"net/http"

	schapi "github.com/viant/agently/client/scheduler"
	schstorecli "github.com/viant/agently/client/scheduler/store"
	"github.com/viant/datly"
)

type handler struct {
	dao     *datly.Service
	router  *datly.Router[Service]
	service Service
}

// NewHandler constructs the handler with read/write store, datly service, and
// an orchestration scheduler client used for on-demand run triggers.
func NewHandler(dao *datly.Service, store schstorecli.Client, sched schapi.Client) (http.Handler, error) {
	return newWith(dao, Service{store: store, scheduler: sched})
}

func newWith(dao *datly.Service, svc Service) (http.Handler, error) {
	if dao == nil || svc.store == nil {
		return nil, fmt.Errorf("scheduler http: missing dao or store")
	}
	r := datly.NewRouter[Service](dao, svc)
	if err := registerRoutes(context.Background(), dao, r); err != nil {
		return nil, err
	}
	return &handler{dao: dao, router: r, service: svc}, nil
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Datly expects RFC3339 timestamps (with seconds). Many UI datetime pickers
	// emit ISO-8601 without seconds (e.g. 2026-01-01T00:00+01:00), which fails
	// Datly's time parser. Normalize scheduler write payloads to include seconds.
	if err := normalizeSchedulerWriteTimeFields(r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Dependencies are enforced at construction (newWith). Delegate to Datly router, which writes responses.
	if err := h.router.Run(w, r); err != nil {
		if datly.IsRouteNotFound(err) {
			http.NotFound(w, r)
		}
		return
	}
}
