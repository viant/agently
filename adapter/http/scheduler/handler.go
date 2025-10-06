package scheduler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	schapi "github.com/viant/agently/client/scheduler"
	schstorecli "github.com/viant/agently/client/scheduler/store"
)

type handler struct{ store schstorecli.Client }

// New constructs a handler exposing:
//
//	GET    /v1/api/agently/scheduler/schedule/           -> list schedules
//	GET    /v1/api/agently/scheduler/schedule/{id}       -> get schedule
//	PATCH  /v1/api/agently/schedule                      -> upsert schedule(s)
//	GET    /v1/api/agently/scheduler/run/{id}            -> list runs for schedule id
//	PATCH  /v1/api/agently/schedule-run                  -> upsert run(s)
func New() (http.Handler, error) { return nil, fmt.Errorf("scheduler: use NewWithClient") }

// NewWithDatly constructs the handler using a shared datly.Service instance.
// Preferred constructor: inject a scheduler store client
func NewWithClient(store schstorecli.Client) (http.Handler, error) {
	return &handler{store: store}, nil
}

type apiResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, data interface{}, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		if status == 0 {
			status = http.StatusInternalServerError
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(apiResponse{Status: "ERROR", Message: err.Error()})
		return
	}
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiResponse{Status: "ok", Data: data})
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.store == nil {
		writeJSON(w, http.StatusInternalServerError, nil, fmt.Errorf("scheduler store not initialized"))
		return
	}

	// Normalize path without trailing slashes multiplicity
	path := r.URL.Path
	// Routes are mounted under /v1/api/agently/… – match suffixes
	// Examples handled here:
	//  - /v1/api/agently/scheduler/schedule/
	//  - /v1/api/agently/scheduler/schedule/{id}
	//  - /v1/api/agently/scheduler/run/{id}
	//  - /v1/api/agently/schedule
	//  - /v1/api/agently/schedule-run

	// Schedules list or item
	if strings.HasPrefix(path, "/v1/api/agently/scheduler/schedule/") {
		switch r.Method {
		case http.MethodGet:
			rest := strings.TrimPrefix(path, "/v1/api/agently/scheduler/schedule/")
			if rest == "" { // list
				items, err := h.store.GetSchedules(r.Context())
				if err != nil {
					writeJSON(w, 0, nil, err)
					return
				}
				// Return raw view models; default JSON marshaler applies field names with json tags (if any)
				writeJSON(w, 0, items, nil)
				return
			}
			// get by id
			id := strings.Trim(rest, "/")
			item, err := h.store.GetSchedule(r.Context(), id)
			if item == nil && err == nil {
				writeJSON(w, http.StatusNotFound, nil, fmt.Errorf("not found"))
				return
			}
			if err != nil {
				writeJSON(w, 0, nil, err)
				return
			}
			writeJSON(w, 0, item, nil)
			return
		default:
			writeJSON(w, http.StatusMethodNotAllowed, nil, fmt.Errorf("method not allowed"))
			return
		}
	}

	// Runs list for schedule id
	if strings.HasPrefix(path, "/v1/api/agently/scheduler/run/") {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, nil, fmt.Errorf("method not allowed"))
			return
		}
		id := strings.TrimPrefix(path, "/v1/api/agently/scheduler/run/")
		since := strings.TrimSpace(r.URL.Query().Get("since"))
		items, err := h.store.GetRuns(r.Context(), strings.Trim(id, "/"), since)
		if err != nil {
			writeJSON(w, 0, nil, err)
			return
		}
		writeJSON(w, 0, items, nil)
		return
	}

	// Schedule write (PATCH)
	if path == "/v1/api/agently/schedule" {
		if r.Method != http.MethodPatch && r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, nil, fmt.Errorf("method not allowed"))
			return
		}
		var req struct {
			Data      []schapi.MutableSchedule `json:"data"`
			Schedules []schapi.MutableSchedule `json:"Schedules"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, nil, err)
			return
		}
		items := req.Data
		if len(items) == 0 && len(req.Schedules) > 0 {
			items = req.Schedules
		}
		if len(items) == 0 {
			writeJSON(w, http.StatusBadRequest, nil, fmt.Errorf("Schedules payload required"))
			return
		}
		for i := range items {
			if err := h.store.PatchSchedule(r.Context(), &items[i]); err != nil {
				writeJSON(w, 0, nil, err)
				return
			}
		}
		writeJSON(w, 0, "ok", nil)
		return
	}

	// Run write (PATCH)
	if path == "/v1/api/agently/schedule-run" {
		if r.Method != http.MethodPatch && r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, nil, fmt.Errorf("method not allowed"))
			return
		}
		var req struct {
			Data []schapi.MutableRun `json:"data"`
			Runs []schapi.MutableRun `json:"Runs"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, nil, err)
			return
		}
		items := req.Data
		if len(items) == 0 && len(req.Runs) > 0 {
			items = req.Runs
		}
		if len(items) == 0 {
			writeJSON(w, http.StatusBadRequest, nil, fmt.Errorf("Runs payload required"))
			return
		}
		for i := range items {
			if err := h.store.PatchRun(r.Context(), &items[i]); err != nil {
				writeJSON(w, 0, nil, err)
				return
			}
		}
		writeJSON(w, 0, "ok", nil)
		return
	}

	writeJSON(w, http.StatusNotFound, nil, fmt.Errorf("not found"))
}

// mapping helpers removed – default JSON marshaling is used
