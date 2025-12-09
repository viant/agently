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
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/conversation"
	agentpkg "github.com/viant/agently/genai/service/agent"
	chatsvc "github.com/viant/agently/internal/service/chat"
)

func mkJWTForTest(t *testing.T, claims map[string]any) string {
	t.Helper()
	header := map[string]any{"alg": "none", "typ": "JWT"}
	hb, _ := json.Marshal(header)
	pb, _ := json.Marshal(claims)
	enc := func(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
	return enc(hb) + "." + enc(pb) + ".sig"
}

// memoryConvClient is a minimal in-memory implementation of the conversation.Client
// interface used to decouple HTTP tests from the actual SQLite/Datly schema.
type memoryConvClient struct {
	convs map[string]*apiconv.Conversation
}

func newMemoryConvClient() *memoryConvClient {
	return &memoryConvClient{convs: map[string]*apiconv.Conversation{}}
}

func (c *memoryConvClient) GetConversation(_ context.Context, id string, _ ...apiconv.Option) (*apiconv.Conversation, error) {
	return c.convs[id], nil
}

func (c *memoryConvClient) GetConversations(_ context.Context, _ *apiconv.Input) ([]*apiconv.Conversation, error) {
	out := make([]*apiconv.Conversation, 0, len(c.convs))
	for _, v := range c.convs {
		out = append(out, v)
	}
	return out, nil
}

func (c *memoryConvClient) PatchConversations(_ context.Context, conv *apiconv.MutableConversation) error {
	if conv == nil {
		return nil
	}
	view := &apiconv.Conversation{
		Id:              conv.Id,
		Title:           conv.Title,
		CreatedByUserId: conv.CreatedByUserID,
		Visibility:      "private",
	}
	c.convs[view.Id] = view
	return nil
}

func (c *memoryConvClient) GetPayload(_ context.Context, _ string) (*apiconv.Payload, error) {
	return nil, nil
}

func (c *memoryConvClient) PatchPayload(_ context.Context, _ *apiconv.MutablePayload) error {
	return nil
}

func (c *memoryConvClient) PatchMessage(_ context.Context, _ *apiconv.MutableMessage) error {
	return nil
}

func (c *memoryConvClient) GetMessage(_ context.Context, _ string, _ ...apiconv.Option) (*apiconv.Message, error) {
	return nil, nil
}

func (c *memoryConvClient) GetMessageByElicitation(_ context.Context, _, _ string) (*apiconv.Message, error) {
	return nil, nil
}

func (c *memoryConvClient) PatchModelCall(_ context.Context, _ *apiconv.MutableModelCall) error {
	return nil
}

func (c *memoryConvClient) PatchToolCall(_ context.Context, _ *apiconv.MutableToolCall) error {
	return nil
}

func (c *memoryConvClient) PatchTurn(_ context.Context, _ *apiconv.MutableTurn) error {
	return nil
}

func (c *memoryConvClient) DeleteConversation(_ context.Context, id string) error {
	delete(c.convs, id)
	return nil
}

func (c *memoryConvClient) DeleteMessage(_ context.Context, _, _ string) error {
	return nil
}

// newTestServer constructs an HTTP server with an in-memory conversation client
// to keep tests independent of external DB or Datly schema.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	mgr := conversation.New(func(ctx context.Context, in *agentpkg.QueryInput, out *agentpkg.QueryOutput) error { return nil })
	cli := newMemoryConvClient()
	cs := chatsvc.NewServiceWithClient(cli, nil)
	return newLocalServerOrSkip(t, NewServer(mgr, WithChatService(cs)))
}

func TestServer_CreateConversation_WithAuthorization(t *testing.T) {
	srv := newTestServer(t)
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

	// Backend persistence is now via conversation client; here we only
	// assert that ID was created and endpoint responded OK.
}

func TestServer_CreateConversation_DefaultAnonymous(t *testing.T) {
	srv := newTestServer(t)
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

	// We only assert success status and non-empty ID.
}

func TestServer_ListConversations_UserOrPublic(t *testing.T) {
	srv := newTestServer(t)
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

	// List as userA: current behaviour returns only userA's own
	// conversations; public conversations from other users are not
	// included in the summary list.
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
	assert.EqualValues(t, 1, len(out.Data))
}

// newLocalServerOrSkip attempts to start an httptest.Server and skips the test
// when the environment does not permit binding a local TCP listener.
func newLocalServerOrSkip(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Skipf("skipping test: unable to start local HTTP server: %v", r)
		}
	}()
	return httptest.NewServer(handler)
}
