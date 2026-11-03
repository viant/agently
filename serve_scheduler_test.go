package agently

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	schedulemodel "github.com/viant/agently-core/model/schedule"
	"github.com/viant/agently-core/sdk"
	"github.com/viant/agently-core/service/scheduler"
)

type interactiveScheduleStore struct{ scheduler.Store }

func (interactiveScheduleStore) List(context.Context) ([]*schedulemodel.ScheduleView, error) {
	return []*schedulemodel.ScheduleView{}, nil
}

func TestInteractiveHostKeepsScheduledTasksAPIWithoutExecutingTasks(t *testing.T) {
	t.Setenv("AGENTLY_SCHEDULER_API", "true")
	t.Setenv("AGENTLY_SCHEDULER_RUN_NOW", "true")
	t.Setenv("AGENTLY_SCHEDULER_RUNNER", "true")
	options := schedulerOptionsForServe(true)
	service := scheduler.New(interactiveScheduleStore{}, nil)
	handler, err := sdk.NewHandlerWithContext(t.Context(), nil, sdk.WithScheduler(service, scheduler.NewHandler(service), options))
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/api/agently/scheduler/?page=1&size=10", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("scheduled tasks list is unavailable: %d %s", response.Code, response.Body.String())
	}
	var result struct {
		Data struct {
			Schedules []json.RawMessage `json:"schedules"`
		} `json:"data"`
	}
	if err = json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Data.Schedules == nil {
		t.Fatal("expected an empty schedules collection, not a missing route")
	}
	if options.EnableRunNow || options.EnableWatchdog {
		t.Fatal("interactive validation must not execute scheduled work")
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/api/agently/scheduler/run-now/synthetic", nil))
	// The GET collection subtree remains mounted, so ServeMux rejects POST
	// with 405 when its run-now route has not been registered.
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("run-now unexpectedly enabled: %d", response.Code)
	}
}

func TestInteractiveHostPreservesExplicitSchedulerAPIDisable(t *testing.T) {
	t.Setenv("AGENTLY_SCHEDULER_API", "false")
	if schedulerOptionsForServe(true).EnableAPI {
		t.Fatal("explicit API disable was ignored")
	}
}
