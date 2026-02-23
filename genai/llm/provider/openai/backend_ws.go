package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/agently/genai/memory"
	mcbuf "github.com/viant/agently/genai/modelcallctx"
)

type backendWSState struct {
	mu             sync.Mutex
	conn           *websocket.Conn
	lastInput      []InputItem
	lastResponseID string
	turnState      string
}

var backendWSCache sync.Map    // key: baseURL+"|"+conversationID -> *backendWSState
var backendWSDisabled sync.Map // key: normalized baseURL -> disabledUntil (time.Time)

type backendWSCreateRequest struct {
	Type               string          `json:"type"`
	Model              string          `json:"model"`
	Instructions       string          `json:"instructions"`
	Input              []InputItem     `json:"input"`
	Tools              []ResponsesTool `json:"tools,omitempty"`
	ToolChoice         interface{}     `json:"tool_choice,omitempty"`
	ParallelToolCalls  bool            `json:"parallel_tool_calls,omitempty"`
	Reasoning          interface{}     `json:"reasoning,omitempty"`
	PreviousResponseID string          `json:"previous_response_id,omitempty"`
	Store              bool            `json:"store"`
	Stream             bool            `json:"stream"`
	Include            []string        `json:"include,omitempty"`
	PromptCacheKey     string          `json:"prompt_cache_key,omitempty"`
	Text               *TextControls   `json:"text,omitempty"`
}

func backendWebsocketEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("AGENTLY_OPENAI_BACKEND_WS")))
	return v != "0" && v != "false" && v != "off"
}

func backendWSKey(baseURL, conversationID string) string {
	return strings.TrimSpace(strings.ToLower(baseURL)) + "|" + strings.TrimSpace(conversationID)
}

func backendWSBaseKey(baseURL string) string {
	return strings.TrimSpace(strings.ToLower(baseURL))
}

func backendWSDisableTTL() time.Duration {
	v := strings.TrimSpace(os.Getenv("AGENTLY_OPENAI_BACKEND_WS_DISABLE_TTL"))
	if v == "" {
		return 30 * time.Minute
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 30 * time.Minute
	}
	return d
}

func backendWSDisableReason(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		return "unknown websocket error"
	}
	return msg
}

func shouldDisableBackendWS(err error) bool {
	if err == nil {
		return false
	}
	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) {
		// Policy violation / unsupported data / mandatory extension.
		if closeErr.Code == websocket.ClosePolicyViolation || closeErr.Code == websocket.CloseUnsupportedData || closeErr.Code == websocket.CloseMandatoryExtension {
			return true
		}
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "policy violation") || strings.Contains(msg, "websocket: bad handshake")
}

func markBackendWSDisabled(baseURL string, err error, c *Client) {
	if strings.TrimSpace(baseURL) == "" {
		return
	}
	until := time.Now().Add(backendWSDisableTTL())
	backendWSDisabled.Store(backendWSBaseKey(baseURL), until)
	if c != nil {
		c.logf("[openai-ws] backend websocket disabled until=%s reason=%s", until.Format(time.RFC3339), backendWSDisableReason(err))
	}
}

func isBackendWSDisabled(baseURL string) (bool, time.Time) {
	key := backendWSBaseKey(baseURL)
	if key == "" {
		return false, time.Time{}
	}
	v, ok := backendWSDisabled.Load(key)
	if !ok || v == nil {
		return false, time.Time{}
	}
	until, ok := v.(time.Time)
	if !ok || until.IsZero() {
		backendWSDisabled.Delete(key)
		return false, time.Time{}
	}
	if time.Now().After(until) {
		backendWSDisabled.Delete(key)
		return false, time.Time{}
	}
	return true, until
}

func getBackendWSState(baseURL, conversationID string) *backendWSState {
	key := backendWSKey(baseURL, conversationID)
	if v, ok := backendWSCache.Load(key); ok {
		if s, ok := v.(*backendWSState); ok && s != nil {
			return s
		}
	}
	s := &backendWSState{}
	actual, _ := backendWSCache.LoadOrStore(key, s)
	if out, ok := actual.(*backendWSState); ok && out != nil {
		return out
	}
	return s
}

func buildBackendWSURL(baseURL string) (string, error) {
	base := strings.TrimSpace(baseURL)
	if base == "" {
		return "", fmt.Errorf("base URL was empty")
	}
	u, err := url.Parse(strings.TrimRight(base, "/") + "/responses")
	if err != nil {
		return "", err
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	}
	return u.String(), nil
}

func hasInputPrefix(full []InputItem, prefix []InputItem) bool {
	if len(prefix) == 0 || len(prefix) > len(full) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		if !reflect.DeepEqual(full[i], prefix[i]) {
			return false
		}
	}
	return true
}

