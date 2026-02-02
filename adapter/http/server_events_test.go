package http

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/modelcallctx"
	"github.com/viant/agently/genai/streaming"
	"github.com/viant/agently/internal/service/chat"
	agconv "github.com/viant/agently/pkg/agently/conversation"
)

func TestHandleConversationEvents_LongPoll_SequenceResume(t *testing.T) {
	conv := &apiconv.Conversation{
		Transcript: []*agconv.TranscriptView{
			{
				Message: []*agconv.MessageView{
					msgWithSeq("m1", 1),
					msgWithSeq("m2", 2),
					msgWithSeq("m3", 3),
				},
			},
		},
	}
	stub := &stubConvClient{conv: conv}
	svc := chat.NewServiceWithClient(stub, nil)
	srv := &Server{chatSvc: svc, eventSeq: map[string]uint64{}}

	require.NotNil(t, conv.Transcript)
	require.Len(t, conv.Transcript[0].Message, 3)
	require.NotNil(t, conv.Transcript[0].Message[2].Sequence)
	require.Equal(t, 3, *conv.Transcript[0].Message[2].Sequence)
	require.Equal(t, uint64(1), srv.nextEventSeq("c1", conv.Transcript[0].Message[0]))
	require.Equal(t, uint64(2), srv.nextEventSeq("c1", conv.Transcript[0].Message[1]))
	require.Equal(t, uint64(3), srv.nextEventSeq("c1", conv.Transcript[0].Message[2]))

	baseCtx := context.Background()
	msgs := flattenTranscriptMessages(conv)
	require.Len(t, msgs, 3)
	getResp, err := svc.Get(baseCtx, chat.GetRequest{ConversationID: "c1", IncludeToolCall: true})
	require.NoError(t, err)
	require.NotNil(t, getResp)
	require.NotNil(t, getResp.Conversation)
	require.Len(t, getResp.Conversation.Transcript[0].Message, 3)
	all, _, _, err := srv.collectEventEnvelopes(baseCtx, "c1", nil, false, false, 0, false, "")
	require.NoError(t, err)
	require.Len(t, all, 3)
	envs, seq, _, err := srv.collectEventEnvelopes(baseCtx, "c1", nil, false, false, 2, true, "")
	require.NoError(t, err)
	if len(envs) != 1 {
		t.Fatalf("expected 1 event, got %d (lastSeq=%d)", len(envs), seq)
	}
	require.Equal(t, uint64(3), seq)
	require.Equal(t, uint64(3), envs[0].Seq)

	req := httptest.NewRequest("GET", "/v1/api/conversations/c1/events?wait=1&since=2", nil)
	baseCtx = srv.withAuthFromRequest(req)
	baseCtx = memory.WithConversationID(baseCtx, "c1")
	envs, seq, _, err = srv.collectEventEnvelopes(baseCtx, "c1", nil, false, false, 2, true, "")
	require.NoError(t, err)
	if len(envs) != 1 {
		t.Fatalf("expected 1 event (auth ctx), got %d (lastSeq=%d)", len(envs), seq)
	}

	w := httptest.NewRecorder()
	srv.handleConversationEvents(w, req, "c1")

	resp := w.Result()
	defer resp.Body.Close()

	var out struct {
		Events []streamMessageEnvelope `json:"events"`
		Since  string                  `json:"since"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	require.Len(t, out.Events, 1)
	require.Equal(t, uint64(3), out.Events[0].Seq)
}

func TestHandleConversationEvents_SSEHistory(t *testing.T) {
	conv := &apiconv.Conversation{
		Transcript: []*agconv.TranscriptView{
			{
				Message: []*agconv.MessageView{
					msgWithSeq("m1", 1),
					msgWithSeq("m2", 2),
				},
			},
		},
	}
	stub := &stubConvClient{conv: conv}
	svc := chat.NewServiceWithClient(stub, nil)
	srv := &Server{chatSvc: svc, eventSeq: map[string]uint64{}}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.handleConversationEvents(w, r, "c1")
	}))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/events?history=1", nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	event, data := readSSEEvent(t, reader, 2*time.Second)
	require.Equal(t, "assistant_message", event)

	var env streamMessageEnvelope
	require.NoError(t, json.Unmarshal(data, &env))
	require.Equal(t, uint64(1), env.Seq)
	require.NotNil(t, env.Message)
	require.Equal(t, "m1", env.Message.Id)
	require.Equal(t, "c1", env.ConversationID)
}

func TestHandleConversationEvents_SSEDelta(t *testing.T) {
	conv := &apiconv.Conversation{
		Transcript: []*agconv.TranscriptView{{}},
	}
	stub := &stubConvClient{conv: conv}
	svc := chat.NewServiceWithClient(stub, nil)
	pub := streaming.NewPublisher()
	srv := &Server{chatSvc: svc, streamPub: pub, eventSeq: map[string]uint64{}}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.handleConversationEvents(w, r, "c1")
	}))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/events", nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = pub.Publish(context.Background(), &modelcallctx.StreamEvent{
			ConversationID: "c1",
			Message: &agconv.MessageView{
				Id:      "m1",
				Role:    "assistant",
				Type:    "text",
				Interim: 1,
			},
			ContentType: "application/json",
			Content: map[string]interface{}{
				"delta": "hi",
			},
		})
	}()

	reader := bufio.NewReader(resp.Body)
	event, data := readSSEEvent(t, reader, 2*time.Second)
	require.Equal(t, "interim_message", event)

	var env streamMessageEnvelope
	require.NoError(t, json.Unmarshal(data, &env))
	require.Equal(t, uint64(0), env.Seq)
	require.NotNil(t, env.Message)
	require.Equal(t, "m1", env.Message.Id)
	content, ok := env.Content.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "hi", content["delta"])
}

func TestHandleConversationEvents_SSEAttachmentLinked(t *testing.T) {
	conv := &apiconv.Conversation{
		Transcript: []*agconv.TranscriptView{
			{
				Message: []*agconv.MessageView{
					{
						Id:             "m1",
						ConversationId: "c1",
						Role:           "assistant",
						Type:           "text",
						Attachment: []*agconv.AttachmentView{
							{
								Uri:             ptrS("file://tmp/upload.txt"),
								MimeType:        "text/plain",
								ParentMessageId: ptrS("m1"),
							},
						},
					},
				},
			},
		},
	}
	stub := &stubConvClient{conv: conv}
	svc := chat.NewServiceWithClient(stub, nil)
	srv := &Server{chatSvc: svc, eventSeq: map[string]uint64{}}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.handleConversationEvents(w, r, "c1")
	}))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/events?history=1", nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	event, data := readSSEEvent(t, reader, 2*time.Second)
	require.Equal(t, "attachment_linked", event)

	var env streamMessageEnvelope
	require.NoError(t, json.Unmarshal(data, &env))
	require.NotNil(t, env.Message)
	require.Equal(t, "m1", env.Message.Id)
	require.Len(t, env.Message.Attachment, 1)
}

func TestHandleConversationEvents_SSEToolCallEvents(t *testing.T) {
	conv := &apiconv.Conversation{
		Transcript: []*agconv.TranscriptView{
			{
				Message: []*agconv.MessageView{
					{
						Id:             "m1",
						ConversationId: "c1",
						Role:           "assistant",
						Type:           "text",
						ToolCall: &agconv.ToolCallView{
							ToolName: "tools/grep",
							Status:   "running",
						},
					},
					{
						Id:             "m2",
						ConversationId: "c1",
						Role:           "assistant",
						Type:           "text",
						ToolCall: &agconv.ToolCallView{
							ToolName: "tools/grep",
							Status:   "completed",
						},
					},
					{
						Id:             "m3",
						ConversationId: "c1",
						Role:           "assistant",
						Type:           "text",
						ToolCall: &agconv.ToolCallView{
							ToolName: "tools/grep",
							Status:   "failed",
						},
					},
				},
			},
		},
	}
	stub := &stubConvClient{conv: conv}
	svc := chat.NewServiceWithClient(stub, nil)
	srv := &Server{chatSvc: svc, eventSeq: map[string]uint64{}}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.handleConversationEvents(w, r, "c1")
	}))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/events?history=1", nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	event, _ := readSSEEvent(t, reader, 2*time.Second)
	require.Equal(t, "tool_call_started", event)
	event, _ = readSSEEvent(t, reader, 2*time.Second)
	require.Equal(t, "tool_call_completed", event)
	event, _ = readSSEEvent(t, reader, 2*time.Second)
	require.Equal(t, "tool_call_failed", event)
}

func TestHandleConversationEvents_SSEDeltaSuppressesElicitation(t *testing.T) {
	conv := &apiconv.Conversation{
		Transcript: []*agconv.TranscriptView{{}},
	}
	stub := &stubConvClient{conv: conv}
	svc := chat.NewServiceWithClient(stub, nil)
	pub := streaming.NewPublisher()
	srv := &Server{chatSvc: svc, streamPub: pub, eventSeq: map[string]uint64{}}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.handleConversationEvents(w, r, "c1")
	}))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/events", nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = pub.Publish(context.Background(), &modelcallctx.StreamEvent{
			ConversationID: "c1",
			Message: &agconv.MessageView{
				Id:      "m1",
				Role:    "assistant",
				Type:    "text",
				Interim: 1,
			},
			ContentType: "application/json",
			Content: map[string]interface{}{
				"delta": "```json\n{\"type\":\"elicitation\",\"message\":\"need input\"}\n```",
			},
		})
		_ = pub.Publish(context.Background(), &modelcallctx.StreamEvent{
			ConversationID: "c1",
			Message: &agconv.MessageView{
				Id:      "m1",
				Role:    "assistant",
				Type:    "text",
				Interim: 1,
			},
			ContentType: "application/json",
			Content: map[string]interface{}{
				"delta": "ok",
			},
		})
	}()

	reader := bufio.NewReader(resp.Body)
	event, data := readSSEEvent(t, reader, 2*time.Second)
	require.Equal(t, "interim_message", event)

	var env streamMessageEnvelope
	require.NoError(t, json.Unmarshal(data, &env))
	content, ok := env.Content.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "ok", content["delta"])
}

