package agently

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	audit "github.com/viant/agently-core/service/conversationaudit"
	"github.com/viant/agently-core/service/workspace"
	wscfg "github.com/viant/agently-core/workspace/config"
	"github.com/viant/mcp/client/auth/link"
	"gopkg.in/yaml.v3"
)

func auditPrincipal(ctx context.Context, id string) context.Context {
	return link.WithUserInfo(ctx, &link.UserInfo{Subject: id})
}

func TestConversationAuditWorkspaceConfiguration(t *testing.T) {
	// Removed variables cannot enable the window or grant access.
	t.Setenv("AGENTLY_EXPERIMENTAL_CONVERSATION_AUDIT_UI", "true")
	t.Setenv("AGENTLY_CONVERSATION_AUDIT_USERS", `["alice"]`)
	for _, tc := range []struct {
		name, config string
		invalid      bool
	}{
		{"missing", "{}", false},
		{"empty", "conversationAudit: {}", false},
		{"valid", "conversationAudit:\n  ui: {enabled: true}\n  permissions:\n    conversation_audit: {users: ['*', ' alice ']}\n    conversation_audit_admin: {users: [' alice ']}", false},
		{"admin wildcard", "conversationAudit: {permissions: {conversation_audit_admin: {users: ['*']}}}", true},
		{"blank ID", "conversationAudit: {permissions: {conversation_audit: {users: [' ']}}}", true},
		{"wrong list", "conversationAudit: {permissions: {conversation_audit: {users: alice}}}", true},
		{"wrong ID type", "conversationAudit: {permissions: {conversation_audit: {users: [123]}}}", true},
		{"wrong UI type", "conversationAudit: {ui: {enabled: 'true'}}", true},
		{"unknown grant", "conversationAudit: {permissions: {conversation_audit_admn: {users: ['alice']}}}", true},
		{"null section", "conversationAudit: null", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var root wscfg.Root
			require.NoError(t, yaml.Unmarshal([]byte(tc.config), &root))
			opts, err := conversationAuditOptions(&root)
			if tc.invalid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			caps, err := opts.Capabilities(auditPrincipal(context.Background(), "alice"))
			require.NoError(t, err)
			if tc.name == "valid" {
				require.True(t, caps.UIEnabled)
				require.True(t, caps.CanAudit)
				require.True(t, caps.CanAuditAllUsers)
				require.Equal(t, []string{"*", "alice"}, opts.UserIDs)
			} else {
				require.False(t, caps.UIEnabled)
				require.False(t, caps.CanAudit)
			}
		})
	}
	opts, err := conversationAuditOptions(nil)
	require.NoError(t, err)
	require.Equal(t, audit.Options{}, opts)
}

func TestConversationAuditHostNavigationAndWindowAccess(t *testing.T) {
	opts := audit.Options{UIEnabled: true, UserIDs: []string{"alice"}, AuditorUserIDs: []string{"admin"}}
	for _, tc := range []struct {
		id      string
		enabled bool
		status  int
	}{
		{"", true, 401}, {"denied", true, 403}, {"alice", true, 200}, {"admin", true, 200}, {"admin", false, 404},
	} {
		t.Run(tc.id+http.StatusText(tc.status), func(t *testing.T) {
			options := opts
			options.UIEnabled = tc.enabled
			ctx := context.Background()
			if tc.id != "" {
				ctx = auditPrincipal(ctx, tc.id)
			}
			layout := &workspace.Layout{Topbar: &workspace.LayoutTopbar{Actions: []workspace.LayoutMenu{{ID: "before"}, {ID: "automation", Action: &workspace.LayoutAction{Type: "window", WindowKey: "schedule"}}, {ID: "after"}}}}
			for n := 0; n < 2; n++ {
				require.NoError(t, conversationAuditLayout(options)(ctx, layout))
				if tc.status == 200 {
					require.Len(t, layout.Topbar.Actions, 4)
					require.Equal(t, conversationAuditWindow, layout.Topbar.Actions[2].ID)
				} else {
					require.Len(t, layout.Topbar.Actions, 3)
				}
			}
			// Decorating a previously authorized layout must not leak its action.
			require.NoError(t, conversationAuditLayout(options)(auditPrincipal(ctx, "denied"), layout))
			require.Len(t, layout.Topbar.Actions, 3)
			h := withConversationAuditWindow(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }), options)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", "/window/conversation-audit?applyPermission=false", nil).WithContext(ctx))
			require.Equal(t, tc.status, w.Code, w.Body.String())
			require.Equal(t, "private, no-store", w.Header().Get("Cache-Control"))
			if tc.status == 200 {
				var body struct {
					Status string
					Data   struct{ WindowKey string }
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
				require.Equal(t, conversationAuditWindow, body.Data.WindowKey)
			}
			w = httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", "/window/schedule", nil).WithContext(ctx))
			require.Equal(t, 204, w.Code)
		})
	}
}