func (c *Client) backendWSDial(ctx context.Context, conversationID string, turnState string) (*websocket.Conn, string, error) {
	apiKey, err := c.apiKey(ctx)
	if err != nil {
		return nil, "", err
	}
	wsURL, err := buildBackendWSURL(c.BaseURL)
	if err != nil {
		return nil, "", err
	}
	h := http.Header{}
	h.Set("Authorization", "Bearer "+apiKey)
	// Codex websocket contract: advertise responses websocket protocol version(s).
	h.Add("OpenAI-Beta", "responses_websockets=2026-02-04")
	h.Add("OpenAI-Beta", "responses_websockets=2026-02-06")
	if ua := c.userAgentOverride(); ua != "" {
		h.Set("User-Agent", ua)
	}
	if originator := c.originatorHeader(); originator != "" {
		h.Set("originator", originator)
	}
	if features := c.codexBetaFeaturesHeader(); features != "" {
		h.Set("x-codex-beta-features", features)
	}
	if ts := strings.TrimSpace(turnState); ts != "" {
		h.Set("x-codex-turn-state", ts)
	}
	if strings.TrimSpace(conversationID) != "" {
		h.Set("session_id", strings.TrimSpace(conversationID))
	}
	if accountID, err := c.chatGPTAccountID(ctx); err == nil && strings.TrimSpace(accountID) != "" {
		h.Set("ChatGPT-Account-Id", strings.TrimSpace(accountID))
	}
	d := websocket.Dialer{HandshakeTimeout: 30 * time.Second, EnableCompression: true}
	conn, resp, err := d.DialContext(ctx, wsURL, h)
	if err != nil {
		return nil, "", err
	}
	_ = conn.SetReadDeadline(time.Time{})
	nextTurnState := ""
	if resp != nil {
		nextTurnState = strings.TrimSpace(resp.Header.Get("x-codex-turn-state"))
	}
	return conn, nextTurnState, nil
}

func backendEventType(raw []byte) string {
	var ev struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(raw, &ev) == nil {
		return strings.TrimSpace(ev.Type)
	}
	return ""
}

func isBackendTerminalEvent(eventType string) bool {
	return eventType == "response.completed" || eventType == "response.failed" || eventType == "error"
}

func (c *Client) streamViaBackendWebSocket(ctx context.Context, req *Request, orig *llm.GenerateRequest, events chan<- llm.StreamEvent) error {
	conversationID := strings.TrimSpace(memory.ConversationIDFromContext(ctx))
	if conversationID == "" {
		return fmt.Errorf("backend websocket requires conversation id in context")
	}
	state := getBackendWSState(c.BaseURL, conversationID)
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.conn == nil {
		conn, nextTurnState, err := c.backendWSDial(ctx, conversationID, state.turnState)
		if err != nil {
			return err
		}
		state.conn = conn
		if nextTurnState != "" {
			state.turnState = nextTurnState
		}
	}

	payload := ToChatGPTBackendResponsesPayload(req)
	create := backendWSCreateRequest{
		Type:              "response.create",
		Model:             payload.Model,
		Instructions:      payload.Instructions,
		Input:             payload.Input,
		Tools:             payload.Tools,
		ToolChoice:        payload.ToolChoice,
		ParallelToolCalls: payload.ParallelToolCalls,
		Reasoning:         payload.Reasoning,
		Store:             payload.Store,
		Stream:            true,
		Include:           payload.Include,
		PromptCacheKey:    payload.PromptCacheKey,
		Text:              payload.Text,
	}

	// Incremental parity: reuse previous response when current input is a strict append.
	if strings.TrimSpace(state.lastResponseID) != "" && hasInputPrefix(payload.Input, state.lastInput) && len(payload.Input) > len(state.lastInput) {
		create.PreviousResponseID = strings.TrimSpace(state.lastResponseID)
		create.Input = payload.Input[len(state.lastInput):]
	}

	body, err := json.Marshal(create)
	if err != nil {
		return err
	}
	if err := state.conn.WriteMessage(websocket.TextMessage, body); err != nil {
		_ = state.conn.Close()
		state.conn = nil
		return err
	}
	state.lastInput = payload.Input

	observer := mcbuf.ObserverFromContext(ctx)
	p := &streamProcessor{
		client:   c,
		ctx:      ctx,
		observer: observer,
		events:   events,
		agg:      newStreamAggregator(),
		state:    &streamState{emittedToolCallIDs: map[string]struct{}{}, emittedToolCallArgs: map[string]string{}},
		req:      req,
		orig:     orig,
	}

	for {
		mt, raw, err := state.conn.ReadMessage()
		if err != nil {
			_ = state.conn.Close()
			state.conn = nil
			return err
		}
		if mt != websocket.TextMessage && mt != websocket.BinaryMessage {
			continue
		}
		eventType := backendEventType(raw)
		if eventType == "response.created" || eventType == "response.in_progress" {
			var e struct {
				Response struct {
					ID string `json:"id"`
				} `json:"response"`
			}
			if json.Unmarshal(raw, &e) == nil && strings.TrimSpace(e.Response.ID) != "" {
				state.lastResponseID = strings.TrimSpace(e.Response.ID)
			}
		}
		if !p.handleEvent(eventType, string(raw)) {
			p.finalize(nil)
			return nil
		}
		if isBackendTerminalEvent(eventType) {
			p.finalize(nil)
			return nil
		}
	}
}