func TestHandleConversationEvents_SSEElicitationEvent(t *testing.T) {
	conv := &apiconv.Conversation{
		Transcript: []*agconv.TranscriptView{{
			Message: []*agconv.MessageView{
				{
					Id:             "m1",
					ConversationId: "c1",
					Role:           "assistant",
					Type:           "text",
					ElicitationId:  ptrS("e1"),
					Content:        ptrS("{\"type\":\"elicitation\",\"message\":\"need input\",\"requestedSchema\":{\"type\":\"object\",\"properties\":{}}}"),
				},
			},
		}},
	}
	stub := &stubConvClient{conv: conv}
	svc := chat.NewServiceWithClient(stub, nil)
	srv := &Server{chatSvc: svc, eventSeq: map[string]uint64{}}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.handleConversationEvents(w, r, "c1")
	}))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/events?history=1", nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	event, data := readSSEEvent(t, reader, 2*time.Second)
	require.Equal(t, "elicitation", event)

	var env streamMessageEnvelope
	require.NoError(t, json.Unmarshal(data, &env))
	require.Equal(t, "m1", env.Message.Id)
	require.Equal(t, "application/elicitation+json", env.ContentType)
}

func msgWithSeq(id string, seq int) *agconv.MessageView {
	s := seq
	return &agconv.MessageView{
		Id:       id,
		Role:     "assistant",
		Type:     "text",
		Sequence: &s,
	}
}

