package memory

import (
   "context"
   "fmt"
   "testing"

   "github.com/stretchr/testify/assert"
)

func TestSummaryPolicy(t *testing.T) {
   // Create 6 messages m1..m6
   msgs := make([]Message, 6)
   for i := 0; i < 6; i++ {
       msgs[i] = Message{Role: "user", Content: fmt.Sprintf("m%d", i+1)}
   }
   // Fake summarizer returns constant summary
   summarizer := func(ctx context.Context, in []Message) (Message, error) {
       // Expect summarizer to receive first 3 messages
       assert.Equal(t, 3, len(in))
       return Message{Role: "system", Content: "summary"}, nil
   }
   // Keep last 3 messages after summarization
   policy := NewSummaryPolicy(3, summarizer)
   out, err := policy.Apply(context.Background(), msgs)
   assert.NoError(t, err)
   // Expect output: [summary, m4, m5, m6]
   expected := []Message{
       {Role: "system", Content: "summary"},
       {Role: "user", Content: "m4"},
       {Role: "user", Content: "m5"},
       {Role: "user", Content: "m6"},
   }
   assert.EqualValues(t, expected, out)
}

func TestNextTokenPolicy(t *testing.T) {
   // Messages with varying content lengths
   msgs := []Message{
       {Role: "user", Content: "aaaaa"},    // length 5
       {Role: "user", Content: "bbbbbb"},   // length 6
       {Role: "user", Content: "ccccccc"},  // length 7
       {Role: "user", Content: "dddddddd"}, // length 8
       {Role: "user", Content: "eeeeeeeee"},// length 9
   }
   threshold := 20
   keep := 2
   // Summarizer should receive first len(msgs)-keep messages
   summarizer := func(ctx context.Context, in []Message) (Message, error) {
       expectedCount := len(msgs) - keep
       if len(in) != expectedCount {
           t.Errorf("expected %d messages to summarize, got %d", expectedCount, len(in))
       }
       return Message{Role: "system", Content: "SUMMARY"}, nil
   }
   // Estimator counts each character as 1 token
   estimator := func(m Message) int {
       return len(m.Content)
   }
   policy := NewNextTokenPolicy(threshold, keep, summarizer, estimator)
   out, err := policy.Apply(context.Background(), msgs)
   assert.NoError(t, err)
   // After summarization, expect [SUMMARY, msgs[3], msgs[4]]
   expected := []Message{
       {Role: "system", Content: "SUMMARY"},
       msgs[3],
       msgs[4],
   }
   assert.EqualValues(t, expected, out)
}