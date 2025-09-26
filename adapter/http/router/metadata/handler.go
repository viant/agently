package metadata

import (
	"encoding/json"
	"errors"
	"net/http"

	execsvc "github.com/viant/agently/genai/executor"
)

// New returns an http.HandlerFunc that writes the executor's default agent
// and model configuration in JSON form:
//
//	{
//	    "agent": "chat",
//	    "model": "o4-mini"
//	}
func New(exec *execsvc.Service) http.HandlerFunc {
	type response struct {
		Agent string `json:"agent"`
		Model string `json:"model"`
	}

	return func(w http.ResponseWriter, _ *http.Request) {
		cfg := exec.Config()
		if cfg == nil {
			http.Error(w, ErrNilConfig.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Status string   `json:"status"`
			Data   response `json:"data"`
		}{
			Status: "ok",
			Data: response{
				Agent: cfg.Default.Agent,
				Model: cfg.Default.Model,
			},
		})
	}
}

// Shared errors
var (
	ErrNilConfig = errors.New("executor config is nil")
)
