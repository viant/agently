package agently

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	audit "github.com/viant/agently-core/service/conversationaudit"
	"github.com/viant/agently-core/service/workspace"
)

const conversationAuditWindow = "conversation-audit"

func conversationAuditOptions() (audit.Options, error) {
	var opts audit.Options
	if raw := strings.TrimSpace(os.Getenv("AGENTLY_EXPERIMENTAL_CONVERSATION_AUDIT_UI")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return opts, fmt.Errorf("AGENTLY_EXPERIMENTAL_CONVERSATION_AUDIT_UI must be a boolean")
		}
		opts.UIEnabled = value
	}
	if raw := strings.TrimSpace(os.Getenv("AGENTLY_CONVERSATION_AUDIT_USERS")); raw != "" {
		if err := json.Unmarshal([]byte(raw), &opts.AuditorUserIDs); err != nil {
			return opts, fmt.Errorf("AGENTLY_CONVERSATION_AUDIT_USERS must be a JSON array of server user IDs")
		}
		for _, id := range opts.AuditorUserIDs {
			if strings.TrimSpace(id) == "" {
				return opts, fmt.Errorf("audit user IDs must not be empty")
			}
		}
	}
	return opts, nil
}

func conversationAuditLayout(enabled bool) func(context.Context, *workspace.Layout) error {
	return func(_ context.Context, layout *workspace.Layout) error {
		if layout.Topbar == nil {
			if !enabled {
				return nil
			}
			layout.Topbar = &workspace.LayoutTopbar{}
		}
		actions := make([]workspace.LayoutMenu, 0, len(layout.Topbar.Actions)+1)
		for _, item := range layout.Topbar.Actions {
			if item.ID == conversationAuditWindow || (item.Action != nil && item.Action.WindowKey == conversationAuditWindow) {
				continue
			}
			actions = append(actions, item)
		}
		if enabled {
			auditAction := workspace.LayoutMenu{ID: conversationAuditWindow, Title: "Conversation audit", Icon: "lucide:history", Action: &workspace.LayoutAction{Type: "window", WindowKey: conversationAuditWindow}}
			index := len(actions)
			for i, item := range actions {
				if item.Action != nil && (item.Action.WindowKey == "schedule" || item.Action.WindowKey == "schedules" || item.ID == "schedule" || item.ID == "scheduled-tasks") {
					index = i + 1
					break
				}
			}
			actions = append(actions, workspace.LayoutMenu{})
			copy(actions[index+1:], actions[index:])
			actions[index] = auditAction
		}
		layout.Topbar.Actions = actions
		return nil
	}
}

// The host owns this reserved destination. The API remains independently
// available; hiding the window is an experimental UI rollout switch.
func withConversationAuditWindow(next http.Handler, enabled bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSuffix(r.URL.Path, "/") != "/window/"+conversationAuditWindow {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		if !enabled {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "data": map[string]any{"windowKey": conversationAuditWindow, "title": "Conversation audit", "dataSource": map[string]any{}, "view": map[string]any{"containers": []any{}}}})
	})
}