func ptrS(v string) *string { return &v }

func readSSEEvent(t *testing.T, r *bufio.Reader, timeout time.Duration) (string, []byte) {
	t.Helper()
	type result struct {
		event string
		data  []byte
		err   error
	}
	ch := make(chan result, 1)
	go func() {
		var event string
		var data bytes.Buffer
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				ch <- result{err: err}
				return
			}
			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				if event != "" || data.Len() > 0 {
					ch <- result{event: event, data: bytes.TrimSpace(data.Bytes())}
					return
				}
				continue
			}
			if strings.HasPrefix(line, ":") {
				continue
			}
			if strings.HasPrefix(line, "event:") {
				event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
				continue
			}
			if strings.HasPrefix(line, "data:") {
				if data.Len() > 0 {
					data.WriteByte('\n')
				}
				data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}
	}()
	select {
	case res := <-ch:
		if res.err != nil {
			t.Fatalf("read SSE: %v", res.err)
		}
		return res.event, res.data
	case <-time.After(timeout):
		t.Fatalf("timeout waiting for SSE event")
		return "", nil
	}
}

type stubConvClient struct {
	conv *apiconv.Conversation
}

func (s *stubConvClient) GetConversation(ctx context.Context, id string, options ...apiconv.Option) (*apiconv.Conversation, error) {
	return s.conv, nil
}
func (s *stubConvClient) GetConversations(ctx context.Context, input *apiconv.Input) ([]*apiconv.Conversation, error) {
	return nil, nil
}
func (s *stubConvClient) PatchConversations(ctx context.Context, conversations *apiconv.MutableConversation) error {
	return nil
}
func (s *stubConvClient) GetPayload(ctx context.Context, id string) (*apiconv.Payload, error) {
	return nil, nil
}
func (s *stubConvClient) PatchPayload(ctx context.Context, payload *apiconv.MutablePayload) error {
	return nil
}
func (s *stubConvClient) PatchMessage(ctx context.Context, message *apiconv.MutableMessage) error {
	return nil
}
func (s *stubConvClient) GetMessage(ctx context.Context, id string, options ...apiconv.Option) (*apiconv.Message, error) {
	return nil, nil
}
func (s *stubConvClient) GetMessageByElicitation(ctx context.Context, conversationID, elicitationID string) (*apiconv.Message, error) {
	return nil, nil
}
func (s *stubConvClient) PatchModelCall(ctx context.Context, modelCall *apiconv.MutableModelCall) error {
	return nil
}
func (s *stubConvClient) PatchToolCall(ctx context.Context, toolCall *apiconv.MutableToolCall) error {
	return nil
}
func (s *stubConvClient) PatchTurn(ctx context.Context, turn *apiconv.MutableTurn) error {
	return nil
}
func (s *stubConvClient) DeleteConversation(ctx context.Context, id string) error {
	return nil
}
func (s *stubConvClient) DeleteMessage(ctx context.Context, conversationID, messageID string) error {
	return nil
}
