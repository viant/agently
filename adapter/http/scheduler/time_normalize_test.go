package scheduler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestNormalizeRFC3339MissingSeconds(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "offset without seconds",
			in:   "2026-01-01T00:00+01:00",
			want: "2026-01-01T00:00:00+01:00",
		},
		{
			name: "utc without seconds",
			in:   "2026-01-01T00:00Z",
			want: "2026-01-01T00:00:00Z",
		},
		{
			name: "already has seconds",
			in:   "2026-01-01T00:00:00+01:00",
			want: "2026-01-01T00:00:00+01:00",
		},
		{
			name: "date only",
			in:   "2026-01-01",
			want: "2026-01-01",
		},
		{
			name: "non time",
			in:   "hello",
			want: "hello",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeRFC3339MissingSeconds(tt.in); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeJSONTimeFields_ScopedByKey(t *testing.T) {
	payload := map[string]any{
		"data": []any{
			map[string]any{
				"startAt":     "2026-01-01T00:00+01:00",
				"endAt":       "2026-01-02T00:00Z",
				"taskPrompt":  "mention 2026-01-01T00:00+01:00 should stay",
				"createdAt":   "2026-01-01T00:00+01:00",
				"updatedAt":   "2026-01-01T00:00:00+01:00",
				"notATimeKey": "2026-01-01T00:00+01:00",
			},
		},
	}

	normalized, changed := normalizeJSONTimeFields("", payload)
	if !changed {
		t.Fatalf("expected payload to change")
	}

	root, ok := normalized.(map[string]any)
	if !ok {
		t.Fatalf("expected map payload, got %T", normalized)
	}
	rows, ok := root["data"].([]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("expected data array of 1, got %T len=%d", root["data"], len(rows))
	}
	row, ok := rows[0].(map[string]any)
	if !ok {
		t.Fatalf("expected data[0] map, got %T", rows[0])
	}

	if row["startAt"] != "2026-01-01T00:00:00+01:00" {
		t.Fatalf("startAt not normalized: %v", row["startAt"])
	}
	if row["endAt"] != "2026-01-02T00:00:00Z" {
		t.Fatalf("endAt not normalized: %v", row["endAt"])
	}
	if row["createdAt"] != "2026-01-01T00:00:00+01:00" {
		t.Fatalf("createdAt not normalized: %v", row["createdAt"])
	}
	// Should remain unchanged (already has seconds).
	if row["updatedAt"] != "2026-01-01T00:00:00+01:00" {
		t.Fatalf("updatedAt should be unchanged: %v", row["updatedAt"])
	}
	// Should not be touched because the key isn't time-related.
	if row["taskPrompt"] != "mention 2026-01-01T00:00+01:00 should stay" {
		t.Fatalf("taskPrompt should be unchanged: %v", row["taskPrompt"])
	}
	if row["notATimeKey"] != "2026-01-01T00:00+01:00" {
		t.Fatalf("notATimeKey should be unchanged: %v", row["notATimeKey"])
	}
}

func TestNormalizeSchedulerWriteTimeFields_RewritesBody(t *testing.T) {
	input := map[string]any{
		"data": []any{
			map[string]any{
				"id":      "sched-1",
				"startAt": "2026-01-01T00:00+01:00",
			},
		},
	}
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	req, err := http.NewRequest(http.MethodPatch, "http://example.com/v1/api/agently/schedule", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if err := normalizeSchedulerWriteTimeFields(req); err != nil {
		t.Fatalf("normalize: %v", err)
	}

	outBody, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read normalized body: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(outBody, &out); err != nil {
		t.Fatalf("unmarshal normalized body: %v\nbody=%s", err, string(outBody))
	}

	data, ok := out["data"].([]any)
	if !ok || len(data) != 1 {
		t.Fatalf("expected data array, got %T len=%d", out["data"], len(data))
	}
	row, ok := data[0].(map[string]any)
	if !ok {
		t.Fatalf("expected data[0] map, got %T", data[0])
	}
	if row["startAt"] != "2026-01-01T00:00:00+01:00" {
		t.Fatalf("startAt not normalized: %v", row["startAt"])
	}
}
