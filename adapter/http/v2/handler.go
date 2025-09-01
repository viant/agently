package v2

import (
    "context"
    "encoding/json"
    "net/http"
    "strconv"
    "strings"
    "time"

    daofactory "github.com/viant/agently/internal/dao/factory"
    mcread "github.com/viant/agently/internal/dao/modelcall/read"
    tcread "github.com/viant/agently/internal/dao/toolcall/read"
    pldaoRead "github.com/viant/agently/internal/dao/payload/read"
    d "github.com/viant/agently/internal/domain"
    domainadapter "github.com/viant/agently/internal/domain/adapter"
    msgread "github.com/viant/agently/internal/dao/message/read"
    turnread "github.com/viant/agently/internal/dao/turn/read"
    usageread "github.com/viant/agently/internal/dao/usage/read"
    "github.com/viant/afs"
)

type Handler struct {
    store d.Store
    apis  *daofactory.API
}

func New() *Handler {
    // Build a memory-backed domain store for now. Later wire SQL when available.
    apis, _ := daofactory.New(nil, daofactory.DAOInMemory, nil)
    st := domainadapter.New(apis.Conversation, apis.Message, apis.Turn, apis.ModelCall, apis.ToolCall, apis.Payload, apis.Usage)
    return &Handler{store: st, apis: apis}
}

// NewWithStore allows injecting a prebuilt domain.Store (e.g. SQL-backed)
func NewWithStore(store d.Store) *Handler { return &Handler{store: store} }

// NewWithAPIs allows injecting both store and DAO APIs; avoids re-wiring in handler.
func NewWithAPIs(store d.Store, apis *daofactory.API) *Handler { return &Handler{store: store, apis: apis} }

func (h *Handler) Register(mux *http.ServeMux) {
    // Transcript
    mux.HandleFunc("GET /v2/api/agently/conversation/{id}/transcript", h.handleTranscript)
    // Operations by message/turn
    mux.HandleFunc("GET /v2/api/agently/messages/{id}/operations", h.handleOpsByMessage)
    mux.HandleFunc("GET /v2/api/agently/turns/{id}/operations", h.handleOpsByTurn)
    // Payload
    mux.HandleFunc("GET /v2/api/agently/payload/{id}", h.handlePayload)
    // Usage
    mux.HandleFunc("GET /v2/api/agently/conversation/{id}/usage", h.handleUsage)
    // Turns
    mux.HandleFunc("GET /v2/api/agently/conversation/{id}/turn", h.handleTurnsByConversation)
}

// ---------------- helpers -----------------

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    _ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "data": v})
}

func badRequest(w http.ResponseWriter, msg string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusBadRequest)
    _ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "ERROR", "message": msg})
}

func writeError(w http.ResponseWriter, code int, msg string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    _ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "ERROR", "message": msg})
}

// ---------------- endpoints ----------------

