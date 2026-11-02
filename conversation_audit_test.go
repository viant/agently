package agently

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/viant/agently-core/service/workspace"
)

func TestConversationAuditHostFlagAndNavigation(t *testing.T) {
	t.Setenv("AGENTLY_EXPERIMENTAL_CONVERSATION_AUDIT_UI", "")
	t.Setenv("AGENTLY_CONVERSATION_AUDIT_USERS", "")
	opts, err := conversationAuditOptions()
	require.NoError(t, err)
	require.False(t, opts.UIEnabled)
	layout := &workspace.Layout{Topbar: &workspace.LayoutTopbar{Actions: []workspace.LayoutMenu{{ID: "before"}, {ID: "automation", Action: &workspace.LayoutAction{Type: "window", WindowKey: "schedule"}}, {ID: "after"}}}}
	require.NoError(t, conversationAuditLayout(true)(context.Background(), layout))
	require.Len(t, layout.Topbar.Actions, 4)
	require.Equal(t, conversationAuditWindow, layout.Topbar.Actions[2].ID)
	require.Equal(t, "after", layout.Topbar.Actions[3].ID)
	require.NoError(t, conversationAuditLayout(true)(context.Background(), layout))
	require.Len(t, layout.Topbar.Actions, 4)
	require.NoError(t, conversationAuditLayout(false)(context.Background(), layout))
	require.Len(t, layout.Topbar.Actions, 3)
	require.Equal(t, "automation", layout.Topbar.Actions[1].ID)
	fallback := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	for _, enabled := range []bool{false, true} {
		h := withConversationAuditWindow(fallback, enabled)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/window/conversation-audit?applyPermission=true", nil))
		if !enabled {
			require.Equal(t, 404, w.Code)
		} else {
			require.Equal(t, 200, w.Code)
			var body struct {
				Status string
				Data   struct{ WindowKey string }
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			require.Equal(t, "ok", body.Status)
			require.Equal(t, conversationAuditWindow, body.Data.WindowKey)
		}
		w = httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/window/schedule", nil))
		require.Equal(t, 204, w.Code)
	}
	t.Setenv("AGENTLY_EXPERIMENTAL_CONVERSATION_AUDIT_UI", "true")
	t.Setenv("AGENTLY_CONVERSATION_AUDIT_USERS", `["example-auditor"]`)
	opts, err = conversationAuditOptions()
	require.NoError(t, err)
	require.True(t, opts.UIEnabled)
	require.Equal(t, []string{"example-auditor"}, opts.AuditorUserIDs)
	t.Setenv("AGENTLY_CONVERSATION_AUDIT_USERS", "bad config")
	_, err = conversationAuditOptions()
	require.Error(t, err)
}
