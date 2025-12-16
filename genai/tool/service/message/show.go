package message

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	sed "github.com/rwtodd/Go.Sed/sed"
	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/textclip"
	"github.com/viant/mcp-protocol/extension"
)

type ShowInput struct {
	MessageID string             `json:"messageId"`
	ByteRange *textclip.IntRange `json:"byteRange,omitempty" description:"Optional byte range [from,to) over the selected content."`
	Sed       []string           `json:"sed,omitempty" description:"List of sed programs applied in order to the selected content."`
	Transform *TransformSpec     `json:"transform,omitempty" description:"Transform with selector+fields or queryLanguage+query, then format as csv or ndjson."`
}

type ShowOutput struct {
	Content string `json:"content"`
	Offset  int    `json:"offset"`
	Limit   int    `json:"limit"`
	Size    int    `json:"size"`
	// Continuation carries paging/truncation hints when only part of the message is returned.
	Continuation *extension.Continuation `json:"continuation,omitempty"`
}

type DebugInfo struct {
	Source     string `json:"source,omitempty"`
	RawLen     int    `json:"rawLen,omitempty"`
	RawPreview string `json:"rawPreview,omitempty"`
}

type TransformSpec struct {
	Selector string   `json:"selector,omitempty" description:"Simple dot-path (e.g., data or data.items). No wildcards or object construction."`
	Format   string   `json:"format,omitempty" description:"Output format" choice:"csv" choice:"ndjson"`
	Fields   []string `json:"fields,omitempty" description:"CSV column order; derived when empty."`
	MaxRows  int      `json:"maxRows,omitempty" description:"Row cap (0 = no cap)."`
}

func (s *Service) show(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*ShowInput)
	if !ok {
		return fmt.Errorf("invalid input")
	}
	output, ok := out.(*ShowOutput)
	if !ok {
		return fmt.Errorf("invalid output")
	}

	msg, err := s.conv.GetMessage(ctx, input.MessageID, apiconv.WithIncludeToolCall(true))
	if err != nil {
		return fmt.Errorf("failed to get message: %v %v", input.MessageID, err)
	}

	// Prefer original tool-call payload inline body when available.
	var result []byte
	if alt, _ := preferToolPayload(msg, ""); len(alt) > 0 {
		result = alt
	} else {
		result = []byte(msg.GetContentPreferContent())
	}
	// 1) Optional transform on the full raw body first
	if input.Transform != nil && strings.TrimSpace(input.Transform.Format) != "" {
		// Attach transient debug info about the transform input
		if result, _, err = applyTransform(result, input.Transform); err != nil {
			return fmt.Errorf("failed to apply transform: %v", err)
		}
	}
	size := len(result)
	// 2) Optional slicing by byte range; if not provided, return full content
	clipped, start, end, err := clipWithOffsets(result, input)
	if err != nil {
		return err
	}

	// 3) Optional sed programs (in order)
	transformed, err := applySedAll(clipped, toSedList(*input))
	if err != nil {
		return err
	}

	output.Content = string(transformed)
	output.Offset = start
	output.Limit = end - start
	output.Size = size
	if end < size {
		remaining := size - end
		nextOffset := end

		// keep paging in same chunk size when possible
		nextLength := output.Limit
		if nextLength <= 0 {
			nextLength = remaining
		}
		if nextLength > remaining {
			nextLength = remaining
		}

		output.Continuation = &extension.Continuation{
			HasMore:   true,
			Remaining: remaining,    // decreases each page
			Returned:  output.Limit, // bytes returned this call
			NextRange: &extension.RangeHint{
				Bytes: &extension.ByteRange{
					Offset: nextOffset,
					Length: nextLength,
				},
			},
		}
	}
	return nil
}

func clipWithOffsets(b []byte, in *ShowInput) ([]byte, int, int, error) {
	if in.ByteRange != nil {
		return textclip.ClipBytes(b, in.ByteRange)
	}
	return b, 0, len(b), nil
}

