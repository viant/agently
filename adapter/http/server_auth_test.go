package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/conversation"
	agentpkg "github.com/viant/agently/genai/service/agent"
	daofactory "github.com/viant/agently/internal/dao/factory"
	dstore "github.com/viant/agently/internal/domain/adapter"
)

func mkJWTForTest(t *testing.T, claims map[string]any) string {
	t.Helper()
	header := map[string]any{"alg": "none", "typ": "JWT"}
	hb, _ := json.Marshal(header)
	pb, _ := json.Marshal(claims)
	enc := func(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
	return enc(hb) + "." + enc(pb) + ".sig"
}

func TestServer_CreateConversation_WithAuthorization(t *testing.T) {
	// prepare in-memory store and manager
	apis, err := daofactory.New(context.Background(), daofactory.DAOInMemory, nil)
	assert.NoError(t, err)
	store := dstore.New(apis.Conversation, apis.Message, apis.Turn, apis.ModelCall, apis.ToolCall, apis.Payload, apis.Usage)

	mgr := conversation.New(func(ctx context.Context, in *agentpkg.QueryInput, out *agentpkg.QueryOutput) error { return nil })

	srv := httptest.NewServer(NewServer(mgr, WithStore(store)))
	defer srv.Close()

	token := mkJWTForTest(t, map[string]any{"sub": "user-123"})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/api/conversations", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := srv.Client().Do(req)
	assert.NoError(t, err)
	defer resp.Body.Close()
	assert.EqualValues(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Status string `json:"status"`
		Data   struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	assert.NotEmpty(t, body.Data.ID)

	// verify created_by_user_id persisted
	cv, err := store.Conversations().Get(context.Background(), body.Data.ID)
	assert.NoError(t, err)
	if assert.NotNil(t, cv) {
		assert.NotNil(t, cv.CreatedByUserID)
		assert.EqualValues(t, "user-123", *cv.CreatedByUserID)
	}
}

func TestServer_CreateConversation_DefaultAnonymous(t *testing.T) {
	apis, err := daofactory.New(context.Background(), daofactory.DAOInMemory, nil)
	assert.NoError(t, err)
	store := dstore.New(apis.Conversation, apis.Message, apis.Turn, apis.ModelCall, apis.ToolCall, apis.Payload, apis.Usage)

	mgr := conversation.New(func(ctx context.Context, in *agentpkg.QueryInput, out *agentpkg.QueryOutput) error { return nil })

	srv := httptest.NewServer(NewServer(mgr, WithStore(store)))
	defer srv.Close()

	// No Authorization header
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/api/conversations", http.NoBody)
	resp, err := srv.Client().Do(req)
	assert.NoError(t, err)
	defer resp.Body.Close()
	assert.EqualValues(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Status string `json:"status"`
		Data   struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	assert.NotEmpty(t, body.Data.ID)

	// verify created_by_user_id persisted as anonymous
	cv, err := store.Conversations().Get(context.Background(), body.Data.ID)
	assert.NoError(t, err)
	if assert.NotNil(t, cv) {
		assert.NotNil(t, cv.CreatedByUserID)
		assert.EqualValues(t, "anonymous", *cv.CreatedByUserID)
	}
}

func TestServer_ListConversations_UserOrPublic(t *testing.T) {
	apis, err := daofactory.New(context.Background(), daofactory.DAOInMemory, nil)
	assert.NoError(t, err)
	store := dstore.New(apis.Conversation, apis.Message, apis.Turn, apis.ModelCall, apis.ToolCall, apis.Payload, apis.Usage)

	mgr := conversation.New(func(ctx context.Context, in *agentpkg.QueryInput, out *agentpkg.QueryOutput) error { return nil })
	srv := httptest.NewServer(NewServer(mgr, WithStore(store)))
	defer srv.Close()

	// Create: userA private
	tokA := mkJWTForTest(t, map[string]any{"sub": "userA"})
	reqA, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/api/conversations", http.NoBody)
	reqA.Header.Set("Authorization", "Bearer "+tokA)
	respA, err := srv.Client().Do(reqA)
	assert.NoError(t, err)
	respA.Body.Close()

	// Create: userB private
	tokB := mkJWTForTest(t, map[string]any{"sub": "userB"})
	reqB, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/api/conversations", http.NoBody)
	reqB.Header.Set("Authorization", "Bearer "+tokB)
	respB, err := srv.Client().Do(reqB)
	assert.NoError(t, err)
	respB.Body.Close()

	// Create: userB public
	bodyPub := strings.NewReader(`{"visibility":"public"}`)
	reqBP, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/api/conversations", bodyPub)
	reqBP.Header.Set("Content-Type", "application/json")
	reqBP.Header.Set("Authorization", "Bearer "+tokB)
	respBP, err := srv.Client().Do(reqBP)
	assert.NoError(t, err)
	respBP.Body.Close()

	// List as userA: expect own (userA) and public(userB)
	listReq, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/api/conversations", nil)
	listReq.Header.Set("Authorization", "Bearer "+tokA)
	listResp, err := srv.Client().Do(listReq)
	assert.NoError(t, err)
	defer listResp.Body.Close()
	assert.EqualValues(t, http.StatusOK, listResp.StatusCode)

	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(listResp.Body).Decode(&out)
	// Expect 2 results: userA private + userB public
	assert.EqualValues(t, 2, len(out.Data))
}