func (h *Handler) handleTranscript(w http.ResponseWriter, r *http.Request) {
    convID := r.PathValue("id")
    if strings.TrimSpace(convID) == "" {
        badRequest(w, "conversation id required")
        return
    }
    // Map query to TranscriptAggOptions
    q := r.URL.Query()
    // Default to excluding interim to preserve v1 semantics; allow opt-in via query.
    opts := d.TranscriptAggOptions{ExcludeInterim: true}
    if v := q.Get("excludeInterim"); v != "" { opts.ExcludeInterim = v == "1" || strings.ToLower(v) == "true" }
    if v := q.Get("includeTools"); v != "" { opts.IncludeTools = v == "1" || strings.ToLower(v) == "true" }
    if v := q.Get("includeModelCalls"); v != "" { opts.IncludeModelCalls = v == "1" || strings.ToLower(v) == "true" }
    if v := q.Get("includeToolCalls"); v != "" { opts.IncludeToolCalls = v == "1" || strings.ToLower(v) == "true" }
    if v := q.Get("payloadLevel"); v != "" { opts.PayloadLevel = d.PayloadLevel(strings.ToLower(v)) }
    if v := q.Get("payloadInlineMaxB"); v != "" { if n, _ := strconv.Atoi(v); n > 0 { opts.PayloadInlineMaxB = n } }
    if v := q.Get("redact"); v != "" { opts.RedactSensitive = v == "1" || strings.ToLower(v) == "true" }
    if v := q.Get("since"); v != "" { if t, err := time.Parse(time.RFC3339, v); err == nil { opts.Since = &t } }
    if v := q.Get("limit"); v != "" { if n, err := strconv.Atoi(v); err == nil { opts.Limit = &n } }

    // When DAO APIs are available, return DAO read views directly
    if h.apis != nil && h.apis.Message != nil {
        in := []msgread.InputOption{msgread.WithConversationID(convID)}
        if opts.ExcludeInterim { in = append(in, msgread.WithInterim(0)) }
        if opts.Since != nil { in = append(in, msgread.WithSince(*opts.Since)) }
        // Memory DAO GetTranscript filters by turn strictly; when turn not provided, use GetConversation.
        rows, err := h.apis.Message.GetConversation(r.Context(), convID, in...)
        if err != nil { badRequest(w, err.Error()); return }
        // Optional interim filtering (memory DAO GetConversation ignores input options)
        if opts.ExcludeInterim {
            filtered := make([]*msgread.MessageView, 0, len(rows))
            for _, v := range rows {
                if v == nil { continue }
                if v.Interim != nil && *v.Interim != 0 { continue }
                filtered = append(filtered, v)
            }
            rows = filtered
        }
        // Optional sinceId slicing and limit for parity with v1 polling semantics
        sinceID := strings.TrimSpace(q.Get("sinceId"))
        if sinceID != "" {
            start := -1
            for i := range rows {
                if rows[i] != nil && rows[i].Id == sinceID { start = i; break }
            }
            if start >= 0 { rows = rows[start:] } else { rows = rows[:0] }
        }
        if opts.Limit != nil && *opts.Limit > 0 && len(rows) > *opts.Limit { rows = rows[:*opts.Limit] }
        writeJSON(w, http.StatusOK, rows)
        return
    }
    // Fallback to domain aggregate
    agg, err := h.store.Messages().GetTranscriptAggregated(r.Context(), convID, "", opts)
    if err != nil { badRequest(w, err.Error()); return }
    // Optional sinceId slicing and limit on aggregated transcript
    sinceID := strings.TrimSpace(q.Get("sinceId"))
    if sinceID != "" && agg != nil && len(agg.Messages) > 0 {
        start := -1
        for i := range agg.Messages {
            if agg.Messages[i] != nil && agg.Messages[i].Message != nil && agg.Messages[i].Message.ID == sinceID { start = i; break }
        }
        if start >= 0 { agg.Messages = agg.Messages[start:] } else { agg.Messages = agg.Messages[:0] }
    }
    if opts.Limit != nil && *opts.Limit > 0 && agg != nil && len(agg.Messages) > *opts.Limit { agg.Messages = agg.Messages[:*opts.Limit] }
    writeJSON(w, http.StatusOK, agg)
}

func (h *Handler) handleOpsByMessage(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id")
    if id == "" { badRequest(w, "message id required"); return }
    q := r.URL.Query()
    includePayloads := strings.ToLower(q.Get("includePayloads")) == "1" || strings.ToLower(q.Get("payloads")) == "1" || strings.ToLower(q.Get("includePayloads")) == "true"
    lim, _ := strconv.Atoi(q.Get("limit"))
    off, _ := strconv.Atoi(q.Get("offset"))
    if h.apis != nil {
        // Return DAO read views directly grouped by kind
        models, _ := h.apis.ModelCall.List(r.Context(), mcread.WithMessageID(id))
        tools, _ := h.apis.ToolCall.List(r.Context(), tcread.WithMessageID(id))
        // Optional payload enrichment
        if includePayloads && h.apis.Payload != nil {
            modelsEnriched := make([]map[string]interface{}, 0, len(models))
            for _, v := range models {
                row := map[string]interface{}{"call": v}
                if v.RequestPayloadID != nil && *v.RequestPayloadID != "" {
                    if pv, _ := h.apis.Payload.List(r.Context(), pldaoRead.WithID(*v.RequestPayloadID)); len(pv) > 0 { row["request"] = pv[0] }
                }
                if v.ResponsePayloadID != nil && *v.ResponsePayloadID != "" {
                    if pv, _ := h.apis.Payload.List(r.Context(), pldaoRead.WithID(*v.ResponsePayloadID)); len(pv) > 0 { row["response"] = pv[0] }
                }
                modelsEnriched = append(modelsEnriched, row)
            }
            toolsEnriched := make([]map[string]interface{}, 0, len(tools))
            for _, v := range tools {
                row := map[string]interface{}{"call": v}
                if id := payloadIDFromSnapshot(v.RequestSnapshot); id != "" {
                    if pv, _ := h.apis.Payload.List(r.Context(), pldaoRead.WithID(id)); len(pv) > 0 { row["request"] = pv[0] }
                }
                if id := payloadIDFromSnapshot(v.ResponseSnapshot); id != "" {
                    if pv, _ := h.apis.Payload.List(r.Context(), pldaoRead.WithID(id)); len(pv) > 0 { row["response"] = pv[0] }
                }
                toolsEnriched = append(toolsEnriched, row)
            }
            // Apply offset/limit per group
            modelsEnriched = sliceMaps(modelsEnriched, off, lim)
            toolsEnriched = sliceMaps(toolsEnriched, off, lim)
            writeJSON(w, http.StatusOK, map[string]interface{}{"modelCalls": modelsEnriched, "toolCalls": toolsEnriched})
            return
        }
        // Apply offset/limit per group when no enrichment
        models = sliceModels(models, off, lim)
        tools = sliceTools(tools, off, lim)
        writeJSON(w, http.StatusOK, map[string]interface{}{"modelCalls": models, "toolCalls": tools})
        return
    }
    ops, err := h.store.Operations().GetByMessage(r.Context(), id)
    if err != nil { badRequest(w, err.Error()); return }
    if lim > 0 || off > 0 {
        if off < 0 { off = 0 }
        if off > len(ops) { ops = ops[:0] } else { ops = ops[off:] }
        if lim > 0 && len(ops) > lim { ops = ops[:lim] }
    }
    writeJSON(w, http.StatusOK, ops)
}

