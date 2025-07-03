package workflow

import (
	"encoding/json"
	"net/http"
	"time"

	"context"
	"github.com/google/uuid"
	execsvc "github.com/viant/agently/genai/executor"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/service"
)

// runRequest is the expected JSON payload for POST /v1/api/workflow/run.
type runRequest struct {
	Location string      `json:"location"`
	TaskID   string      `json:"taskId,omitempty"`
	Input    interface{} `json:"input,omitempty"`
	Title    string      `json:"title,omitempty"`
	Timeout  int         `json:"timeoutSeconds,omitempty"`
}

type runResponse struct {
	ConversationID string `json:"conversationId"`
}

// New returns an http.Handler that exposes a single POST /run endpoint.
func New(exec *execsvc.Service, svc *service.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var req runRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		convID := uuid.New().String()

		// Ensure conversation exists in history so UI can fetch immediately.
		if hist := exec.Conversation().History(); hist != nil {
			if hs, ok := hist.(*memory.HistoryStore); ok {
				hs.EnsureConversation(convID)
			}
		}

		// Kick off workflow in background – fire & forget.
		go func() {
			ctx := context.Background()
			// propagate conversation ID so any tasks that post messages know where to go.
			ctx = context.WithValue(ctx, memory.ConversationIDKey, convID)

			to := time.Duration(req.Timeout) * time.Second

			_, _ = svc.ExecuteWorkflow(ctx, service.WorkflowRequest{
				Location: req.Location,
				TaskID:   req.TaskID,
				Input:    req.Input,
				Timeout:  to,
			})
		}()

		resp := runResponse{ConversationID: convID}
		_ = json.NewEncoder(w).Encode(resp)
	})
}
