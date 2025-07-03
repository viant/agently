package metadata

import (
	"encoding/json"
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
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(response{
			Agent: cfg.Default.Agent,
			Model: cfg.Default.Model,
		})
	}
}