func (h *Handler) handleOpsByTurn(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id")
    if id == "" { badRequest(w, "turn id required"); return }
    q := r.URL.Query()
    includePayloads := strings.ToLower(q.Get("includePayloads")) == "1" || strings.ToLower(q.Get("payloads")) == "1" || strings.ToLower(q.Get("includePayloads")) == "true"
    lim, _ := strconv.Atoi(q.Get("limit"))
    off, _ := strconv.Atoi(q.Get("offset"))
    if h.apis != nil {
        models, _ := h.apis.ModelCall.List(r.Context(), mcread.WithTurnID(id))
        tools, _ := h.apis.ToolCall.List(r.Context(), tcread.WithTurnID(id))
        if includePayloads && h.apis.Payload != nil {
            modelsEnriched := make([]map[string]interface{}, 0, len(models))
            for _, v := range models {
                row := map[string]interface{}{"call": v}
                if v.RequestPayloadID != nil && *v.RequestPayloadID != "" {
                    if pv, _ := h.apis.Payload.List(r.Context(), pldaoRead.WithID(*v.RequestPayloadID)); len(pv) > 0 { row["request"] = pv[0] }
                }
                if v.ResponsePayloadID != nil && *v.ResponsePayloadID != "" {
                    if pv, _ := h.apis.Payload.List(r.Context(), pldaoRead.WithID(*v.ResponsePayloadID)); len(pv) > 0 { row["response"] = pv[0] }
                }
                modelsEnriched = append(modelsEnriched, row)
            }
            toolsEnriched := make([]map[string]interface{}, 0, len(tools))
            for _, v := range tools {
                row := map[string]interface{}{"call": v}
                if id := payloadIDFromSnapshot(v.RequestSnapshot); id != "" {
                    if pv, _ := h.apis.Payload.List(r.Context(), pldaoRead.WithID(id)); len(pv) > 0 { row["request"] = pv[0] }
                }
                if id := payloadIDFromSnapshot(v.ResponseSnapshot); id != "" {
                    if pv, _ := h.apis.Payload.List(r.Context(), pldaoRead.WithID(id)); len(pv) > 0 { row["response"] = pv[0] }
                }
                toolsEnriched = append(toolsEnriched, row)
            }
            modelsEnriched = sliceMaps(modelsEnriched, off, lim)
            toolsEnriched = sliceMaps(toolsEnriched, off, lim)
            writeJSON(w, http.StatusOK, map[string]interface{}{"modelCalls": modelsEnriched, "toolCalls": toolsEnriched})
            return
        }
        models = sliceModels(models, off, lim)
        tools = sliceTools(tools, off, lim)
        writeJSON(w, http.StatusOK, map[string]interface{}{"modelCalls": models, "toolCalls": tools})
        return
    }
    ops, err := h.store.Operations().GetByTurn(r.Context(), id)
    if err != nil { badRequest(w, err.Error()); return }
    if lim > 0 || off > 0 {
        if off < 0 { off = 0 }
        if off > len(ops) { ops = ops[:0] } else { ops = ops[off:] }
        if lim > 0 && len(ops) > lim { ops = ops[:lim] }
    }
    writeJSON(w, http.StatusOK, ops)
}

