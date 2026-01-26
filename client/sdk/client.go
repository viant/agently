package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	conv "github.com/viant/agently/client/conversation"
)

// Client is a minimal HTTP SDK for Agently REST APIs.
type Client struct {
	baseURL       string
	http          *http.Client
	tokenProvider TokenProvider
	headers       map[string]string
	retry         RetryPolicy
	requestHook   func(*http.Request) error
	responseHook  func(*http.Response) error
}

// New constructs a new SDK client.
func New(baseURL string, opts ...Option) *Client {
	c := &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		http:    &http.Client{},
	}
	c.retry = RetryPolicy{
		MaxAttempts: 1,
		Delay:       200 * time.Millisecond,
		RetryStatuses: map[int]struct{}{
			http.StatusTooManyRequests:    {},
			http.StatusBadGateway:         {},
			http.StatusServiceUnavailable: {},
			http.StatusGatewayTimeout:     {},
		},
		RetryMethods: map[string]struct{}{
			http.MethodGet:  {},
			http.MethodHead: {},
		},
		RetryOnError: true,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	return c
}

// CreateConversation creates a new conversation.
func (c *Client) CreateConversation(ctx context.Context, req *CreateConversationRequest) (*CreateConversationResponse, error) {
	var resp CreateConversationResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v1/api/conversations", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ListConversations lists conversation summaries.
func (c *Client) ListConversations(ctx context.Context) ([]ConversationSummary, error) {
	var resp []ConversationSummary
	if err := c.doJSON(ctx, http.MethodGet, "/v1/api/conversations", nil, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// PostMessage sends a message to a conversation and returns the message id.
func (c *Client) PostMessage(ctx context.Context, conversationID string, req *PostMessageRequest) (*PostMessageResponse, error) {
	if strings.TrimSpace(conversationID) == "" {
		return nil, fmt.Errorf("conversationID is required")
	}
	uri := fmt.Sprintf("/v1/api/conversations/%s/messages", url.PathEscape(conversationID))
	var resp PostMessageResponse
	if err := c.doJSON(ctx, http.MethodPost, uri, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetMessages fetches a conversation transcript via GET /messages.
func (c *Client) GetMessages(ctx context.Context, conversationID string, since string) (*conv.Conversation, error) {
	if strings.TrimSpace(conversationID) == "" {
		return nil, fmt.Errorf("conversationID is required")
	}
	uri := fmt.Sprintf("/v1/api/conversations/%s/messages", url.PathEscape(conversationID))
	if since != "" {
		uri += "?since=" + url.QueryEscape(since)
	}
	var wrapped struct {
		Conversation *conv.Conversation `json:"conversation"`
	}
	err := c.doJSON(ctx, http.MethodGet, uri, nil, &wrapped)
	if err == nil && wrapped.Conversation != nil {
		return wrapped.Conversation, nil
	}
	var convo conv.Conversation
	if err2 := c.doJSON(ctx, http.MethodGet, uri, nil, &convo); err2 == nil {
		return &convo, nil
	}
	return nil, err
}

// GetMessagesWithOptions fetches messages with extended query options.
func (c *Client) GetMessagesWithOptions(ctx context.Context, conversationID string, opts *GetMessagesOptions) (*conv.Conversation, error) {
	if strings.TrimSpace(conversationID) == "" {
		return nil, fmt.Errorf("conversationID is required")
	}
	qs := []string{}
	if opts != nil {
		if strings.TrimSpace(opts.Since) != "" {
			qs = append(qs, "since="+url.QueryEscape(opts.Since))
		}
		if opts.IncludeModelCallPayload {
			qs = append(qs, "includeModelCallPayload=1")
		}
		if opts.IncludeLinked {
			qs = append(qs, "includeLinked=1")
		}
	}
	uri := fmt.Sprintf("/v1/api/conversations/%s/messages", url.PathEscape(conversationID))
	if len(qs) > 0 {
		uri += "?" + strings.Join(qs, "&")
	}
	var wrapped struct {
		Conversation *conv.Conversation `json:"conversation"`
	}
	err := c.doJSON(ctx, http.MethodGet, uri, nil, &wrapped)
	if err == nil && wrapped.Conversation != nil {
		return wrapped.Conversation, nil
	}
	var convo conv.Conversation
	if err2 := c.doJSON(ctx, http.MethodGet, uri, nil, &convo); err2 == nil {
		return &convo, nil
	}
	return nil, err
}

// StreamEvents opens an SSE stream to /events and emits envelopes and deltas.
// The caller should cancel ctx to stop the stream.
func (c *Client) StreamEvents(ctx context.Context, conversationID string, since string, include []string) (<-chan *StreamEventEnvelope, <-chan error, error) {
	if strings.TrimSpace(conversationID) == "" {
		return nil, nil, fmt.Errorf("conversationID is required")
	}
	qs := []string{}
	if since != "" {
		qs = append(qs, "since="+url.QueryEscape(since))
	}
	if len(include) > 0 {
		qs = append(qs, "include="+url.QueryEscape(strings.Join(include, ",")))
	}
	uri := fmt.Sprintf("/v1/api/conversations/%s/events", url.PathEscape(conversationID))
	if len(qs) > 0 {
		uri += "?" + strings.Join(qs, "&")
	}
	req, err := c.newRequest(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("stream failed: %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	events := make(chan *StreamEventEnvelope, 64)
	errs := make(chan error, 1)
	go func() {
		defer resp.Body.Close()
		defer close(events)
		defer close(errs)
		if err := readSSE(ctx, resp.Body, events); err != nil {
			errs <- err
		}
	}()
	return events, errs, nil
}

// PollEvents long-polls /events with wait=ms and returns any events.
func (c *Client) PollEvents(ctx context.Context, conversationID string, since string, include []string, wait time.Duration) (*PollResponse, error) {
	if strings.TrimSpace(conversationID) == "" {
		return nil, fmt.Errorf("conversationID is required")
	}
	qs := []string{}
	if since != "" {
		qs = append(qs, "since="+url.QueryEscape(since))
	}
	if len(include) > 0 {
		qs = append(qs, "include="+url.QueryEscape(strings.Join(include, ",")))
	}
	if wait > 0 {
		qs = append(qs, "wait="+strconv.Itoa(int(wait.Milliseconds())))
	}
	uri := fmt.Sprintf("/v1/api/conversations/%s/events", url.PathEscape(conversationID))
	if len(qs) > 0 {
		uri += "?" + strings.Join(qs, "&")
	}
	var out PollResponse
	if err := c.doJSON(ctx, http.MethodGet, uri, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PostAndStream posts a message then streams events for the conversation.
// The caller should cancel ctx to stop the stream.
func (c *Client) PostAndStream(ctx context.Context, conversationID string, req *PostMessageRequest, include []string) (string, <-chan *StreamEventEnvelope, <-chan error, error) {
	resp, err := c.PostMessage(ctx, conversationID, req)
	if err != nil {
		return "", nil, nil, err
	}
	events, errs, err := c.StreamEvents(ctx, conversationID, "", include)
	if err != nil {
		return resp.ID, nil, nil, err
	}
	return resp.ID, events, errs, nil
}

// StreamConversation emits merged updates for assistant output, combining deltas and final messages.
func (c *Client) StreamConversation(ctx context.Context, conversationID string, since string) (<-chan ChatTurnUpdate, <-chan error, error) {
	events, errs, err := c.StreamEvents(ctx, conversationID, since, []string{"text", "tool_op", "control"})
	if err != nil {
		return nil, nil, err
	}
	out := make(chan ChatTurnUpdate, 64)
	go func() {
		defer close(out)
		buf := NewMessageBuffer()
		for ev := range events {
			if ev == nil || ev.Message == nil {
				continue
			}
			id, text, ok := buf.ApplyEvent(ev)
			if !ok {
				continue
			}
			final := ev.Message.Interim == 0
			out <- ChatTurnUpdate{MessageID: id, Text: text, IsFinal: final}
		}
	}()
	return out, errs, nil
}

// PostAndStreamConversation posts a message and returns merged updates for assistant output.
func (c *Client) PostAndStreamConversation(ctx context.Context, conversationID string, req *PostMessageRequest) (string, <-chan ChatTurnUpdate, <-chan error, error) {
	resp, err := c.PostMessage(ctx, conversationID, req)
	if err != nil {
		return "", nil, nil, err
	}
	updates, errs, err := c.StreamConversation(ctx, conversationID, "")
	if err != nil {
		return resp.ID, nil, nil, err
	}
	return resp.ID, updates, errs, nil
}

// RunTool executes a tool in the context of a conversation.
func (c *Client) RunTool(ctx context.Context, conversationID string, req *ToolRunRequest) (ToolRunResponse, error) {
	if strings.TrimSpace(conversationID) == "" {
		return nil, fmt.Errorf("conversationID is required")
	}
	if req == nil || strings.TrimSpace(req.Service) == "" {
		return nil, fmt.Errorf("tool service is required")
	}
	uri := fmt.Sprintf("/v1/api/conversations/%s/tools/run", url.PathEscape(conversationID))
	var out map[string]interface{}
	if err := c.doJSON(ctx, http.MethodPost, uri, req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetPayload downloads a payload by id.
func (c *Client) GetPayload(ctx context.Context, payloadID string) (*PayloadResponse, error) {
	if strings.TrimSpace(payloadID) == "" {
		return nil, fmt.Errorf("payloadID is required")
	}
	uri := fmt.Sprintf("/v1/api/payloads/%s", url.PathEscape(payloadID))
	req, err := c.newRequest(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		b, _ := io.ReadAll(resp.Body)
		return nil, &HTTPError{StatusCode: resp.StatusCode, Status: resp.Status, Body: strings.TrimSpace(string(b))}
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	ct := resp.Header.Get("Content-Type")
	return &PayloadResponse{ContentType: ct, Body: body}, nil
}

// UploadAttachment uploads a file to /upload and returns its staging descriptor.
func (c *Client) UploadAttachment(ctx context.Context, name string, r io.Reader) (*UploadResponse, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("name is required")
	}
	if r == nil {
		return nil, fmt.Errorf("reader is required")
	}
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, r); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	req, err := c.newRequest(ctx, http.MethodPost, "/upload", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Body = io.NopCloser(body)
	req.ContentLength = int64(body.Len())
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		b, _ := io.ReadAll(resp.Body)
		return nil, &HTTPError{StatusCode: resp.StatusCode, Status: resp.Status, Body: strings.TrimSpace(string(b))}
	}
	var out UploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AuthProviders lists configured auth providers.
func (c *Client) AuthProviders(ctx context.Context) ([]AuthProvider, error) {
	var out []AuthProvider
	if err := c.doJSON(ctx, http.MethodGet, "/v1/api/auth/providers", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// AuthMe returns the current user profile (cookie or bearer).
func (c *Client) AuthMe(ctx context.Context) (map[string]interface{}, error) {
	var out map[string]interface{}
	if err := c.doJSON(ctx, http.MethodGet, "/v1/api/auth/me", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// AuthLogout clears the current session.
func (c *Client) AuthLogout(ctx context.Context) error {
	return c.doJSON(ctx, http.MethodPost, "/v1/api/auth/logout", map[string]string{}, nil)
}

// AuthLocalLogin sets a session cookie for local auth.
func (c *Client) AuthLocalLogin(ctx context.Context, name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("name is required")
	}
	return c.doJSON(ctx, http.MethodPost, "/v1/api/auth/local/login", &LocalLoginRequest{Name: name}, nil)
}

// AuthOAuthInitiate returns an auth URL for BFF OAuth flows.
func (c *Client) AuthOAuthInitiate(ctx context.Context) (*OAuthInitiateResponse, error) {
	var out OAuthInitiateResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v1/api/auth/oauth/initiate", map[string]string{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// NewMessageBuffer creates a message buffer for delta reconciliation.
func NewMessageBuffer() *MessageBuffer {
	return &MessageBuffer{ByMessageID: map[string]string{}}
}

// ApplyEvent merges a stream envelope into the buffer.
// It returns the message id and current text when it changed.
func (b *MessageBuffer) ApplyEvent(ev *StreamEventEnvelope) (string, string, bool) {
	if b == nil || ev == nil || ev.Message == nil {
		return "", "", false
	}
	msgID := strings.TrimSpace(ev.Message.Id)
	if msgID == "" {
		return "", "", false
	}
	text := b.ByMessageID[msgID]
	if ev.Content != nil {
		if delta, ok := ev.Content["delta"].(string); ok && delta != "" {
			text += delta
			b.ByMessageID[msgID] = text
			return msgID, text, true
		}
		if t, ok := ev.Content["text"].(string); ok && t != "" {
			text = t
			b.ByMessageID[msgID] = text
			return msgID, text, true
		}
	}
	// Fallback to message content when present.
	if ev.Message.Content != nil && strings.TrimSpace(*ev.Message.Content) != "" {
		text = strings.TrimSpace(*ev.Message.Content)
		b.ByMessageID[msgID] = text
		return msgID, text, true
	}
	return "", "", false
}

// ReconcileFromTranscript replaces buffered text with final assistant messages.
func (b *MessageBuffer) ReconcileFromTranscript(conv *conv.Conversation) {
	if b == nil || conv == nil || conv.Transcript == nil {
		return
	}
	for _, turn := range conv.Transcript {
		if turn == nil || turn.Message == nil {
			continue
		}
		for _, m := range turn.Message {
			if m == nil {
				continue
			}
			if strings.ToLower(strings.TrimSpace(m.Role)) != "assistant" {
				continue
			}
			if m.Content != nil && strings.TrimSpace(*m.Content) != "" {
				b.ByMessageID[m.Id] = strings.TrimSpace(*m.Content)
			}
		}
	}
}

func (c *Client) doJSON(ctx context.Context, method, uri string, in, out interface{}) error {
	attempts := c.retry.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		req, err := c.newRequest(ctx, method, uri, in)
		if err != nil {
			return err
		}
		if c.requestHook != nil {
			if err := c.requestHook(req); err != nil {
				return err
			}
		}
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			if !c.shouldRetry(method, 0, err) || attempt == attempts {
				return err
			}
			time.Sleep(c.retry.Delay)
			continue
		}
		if c.responseHook != nil {
			if err := c.responseHook(resp); err != nil {
				resp.Body.Close()
				return err
			}
		}
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			herr := &HTTPError{StatusCode: resp.StatusCode, Status: resp.Status, Body: strings.TrimSpace(string(b))}
			lastErr = herr
			if !c.shouldRetry(method, resp.StatusCode, herr) || attempt == attempts {
				return herr
			}
			time.Sleep(c.retry.Delay)
			continue
		}
		if out == nil {
			resp.Body.Close()
			return nil
		}
		dec := json.NewDecoder(resp.Body)
		err = dec.Decode(out)
		resp.Body.Close()
		if err != nil {
			return err
		}
		return nil
	}
	return lastErr
}

func (c *Client) newRequest(ctx context.Context, method, uri string, in interface{}) (*http.Request, error) {
	var body io.Reader
	if in != nil {
		buf := &bytes.Buffer{}
		if err := json.NewEncoder(buf).Encode(in); err != nil {
			return nil, err
		}
		body = buf
	}
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, err
	}
	rel, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}
	full := base.ResolveReference(rel)
	req, err := http.NewRequestWithContext(ctx, method, full.String(), body)
	if err != nil {
		return nil, err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	if c.tokenProvider != nil {
		tok, err := c.tokenProvider(ctx)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(tok) != "" {
			req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(tok))
		}
	}
	return req, nil
}

func (c *Client) shouldRetry(method string, status int, err error) bool {
	if c.retry.MaxAttempts <= 1 {
		return false
	}
	if _, ok := c.retry.RetryMethods[strings.ToUpper(method)]; !ok {
		return false
	}
	if status > 0 {
		_, ok := c.retry.RetryStatuses[status]
		return ok
	}
	if err != nil {
		return c.retry.RetryOnError
	}
	return false
}

func readSSE(ctx context.Context, r io.Reader, out chan<- *StreamEventEnvelope) error {
	buf := make([]byte, 0, 64*1024)
	tmp := make([]byte, 4096)
	var event string
	var data []byte
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		n, err := r.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			for {
				idx := bytes.Index(buf, []byte("\n"))
				if idx == -1 {
					break
				}
				line := string(bytes.TrimRight(buf[:idx], "\r"))
				buf = buf[idx+1:]
				if line == "" {
					if len(data) > 0 {
						var env StreamEventEnvelope
						if jerr := json.Unmarshal(data, &env); jerr == nil {
							out <- &env
						}
					}
					event = ""
					data = data[:0]
					continue
				}
				if strings.HasPrefix(line, "event:") {
					event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
					_ = event
					continue
				}
				if strings.HasPrefix(line, "data:") {
					payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
					data = append(data, []byte(payload)...)
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}
