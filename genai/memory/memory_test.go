package memory

import (
   "context"
   "testing"

   "github.com/stretchr/testify/assert"
)

// fakeEmbedder maps input texts to predefined vectors for testing.
type fakeEmbedder struct{
   embeddings map[string][]float32
}

// Embed returns vectors corresponding to the input texts.
func (f *fakeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
   result := make([][]float32, len(texts))
   for i, t := range texts {
       if v, ok := f.embeddings[t]; ok {
           result[i] = v
       } else {
           // default zero vector of length 2
           result[i] = make([]float32, 2)
       }
   }
   return result, nil
}

func TestInMemoryStore_Save_List_Query(t *testing.T) {
   // Prepare fake embeddings
   embedMap := map[string][]float32{
       "hello": {1, 0},
       "world": {0, 1},
       "foo":   {1, 1},
   }
   fake := &fakeEmbedder{embeddings: embedMap}
   store := NewInMemoryStore(fake.Embed)

   ctx := context.Background()
   convID := "conv1"
   msgs := []Message{
       {Role: "user", Content: "hello"},
       {Role: "assistant", Content: "foo"},
       {Role: "user", Content: "world"},
   }
   // Save messages
   for _, msg := range msgs {
       err := store.Save(ctx, convID, msg)
       assert.NoError(t, err)
   }

   // Test List returns in insertion order
   listed := store.List(ctx, convID)
   assert.EqualValues(t, msgs, listed)

   // Query top 2 relevant to "world"
   res, err := store.Query(ctx, convID, "world", 2)
   assert.NoError(t, err)
   // expected order: exact match "world", then next most similar "foo"
   expected := []Message{msgs[2], msgs[1]}
   assert.EqualValues(t, expected, res)
}