// -------------- small helpers ----------------

func sliceModels(in []*mcread.ModelCallView, off, lim int) []*mcread.ModelCallView {
    if off < 0 { off = 0 }
    if off > len(in) { return in[:0] }
    out := in[off:]
    if lim > 0 && len(out) > lim { out = out[:lim] }
    return out
}

func sliceTools(in []*tcread.ToolCallView, off, lim int) []*tcread.ToolCallView {
    if off < 0 { off = 0 }
    if off > len(in) { return in[:0] }
    out := in[off:]
    if lim > 0 && len(out) > lim { out = out[:lim] }
    return out
}

func sliceMaps(in []map[string]interface{}, off, lim int) []map[string]interface{} {
    if off < 0 { off = 0 }
    if off > len(in) { return in[:0] }
    out := in[off:]
    if lim > 0 && len(out) > lim { out = out[:lim] }
    return out
}

func payloadIDFromSnapshot(snapshot *string) string {
    if snapshot == nil || *snapshot == "" { return "" }
    var x struct{ PayloadID string `json:"payloadId"` }
    if json.Unmarshal([]byte(*snapshot), &x) == nil { return x.PayloadID }
    return ""
}

func (h *Handler) handlePayload(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id")
    if id == "" { badRequest(w, "payload id required"); return }
    pv, err := h.store.Payloads().Get(r.Context(), id)
    if err != nil { badRequest(w, err.Error()); return }
    if pv == nil { writeError(w, http.StatusNotFound, "payload not found"); return }
    // raw mode: stream inline body when available and not redacted
    if raw := r.URL.Query().Get("raw"); raw == "1" || strings.ToLower(raw) == "true" {
        if pv.Redacted != nil && *pv.Redacted == 1 {
            w.WriteHeader(http.StatusNoContent)
            return
        }
        // Parse single-range header when present
        total := int64(pv.SizeBytes)
        start, end, partial, rngErr := parseRange(r.Header.Get("Range"), total)
        if rngErr != "" {
            // Invalid range – RFC 7233 416 with Content-Range: */length
            w.Header().Set("Content-Range", "bytes */"+strconv.FormatInt(total, 10))
            writeError(w, http.StatusRequestedRangeNotSatisfiable, rngErr)
            return
        }
        // Inline storage
        if pv.Storage == "inline" && pv.InlineBody != nil {
            data := *pv.InlineBody
            if int64(len(data)) != total { total = int64(len(data)) }
            // Clamp to total
            if partial {
                if start < 0 { start = 0 }
                if end >= total { end = total - 1 }
                if start > end || start >= total { w.Header().Set("Content-Range", "bytes */"+strconv.FormatInt(total, 10)); writeError(w, http.StatusRequestedRangeNotSatisfiable, "unsatisfiable range"); return }
                data = data[start : end+1]
                w.Header().Set("Accept-Ranges", "bytes")
                if pv.MimeType != "" { w.Header().Set("Content-Type", pv.MimeType) }
                w.Header().Set("Content-Length", strconv.Itoa(len(data)))
                w.Header().Set("Content-Range", contentRangeHeader(start, end, total))
                w.WriteHeader(http.StatusPartialContent)
                _, _ = w.Write(data)
                return
            }
            w.Header().Set("Accept-Ranges", "bytes")
            if pv.MimeType != "" { w.Header().Set("Content-Type", pv.MimeType) }
            w.Header().Set("Content-Length", strconv.Itoa(len(data)))
            w.WriteHeader(http.StatusOK)
            _, _ = w.Write(data)
            return
        }
        // Object storage – apply timeout and naive range slicing
        if pv.Storage == "object" && pv.URI != nil {
            fs := afs.New()
            // Short timeout for remote fetch to avoid hanging connections
            ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
            defer cancel()
            blob, err := fs.DownloadWithURL(ctx, *pv.URI)
            if err != nil {
                if ctx.Err() == context.DeadlineExceeded {
                    writeError(w, http.StatusGatewayTimeout, "payload download timeout")
                } else {
                    writeError(w, http.StatusBadGateway, err.Error())
                }
                return
            }
            if total == 0 { total = int64(len(blob)) }
            // Serve partial when requested
            if partial {
                if start < 0 { start = 0 }
                if end >= total { end = total - 1 }
                if start > end || start >= total { w.Header().Set("Content-Range", "bytes */"+strconv.FormatInt(total, 10)); writeError(w, http.StatusRequestedRangeNotSatisfiable, "unsatisfiable range"); return }
                segment := blob[start : end+1]
                w.Header().Set("Accept-Ranges", "bytes")
                if pv.MimeType != "" { w.Header().Set("Content-Type", pv.MimeType) }
                w.Header().Set("Content-Length", strconv.Itoa(len(segment)))
                w.Header().Set("Content-Range", contentRangeHeader(start, end, total))
                w.WriteHeader(http.StatusPartialContent)
                _, _ = w.Write(segment)
                return
            }
            w.Header().Set("Accept-Ranges", "bytes")
            if pv.MimeType != "" { w.Header().Set("Content-Type", pv.MimeType) }
            w.Header().Set("Content-Length", strconv.Itoa(len(blob)))
            w.WriteHeader(http.StatusOK)
            _, _ = w.Write(blob)
            return
        }
        // Unknown storage or missing content
        w.WriteHeader(http.StatusNoContent)
        return
    }
    writeJSON(w, http.StatusOK, pv)
}

