package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/conversation"
	agentpkg "github.com/viant/agently/genai/extension/fluxor/llm/agent"
	"github.com/viant/agently/genai/memory"
)

// echoHandler is a lightweight QueryHandler stub that echoes the user query and
// records the incoming message in the provided memory store so that list &
// retrieval work during tests.
func echoHandler(store *memory.HistoryStore) conversation.QueryHandler {
	return func(ctx context.Context, input *agentpkg.QueryInput, output *agentpkg.QueryOutput) error {
		_ = store.AddMessage(ctx, input.ConversationID, memory.Message{Role: "user", Content: input.Query})
		output.Content = "echo: " + input.Query
		return nil
	}
}

func TestConversationREST_EndToEnd(t *testing.T) {
	store := memory.NewHistoryStore()
	mgr := conversation.New(store, echoHandler(store))

	// Build HTTP server around the manager.
	srv := httptest.NewServer(NewServer(mgr))
	defer srv.Close()

	client := srv.Client()

	// 1. Create new conversation (POST /conversations)
	resp, err := client.Post(srv.URL+"/v1/api/conversations", "application/json", nil)
	assert.NoError(t, err)
	defer resp.Body.Close()
	assert.EqualValues(t, http.StatusOK, resp.StatusCode)

	var createResp map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&createResp)
	convID := createResp["id"]
	assert.NotEmpty(t, convID)

	// 2. Send a chat message (should respond 202 Accepted with generated message ID).
	body := map[string]string{"content": "hello"}
	raw, _ := json.Marshal(body)
	resp, err = client.Post(srv.URL+"/v1/api/conversations/"+convID+"/messages", "application/json", bytes.NewReader(raw))
	assert.NoError(t, err)
	var postResp struct {
		Status string            `json:"status"`
		Data   map[string]string `json:"data"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&postResp)
	resp.Body.Close()
	assert.EqualValues(t, http.StatusAccepted, resp.StatusCode)
	msgID := postResp.Data["id"]
	assert.NotEmpty(t, msgID)

	// 3. List conversations → should now contain the ID.
	resp, err = client.Get(srv.URL + "/v1/api/conversations")
	assert.NoError(t, err)
	defer resp.Body.Close()
	assert.EqualValues(t, http.StatusOK, resp.StatusCode)
	var listResp []map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&listResp)

	expectedList := []map[string]string{{"id": convID}}
	assert.EqualValues(t, expectedList, listResp)

	// 4. Fetch messages for conversation (poll until at least 1 message appears).
	var msgResp []map[string]interface{}
	for i := 0; i < 10; i++ {
		resp, err = client.Get(srv.URL + "/v1/api/conversations/" + convID + "/messages")
		assert.NoError(t, err)
		resp.Body.Close()
		_ = json.NewDecoder(resp.Body).Decode(&msgResp)
		if len(msgResp) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	resp.Body.Close()
	assert.GreaterOrEqual(t, len(msgResp), 1)

	// 5. Fetch single message by ID – should match content.
	resp, err = client.Get(srv.URL + "/v1/api/conversations/" + convID + "/messages/" + msgID)
	assert.NoError(t, err)
	defer resp.Body.Close()
	assert.EqualValues(t, http.StatusOK, resp.StatusCode)

	var singleMsg memory.Message
	_ = json.NewDecoder(resp.Body).Decode(&singleMsg)
	assert.EqualValues(t, "user", singleMsg.Role)
	assert.EqualValues(t, "hello", singleMsg.Content)
}
