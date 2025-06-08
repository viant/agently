package agently

import (
	"context"
	"encoding/json"
	"github.com/viant/agently/genai/extension/fluxor/llm/agent"
	"github.com/viant/agently/genai/tool"
	"log"
	"net/http"
	"time"
)

// ServeCmd starts REST server.
type ServeCmd struct {
	Addr   string `short:"a" long:"addr" description:"listen address" default:":8080"`
	Policy string `long:"policy" description:"tool policy: auto|ask|deny" default:"auto"`
}

func (s *ServeCmd) Execute(_ []string) error {
	exec := executorSingleton()
	ctx := context.Background()
	fluxPol := buildFluxorPolicy(s.Policy)
	toolPol := &tool.Policy{Mode: fluxPol.Mode, Ask: stdinAsk}

	mux := http.NewServeMux()

	mux.HandleFunc("/chat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			ConversationID string `json:"conversationId"`
			Location       string `json:"location"`
			Query          string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		in := &agent.QueryInput{ConversationID: req.ConversationID, Location: req.Location, Query: req.Query}
		innerCtx := tool.WithPolicy(ctx, toolPol)
		innerCtx = withFluxorPolicy(innerCtx, fluxPol)
		out, err := exec.Conversation().Accept(innerCtx, in)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(out)
	})

	mux.HandleFunc("/conversations", func(w http.ResponseWriter, r *http.Request) {
		ids, err := exec.Conversation().List(ctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(ids)
	})

	srv := &http.Server{Addr: s.Addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("listening on %s", s.Addr)
	return srv.ListenAndServe()
}
