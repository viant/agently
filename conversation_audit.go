package agently

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	audit "github.com/viant/agently-core/service/conversationaudit"
	"github.com/viant/agently-core/service/workspace"
	wscfg "github.com/viant/agently-core/workspace/config"
)

const conversationAuditWindow = "conversation-audit"

func conversationAuditOptions(root *wscfg.Root) (audit.Options, error) {
	var opts audit.Options
	if root == nil {
		return opts, nil
	}
	section, present := root.Raw["conversationAudit"]
	if !present {
		return opts, nil
	}
	var config struct {
		UI struct {
			Enabled bool `json:"enabled"`
		} `json:"ui"`
		Permissions struct {
			Audit struct {
				Users []string `json:"users"`
			} `json:"conversation_audit"`
			Admin struct {
				Users []string `json:"users"`
			} `json:"conversation_audit_admin"`
		} `json:"permissions"`
	}
	data, err := json.Marshal(section)
	if err != nil {
		return opts, fmt.Errorf("conversationAudit: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if section == nil {
		return opts, fmt.Errorf("conversationAudit must be a configuration object")
	}
	if err = decoder.Decode(&config); err != nil {
		return opts, fmt.Errorf("conversationAudit: %w", err)
	}
	opts.UIEnabled = config.UI.Enabled
	opts.UserIDs = config.Permissions.Audit.Users
	opts.AuditorUserIDs = config.Permissions.Admin.Users
	for _, list := range []struct {
		name  string
		users []string
		admin bool
	}{
		{"conversation_audit", opts.UserIDs, false}, {"conversation_audit_admin", opts.AuditorUserIDs, true},
	} {
		for n, id := range list.users {
			id = strings.TrimSpace(id)
			if id == "" || len(id) > 256 || (list.admin && id == "*") {
				return audit.Options{}, fmt.Errorf("conversationAudit.permissions.%s.users: expected nonempty server user IDs; wildcard is allowed only for conversation_audit", list.name)
			}
			list.users[n] = id
		}
	}
	return opts, nil
}

func conversationAuditLayout(opts audit.Options) func(context.Context, *workspace.Layout) error {
	return func(ctx context.Context, layout *workspace.Layout) error {
		caps, err := opts.Capabilities(ctx)
		enabled := err == nil && caps.UIEnabled && caps.CanAudit
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
// available to authorized principals when the experimental window is hidden.
func withConversationAuditWindow(next http.Handler, opts audit.Options) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSuffix(r.URL.Path, "/") != "/window/"+conversationAuditWindow {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		if !opts.UIEnabled {
			http.NotFound(w, r)
			return
		}
		caps, err := opts.Capabilities(r.Context())
		if err != nil || !caps.CanAudit {
			status := http.StatusForbidden
			if err != nil {
				status = http.StatusUnauthorized
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "Conversation audit access denied."})
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
