package scheduler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// normalizeSchedulerWriteTimeFields rewrites scheduler write payloads in-place to
// include seconds in RFC3339 timestamps when missing.
//
// Example: 2026-01-01T00:00+01:00 -> 2026-01-01T00:00:00+01:00
func normalizeSchedulerWriteTimeFields(r *http.Request) error {
	if r == nil || r.URL == nil {
		return nil
	}
	if r.Method != http.MethodPatch {
		return nil
	}
	switch r.URL.Path {
	case "/v1/api/agently/schedule", "/v1/api/agently/schedule-run":
		// continue
	default:
		return nil
	}
	if r.Body == nil {
		return nil
	}
	if ct := strings.ToLower(r.Header.Get("Content-Type")); ct != "" && !strings.Contains(ct, "application/json") {
		return nil
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	defer func() {
		// Ensure body is always reset for downstream handlers, even if JSON decoding fails.
		if r.Body == nil {
			r.Body = io.NopCloser(bytes.NewReader(body))
			r.ContentLength = int64(len(body))
		}
	}()

	if len(bytes.TrimSpace(body)) == 0 {
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
		return nil
	}

	var payload any
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&payload); err != nil {
		// Restore original body and let Datly handle validation/error reporting.
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
		return nil
	}

	normalized, changed := normalizeJSONTimeFields("", payload)
	if !changed {
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
		return nil
	}

	encoded, err := json.Marshal(normalized)
	if err != nil {
		// Fallback: restore original body.
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
		return nil
	}
	r.Body = io.NopCloser(bytes.NewReader(encoded))
	r.ContentLength = int64(len(encoded))
	return nil
}

func normalizeJSONTimeFields(key string, value any) (any, bool) {
	switch actual := value.(type) {
	case map[string]any:
		changed := false
		for k, v := range actual {
			nv, ok := normalizeJSONTimeFields(k, v)
			if ok {
				actual[k] = nv
				changed = true
			}
		}
		return actual, changed
	case []any:
		changed := false
		for i, v := range actual {
			nv, ok := normalizeJSONTimeFields("", v)
			if ok {
				actual[i] = nv
				changed = true
			}
		}
		return actual, changed
	case string:
		if !isSchedulerTimeKey(key) {
			return actual, false
		}
		normalized := normalizeRFC3339MissingSeconds(actual)
		if normalized == actual {
			return actual, false
		}
		return normalized, true
	default:
		return value, false
	}
}

func isSchedulerTimeKey(key string) bool {
	if key == "" {
		return false
	}
	norm := strings.ToLower(key)
	norm = strings.ReplaceAll(norm, "_", "")
	norm = strings.ReplaceAll(norm, "-", "")
	_, ok := schedulerTimeKeys[norm]
	return ok
}

var schedulerTimeKeys = map[string]struct{}{
	// schedule
	"startat":    {},
	"endat":      {},
	"nextrunat":  {},
	"lastrunat":  {},
	"leaseuntil": {},
	"createdat":  {},
	"updatedat":  {},

	// schedule_run
	"scheduledfor":      {},
	"startedat":         {},
	"completedat":       {},
	"preconditionranat": {},
}

func normalizeRFC3339MissingSeconds(value string) string {
	// Fast-path: expect YYYY-MM-DD(T| )HH:MM...
	if len(value) < len("2006-01-02T15:04") {
		return value
	}
	// Minimal date validation (avoid touching arbitrary strings).
	if value[4] != '-' || value[7] != '-' {
		return value
	}
	sep := value[10]
	if sep != 'T' && sep != ' ' {
		return value
	}
	// Validate "HH:MM" portion.
	if len(value) < 16 || value[13] != ':' {
		return value
	}

	// If there's already seconds (HH:MM:SS...), keep as-is.
	if len(value) > 16 && value[16] == ':' {
		return value
	}

	// Insert seconds when the next token indicates timezone/end/fractional seconds.
	if len(value) == 16 {
		return value + ":00"
	}
	switch value[16] {
	case 'Z', '+', '-', '.':
		return value[:16] + ":00" + value[16:]
	default:
		return value
	}
}