func toSedList(in ShowInput) []string {
	if len(in.Sed) > 0 {
		return in.Sed
	}
	return nil
}

func applySedAll(b []byte, scripts []string) ([]byte, error) {
	if len(scripts) == 0 {
		return b, nil
	}
	r := io.Reader(bytes.NewReader(b))
	for _, sc := range scripts {
		sc = strings.TrimSpace(sc)
		if sc == "" {
			continue
		}
		eng, err := sed.New(strings.NewReader(sc))
		if err != nil {
			return nil, fmt.Errorf("invalid sed program %q: %w", sc, err)
		}
		r = eng.Wrap(r)
	}
	return io.ReadAll(r)
}

func applyTransform(raw []byte, spec *TransformSpec) ([]byte, string, error) {
	if spec == nil || strings.TrimSpace(spec.Format) == "" {
		return raw, "text/plain", nil
	}
	var root interface{}
	// First attempt: parse as-is
	if err := json.Unmarshal(raw, &root); err != nil {
		// Fallback: extract the first complete JSON object/array from noisy content
		if sliced, ok := extractFirstJSON(raw); ok {
			if err2 := json.Unmarshal(sliced, &root); err2 != nil {
				return nil, "", fmt.Errorf("transform: invalid JSON: %w", err2)
			}
		} else {
			return nil, "", fmt.Errorf("transform: invalid JSON: %w", err)
		}
	}
	sel := strings.TrimSpace(spec.Selector)
	node := root
	if sel != "" {
		if containsNonDotPathSyntax(sel) {
			return nil, "", fmt.Errorf("transform: selector supports dot-path only (e.g., data or data.items)")
		}
		parts := strings.Split(sel, ".")
		for _, p := range parts {
			m, ok := node.(map[string]interface{})
			if !ok {
				return nil, "", fmt.Errorf("transform: selector not found")
			}
			v, ok := m[p]
			if !ok {
				return nil, "", fmt.Errorf("transform: selector not found")
			}
			node = v
		}
	}
	switch strings.ToLower(strings.TrimSpace(spec.Format)) {
	case "csv":
		return toCSV(node, spec)
	case "ndjson":
		return toNDJSON(node, spec)
	default:
		return nil, "", fmt.Errorf("transform: unsupported format %q (supported: csv, ndjson)", spec.Format)
	}
}

