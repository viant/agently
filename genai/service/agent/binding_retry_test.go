package agent

import (
    "context"
    "errors"
    "testing"
    apiconv "github.com/viant/agently/client/conversation"
    "github.com/stretchr/testify/assert"
)

// stubConv implements apiconv.Client for testing retry behavior.
type stubConv struct{
    seq []error
    idx int
    result *apiconv.Conversation
}

func (s *stubConv) nextErr() error {
    if s.idx >= len(s.seq) { return nil }
    e := s.seq[s.idx]
    s.idx++
    return e
}

func (s *stubConv) GetConversation(ctx context.Context, id string, options ...apiconv.Option) (*apiconv.Conversation, error) {
    if err := s.nextErr(); err != nil { return nil, err }
    return s.result, nil
}

// Unused interface members for this test
func (s *stubConv) GetConversations(context.Context, *apiconv.Input) ([]*apiconv.Conversation, error) { return nil, nil }
func (s *stubConv) PatchConversations(context.Context, *apiconv.MutableConversation) error { return nil }
func (s *stubConv) GetPayload(context.Context, string) (*apiconv.Payload, error) { return nil, nil }
func (s *stubConv) PatchPayload(context.Context, *apiconv.MutablePayload) error { return nil }
func (s *stubConv) PatchMessage(context.Context, *apiconv.MutableMessage) error { return nil }
func (s *stubConv) GetMessage(context.Context, string, ...apiconv.Option) (*apiconv.Message, error) { return nil, nil }
func (s *stubConv) GetMessageByElicitation(context.Context, string, string) (*apiconv.Message, error) { return nil, nil }
func (s *stubConv) PatchModelCall(context.Context, *apiconv.MutableModelCall) error { return nil }
func (s *stubConv) PatchToolCall(context.Context, *apiconv.MutableToolCall) error { return nil }
func (s *stubConv) PatchTurn(context.Context, *apiconv.MutableTurn) error { return nil }
func (s *stubConv) DeleteConversation(context.Context, string) error { return nil }
func (s *stubConv) DeleteMessage(context.Context, string, string) error { return nil }

func TestFetchConversationWithRetry(t *testing.T) {
    transient := errors.New("i/o timeout: temporary network error")
    fatal := errors.New("permission denied")

    cases := []struct{
        name string
        seq  []error
        result *apiconv.Conversation
        wantErr bool
    }{
        {name: "success-first", seq: nil, result: &apiconv.Conversation{Id: "A"}},
        {name: "retry-then-success", seq: []error{transient, transient}, result: &apiconv.Conversation{Id: "B"}},
        {name: "fatal-no-retry", seq: []error{fatal}, result: nil, wantErr: true},
        {name: "nil-conversation", seq: nil, result: nil, wantErr: true},
    }

    for _, tc := range cases {
        svc := &Service{}
        svc.conversation = &stubConv{seq: tc.seq, result: tc.result}
        got, err := svc.fetchConversationWithRetry(context.Background(), "X")
        if tc.wantErr {
            assert.NotNil(t, err, tc.name)
            continue
        }
        assert.Nil(t, err, tc.name)
        assert.EqualValues(t, tc.result, got, tc.name)
    }
}

