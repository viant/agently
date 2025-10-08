package run

import (
	"context"
	"fmt"
	"reflect"

	runwrite "github.com/viant/agently/pkg/agently/scheduler/run/write"
	"github.com/viant/datly"
	"github.com/viant/datly/repository"
	"github.com/viant/datly/repository/contract"
)

// RunNowPathURI defines POST route to trigger a run on demand.
const RunNowPathURI = "/v1/api/agently/scheduler/run-now"

// No additional aliases; only body variant is supported.

// RunNowOutput is a minimal response payload for run-now route.
type RunNowOutput struct {
	RunId          string `json:"runId"`
	ConversationId string `json:"conversationId,omitempty"`
}

// DefineRunNowComponent registers a pseudo component for POST run-now, using
// runwrite.Run as input and RunNowOutput as output to enable component
// un/marshalling and Datly router integration.
func DefineRunNowComponent(ctx context.Context, dao *datly.Service) error {
	comp, err := repository.NewComponent(
		contract.NewPath("POST", RunNowPathURI),
		repository.WithResource(dao.Resource()),
		repository.WithContract(reflect.TypeOf(runwrite.Run{}), reflect.TypeOf(RunNowOutput{}), &RunFS),
	)
	if err != nil {
		return fmt.Errorf("failed to create RunNow component: %w", err)
	}
	if err := dao.AddComponent(ctx, comp); err != nil {
		return fmt.Errorf("failed to add RunNow component: %w", err)
	}
	return nil
}

// No alias-definers: keep a single canonical route.