// extractFirstJSON scans the input for the first complete JSON object or array
// and returns the corresponding slice. It tolerates leading/trailing noise such as
// labels (e.g., "payload:") or code fences. Strings and escapes are respected.
func extractFirstJSON(b []byte) ([]byte, bool) {
	inString := false
	escape := false
	depth := 0
	start := -1
	for i, c := range b {
		if inString {
			if escape {
				escape = false
				continue
			}
			if c == '\\' {
				escape = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case ' ', '\t', '\r', '\n':
			// skip whitespace outside strings
			continue
		case '"':
			inString = true
			if depth == 0 { // strings before JSON start are noise
				continue
			}
		case '{', '[':
			if depth == 0 {
				start = i
			}
			depth++
		case '}', ']':
			if depth > 0 {
				depth--
			}
			if depth == 0 && start >= 0 {
				return b[start : i+1], true
			}
		default:
			// other characters before JSON start are ignored
		}
	}
	return nil, false
}

// preferToolPayload returns a plausible JSON body from the tool-call payloads when
// the message textual content isn't suitable for JSON parsing. Priority:
// 1) Response payload inline body (if different from message content)
// 2) Request payload inline body
// 3) Parsed ToolCallArguments marshaled to JSON
func preferToolPayload(m *apiconv.Message, used string) ([]byte, string) {
	if m == nil || m.ToolCall == nil {
		return nil, ""
	}
	if m.ToolCall.ResponsePayload != nil && m.ToolCall.ResponsePayload.InlineBody != nil {
		body := strings.TrimSpace(*m.ToolCall.ResponsePayload.InlineBody)
		if body != "" && body != used {
			return []byte(body), "payload.response"
		}
	}
	if m.ToolCall.RequestPayload != nil && m.ToolCall.RequestPayload.InlineBody != nil {
		body := strings.TrimSpace(*m.ToolCall.RequestPayload.InlineBody)
		if body != "" {
			return []byte(body), "payload.request"
		}
	}
	args := m.ToolCallArguments()
	if len(args) > 0 {
		if b, err := json.Marshal(args); err == nil {
			return b, "arguments"
		}
	}
	return nil, ""
}

// removed resolveMessageWithToolCall as GetMessage supports options

// preview returns a clipped string preview for debugging.
func preview(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}

func containsNonDotPathSyntax(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if strings.HasPrefix(s, "$") {
		return true
	}
	if strings.ContainsAny(s, "[]{}*") {
		return true
	}
	if strings.Contains(s, "||") || strings.Contains(s, "&&") {
		return true
	}
	return false
}

func toCSV(node interface{}, spec *TransformSpec) ([]byte, string, error) {
	rows, isArray := node.([]interface{})
	if !isArray {
		if obj, ok := node.(map[string]interface{}); ok {
			fields := spec.Fields
			if len(fields) == 0 {
				for k := range obj {
					fields = append(fields, k)
				}
			}
			buf := &bytes.Buffer{}
			w := csv.NewWriter(buf)
			_ = w.Write(fields)
			rec := make([]string, len(fields))
			for i, f := range fields {
				rec[i] = stringify(obj[f])
			}
			_ = w.Write(rec)
			w.Flush()
			return buf.Bytes(), "text/csv", w.Error()
		}
		buf := &bytes.Buffer{}
		w := csv.NewWriter(buf)
		_ = w.Write([]string{"value"})
		_ = w.Write([]string{stringify(node)})
		w.Flush()
		return buf.Bytes(), "text/csv", w.Error()
	}
	fields := spec.Fields
	if len(fields) == 0 {
		seen := map[string]bool{}
		for i, it := range rows {
			if i >= 100 {
				break
			}
			if obj, ok := it.(map[string]interface{}); ok {
				for k := range obj {
					if !seen[k] {
						seen[k] = true
						fields = append(fields, k)
					}
				}
			}
		}
		if len(fields) == 0 {
			fields = []string{"value"}
		}
	}
	buf := &bytes.Buffer{}
	w := csv.NewWriter(buf)
	_ = w.Write(fields)
	max := len(rows)
	if spec.MaxRows > 0 && spec.MaxRows < max {
		max = spec.MaxRows
	}
	for i := 0; i < max; i++ {
		it := rows[i]
		if obj, ok := it.(map[string]interface{}); ok {
			rec := make([]string, len(fields))
			for j, f := range fields {
				rec[j] = stringify(obj[f])
			}
			_ = w.Write(rec)
			continue
		}
		rec := make([]string, len(fields))
		if len(fields) > 0 {
			rec[0] = stringify(it)
		}
		_ = w.Write(rec)
	}
	w.Flush()
	return buf.Bytes(), "text/csv", w.Error()
}

func toNDJSON(node interface{}, spec *TransformSpec) ([]byte, string, error) {
	var buf bytes.Buffer
	rows, isArray := node.([]interface{})
	if isArray {
		max := len(rows)
		if spec.MaxRows > 0 && spec.MaxRows < max {
			max = spec.MaxRows
		}
		for i := 0; i < max; i++ {
			b, err := json.Marshal(rows[i])
			if err != nil {
				return nil, "", err
			}
			buf.Write(b)
			buf.WriteByte('\n')
		}
		return buf.Bytes(), "application/x-ndjson", nil
	}
	b, err := json.Marshal(node)
	if err != nil {
		return nil, "", err
	}
	buf.Write(b)
	buf.WriteByte('\n')
	return buf.Bytes(), "application/x-ndjson", nil
}

func stringify(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64, bool, int, int64, float32:
		return fmt.Sprint(t)
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}