// parseRange parses a single bytes=START-END header. Returns start/end (inclusive),
// a boolean indicating partial request, and an error message when invalid.
func parseRange(h string, total int64) (int64, int64, bool, string) {
    h = strings.TrimSpace(h)
    if h == "" { return 0, 0, false, "" }
    if !strings.HasPrefix(strings.ToLower(h), "bytes=") { return 0, 0, false, "unsupported range unit" }
    spec := strings.TrimSpace(h[len("bytes="):])
    // Only single range supported
    if strings.Contains(spec, ",") { return 0, 0, false, "multiple ranges not supported" }
    // forms: start-end | start- | -suffix
    if strings.HasPrefix(spec, "-") {
        // suffix length
        n, err := strconv.ParseInt(spec[1:], 10, 64)
        if err != nil || n <= 0 { return 0, 0, false, "invalid suffix range" }
        if total <= 0 { return 0, 0, false, "unknown total length" }
        if n > total { n = total }
        return total - n, total - 1, true, ""
    }
    parts := strings.SplitN(spec, "-", 2)
    if len(parts) != 2 { return 0, 0, false, "invalid range" }
    start, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
    if err != nil || start < 0 { return 0, 0, false, "invalid range start" }
    // no end → to EOF
    if strings.TrimSpace(parts[1]) == "" {
        if total > 0 && start >= total { return 0, 0, false, "unsatisfiable range" }
        if total > 0 { return start, total - 1, true, "" }
        // unknown total – treat as partial to EOF with sentinel end that will be clamped later
        return start, 1<<62 - 1, true, ""
    }
    end, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
    if err != nil || end < start { return 0, 0, false, "invalid range end" }
    if total > 0 && start >= total { return 0, 0, false, "unsatisfiable range" }
    if total > 0 && end >= total { end = total - 1 }
    return start, end, true, ""
}

func contentRangeHeader(start, end, total int64) string {
    return "bytes " + strconv.FormatInt(start, 10) + "-" + strconv.FormatInt(end, 10) + "/" + strconv.FormatInt(total, 10)
}

func (h *Handler) handleUsage(w http.ResponseWriter, r *http.Request) {
    convID := r.PathValue("id")
    in := usageread.Input{}
    if convID != "" { in.ConversationID = convID; in.Has = &usageread.Has{ConversationID: true} }
    rows, err := h.store.Usage().List(r.Context(), in)
    if err != nil { badRequest(w, err.Error()); return }
    writeJSON(w, http.StatusOK, rows)
}

func (h *Handler) handleTurnsByConversation(w http.ResponseWriter, r *http.Request) {
    convID := r.PathValue("id")
    opts := []turnread.InputOption{}
    if convID != "" { opts = append(opts, turnread.WithConversationID(convID)) }
    rows, err := h.store.Turns().List(r.Context(), opts...)
    if err != nil { badRequest(w, err.Error()); return }
    writeJSON(w, http.StatusOK, rows)
}
