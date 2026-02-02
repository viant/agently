package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClientPollEvents(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/api/conversations/c1/events", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		payload := map[string]interface{}{
			"events": []map[string]interface{}{
				{
					"seq":            3,
					"time":           time.Now().UTC().Format(time.RFC3339Nano),
					"conversationId": "c1",
					"message": map[string]interface{}{
						"Id":   "m3",
						"Role": "assistant",
						"Type": "text",
					},
				},
			},
			"since": "3",
		}
		require.NoError(t, json.NewEncoder(w).Encode(payload))
	}

	srv := httptest.NewServer(http.HandlerFunc(handler))
	defer srv.Close()

	client := New(srv.URL)
	resp, err := client.PollEvents(context.Background(), "c1", "2", nil, 0)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, resp.Events, 1)
	require.Equal(t, uint64(3), resp.Events[0].Seq)
	require.NotNil(t, resp.Events[0].Message)
	require.Equal(t, "m3", resp.Events[0].Message.Id)
	require.Equal(t, "3", resp.Since)
}

func TestClientStreamEvents(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/api/conversations/c1/events", r.URL.Path)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		payload := map[string]interface{}{
			"seq":            1,
			"time":           time.Now().UTC().Format(time.RFC3339Nano),
			"conversationId": "c1",
			"message": map[string]interface{}{
				"Id":      "m1",
				"Role":    "assistant",
				"Type":    "text",
				"Interim": 1,
			},
		}
		raw, err := json.Marshal(payload)
		require.NoError(t, err)
		_, _ = w.Write([]byte("event: interim_message\n"))
		_, _ = w.Write([]byte("data: "))
		_, _ = w.Write(raw)
		_, _ = w.Write([]byte("\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}

	srv := httptest.NewServer(http.HandlerFunc(handler))
	defer srv.Close()

	client := New(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	events, errs, err := client.StreamEvents(ctx, "c1", "", nil)
	require.NoError(t, err)
	select {
	case ev := <-events:
		require.NotNil(t, ev)
		require.Equal(t, uint64(1), ev.Seq)
		require.NotNil(t, ev.Message)
		require.Equal(t, "m1", ev.Message.Id)
	case e := <-errs:
		t.Fatalf("stream error: %v", e)
	case <-ctx.Done():
		t.Fatalf("timeout waiting for event")
	}
}

func TestClientStreamEventsDelta(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/api/conversations/c1/events", r.URL.Path)
		require.Equal(t, "since=2", r.URL.RawQuery)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		payload := map[string]interface{}{
			"seq":            0,
			"time":           time.Now().UTC().Format(time.RFC3339Nano),
			"conversationId": "c1",
			"message": map[string]interface{}{
				"Id":      "m1",
				"Role":    "assistant",
				"Type":    "text",
				"Interim": 1,
			},
			"contentType": "application/json",
			"content": map[string]interface{}{
				"delta": "hello",
			},
		}
		raw, err := json.Marshal(payload)
		require.NoError(t, err)
		_, _ = w.Write([]byte("event: interim_message\n"))
		_, _ = w.Write([]byte("data: "))
		_, _ = w.Write(raw)
		_, _ = w.Write([]byte("\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}

	srv := httptest.NewServer(http.HandlerFunc(handler))
	defer srv.Close()

	client := New(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	events, errs, err := client.StreamEvents(ctx, "c1", "2", nil)
	require.NoError(t, err)
	select {
	case ev := <-events:
		require.NotNil(t, ev)
		require.Equal(t, uint64(0), ev.Seq)
		require.NotNil(t, ev.Message)
		require.Equal(t, "m1", ev.Message.Id)
		require.Equal(t, "application/json", ev.ContentType)
		require.NotNil(t, ev.Content)
	case e := <-errs:
		t.Fatalf("stream error: %v", e)
	case <-ctx.Done():
		t.Fatalf("timeout waiting for event")
	}
}
