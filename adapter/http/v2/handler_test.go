package v2

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"
    "os"
    "path/filepath"

    msgw "github.com/viant/agently/internal/dao/message/write"
    plw "github.com/viant/agently/internal/dao/payload/write"
    mcw "github.com/viant/agently/internal/dao/modelcall/write"
    tcw "github.com/viant/agently/internal/dao/toolcall/write"
)

// response envelope used by handler
type resp struct {
    Status string        `json:"status"`
    Data   []interface{} `json:"data"`
}

func TestTranscript_ExcludeInterim(t *testing.T) {
    h := New()
    mux := http.NewServeMux()
    h.Register(mux)
    srv := httptest.NewServer(mux)
    defer srv.Close()

    // Seed two messages: one interim=1, one finalized (interim=0)
    convID := "conv-test"
    now := time.Now().Add(-time.Minute)
    // Interim message
    {
        m := &msgw.Message{}
        m.SetId("m-interim")
        m.SetConversationID(convID)
        m.SetCreatedAt(now)
        m.SetRole("assistant")
        m.SetType("text")
        m.SetContent("working…")
        v := 1
        m.Interim = &v
        m.Has.Interim = true
        if _, err := h.apis.Message.Patch(nil, m); err != nil {
            t.Fatalf("patch interim: %v", err)
        }
    }
    // Final message
    {
        m := &msgw.Message{}
        m.SetId("m-final")
        m.SetConversationID(convID)
        m.SetCreatedAt(now.Add(time.Second))
        m.SetRole("assistant")
        m.SetType("text")
        m.SetContent("done")
        v := 0
        m.Interim = &v
        m.Has.Interim = true
        if _, err := h.apis.Message.Patch(nil, m); err != nil {
            t.Fatalf("patch final: %v", err)
        }
    }

    // 1) Default: exclude interim
    {
        url := srv.URL + "/v2/api/agently/conversation/" + convID + "/transcript"
        res, err := http.Get(url)
        if err != nil { t.Fatalf("GET default: %v", err) }
        defer res.Body.Close()
        if res.StatusCode != http.StatusOK { t.Fatalf("status=%d", res.StatusCode) }
        var out resp
        if err := json.NewDecoder(res.Body).Decode(&out); err != nil { t.Fatalf("decode: %v", err) }
        if out.Status != "ok" { t.Fatalf("status: %s", out.Status) }
        if len(out.Data) != 1 { t.Fatalf("expected 1 message (final), got %d", len(out.Data)) }
    }

    // 2) Include interim explicitly
    {
        url := srv.URL + "/v2/api/agently/conversation/" + convID + "/transcript?excludeInterim=0"
        res, err := http.Get(url)
        if err != nil { t.Fatalf("GET include interim: %v", err) }
        defer res.Body.Close()
        if res.StatusCode != http.StatusOK { t.Fatalf("status=%d", res.StatusCode) }
        var out resp
        if err := json.NewDecoder(res.Body).Decode(&out); err != nil { t.Fatalf("decode: %v", err) }
        if out.Status != "ok" { t.Fatalf("status: %s", out.Status) }
        if len(out.Data) != 2 { t.Fatalf("expected 2 messages (interim+final), got %d", len(out.Data)) }
    }
}

func TestPayload_RangeAndRaw(t *testing.T) {
    h := New()
    mux := http.NewServeMux()
    h.Register(mux)
    srv := httptest.NewServer(mux)
    defer srv.Close()

    // Seed inline payload
    {
        body := []byte("0123456789")
        p := &plw.Payload{}
        p.SetId("p-inline")
        p.SetKind("test")
        p.SetMimeType("text/plain")
        p.SetSizeBytes(len(body))
        p.SetStorage("inline")
        p.SetInlineBody(body)
        if _, err := h.apis.Payload.Patch(nil, p); err != nil {
            t.Fatalf("patch inline payload: %v", err)
        }
    }

    // Range on inline: bytes=2-5 → "2345"
    {
        req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v2/api/agently/payload/p-inline?raw=1", nil)
        req.Header.Set("Range", "bytes=2-5")
        res, err := http.DefaultClient.Do(req)
        if err != nil { t.Fatalf("GET range inline: %v", err) }
        defer res.Body.Close()
        if res.StatusCode != http.StatusPartialContent { t.Fatalf("status=%d", res.StatusCode) }
        if got := res.Header.Get("Content-Range"); got != "bytes 2-5/10" { t.Fatalf("content-range=%s", got) }
        buf := make([]byte, 4)
        if n, _ := res.Body.Read(buf); n != 4 || string(buf) != "2345" {
            t.Fatalf("body unexpected: %q", string(buf[:n]))
        }
    }

    // Full inline
    {
        res, err := http.Get(srv.URL+"/v2/api/agently/payload/p-inline?raw=1")
        if err != nil { t.Fatalf("GET full inline: %v", err) }
        defer res.Body.Close()
        if res.StatusCode != http.StatusOK { t.Fatalf("status=%d", res.StatusCode) }
    }

    // Seed object payload backed by a temp file
    tmpdir := t.TempDir()
    path := filepath.Join(tmpdir, "obj.txt")
    content := []byte("ABCDEFGHIJK") // 11 bytes
    if err := os.WriteFile(path, content, 0644); err != nil { t.Fatalf("write tmp: %v", err) }
    {
        uri := "file://" + path
        p := &plw.Payload{}
        p.SetId("p-object")
        p.SetKind("test")
        p.SetMimeType("text/plain")
        p.SetSizeBytes(len(content))
        p.SetStorage("object")
        p.SetURI(uri)
        if _, err := h.apis.Payload.Patch(nil, p); err != nil {
            t.Fatalf("patch object payload: %v", err)
        }
    }
    // Range suffix on object: bytes=6- → "GHIJK"
    {
        req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v2/api/agently/payload/p-object?raw=1", nil)
        req.Header.Set("Range", "bytes=6-")
        res, err := http.DefaultClient.Do(req)
        if err != nil { t.Fatalf("GET range object: %v", err) }
        defer res.Body.Close()
        if res.StatusCode != http.StatusPartialContent { t.Fatalf("status=%d", res.StatusCode) }
        if got := res.Header.Get("Content-Range"); got != "bytes 6-10/11" { t.Fatalf("content-range=%s", got) }
        buf := make([]byte, 5)
        if n, _ := res.Body.Read(buf); string(buf[:n]) != "GHIJK" { t.Fatalf("body=%q", string(buf[:n])) }
    }

    // Invalid range → 416
    {
        req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v2/api/agently/payload/p-inline?raw=1", nil)
        req.Header.Set("Range", "bytes=999-1000")
        res, err := http.DefaultClient.Do(req)
        if err != nil { t.Fatalf("GET invalid range: %v", err) }
        defer res.Body.Close()
        if res.StatusCode != http.StatusRequestedRangeNotSatisfiable { t.Fatalf("status=%d", res.StatusCode) }
        if got := res.Header.Get("Content-Range"); got != "bytes */10" { t.Fatalf("content-range=%s", got) }
    }
}

func TestOperations_MessageAndTurn(t *testing.T) {
    h := New()
    mux := http.NewServeMux()
    h.Register(mux)
    srv := httptest.NewServer(mux)
    defer srv.Close()

    // Seed payloads for model/tool
    mustPatchPayload := func(id, kind string) {
        p := &plw.Payload{}
        p.SetId(id)
        p.SetKind(kind)
        p.SetMimeType("application/json")
        body := []byte(`{"x":1}`)
        p.SetSizeBytes(len(body))
        p.SetStorage("inline")
        p.SetInlineBody(body)
        if _, err := h.apis.Payload.Patch(nil, p); err != nil {
            t.Fatalf("patch payload %s: %v", id, err)
        }
    }
    mustPatchPayload("pm-req", "model_request")
    mustPatchPayload("pm-res", "model_response")
    mustPatchPayload("pt-req", "tool_request")
    mustPatchPayload("pt-res", "tool_response")

    // Seed model call with payload IDs
    {
        mc := &mcw.ModelCall{MessageID: "m1", Provider: "prov", Model: "m", ModelKind: "chat", Has: &mcw.ModelCallHas{}}
        mc.TurnID = strPtr("t1"); mc.Has.TurnID = true
        mc.RequestPayloadID = strPtr("pm-req"); mc.Has.RequestPayloadID = true
        mc.ResponsePayloadID = strPtr("pm-res"); mc.Has.ResponsePayloadID = true
        if _, err := h.apis.ModelCall.Patch(nil, mc); err != nil {
            t.Fatalf("patch modelcall: %v", err)
        }
        // Extra model call for pagination check
        mc2 := &mcw.ModelCall{MessageID: "m2", Provider: "prov", Model: "m", ModelKind: "chat", Has: &mcw.ModelCallHas{}}
        if _, err := h.apis.ModelCall.Patch(nil, mc2); err != nil {
            t.Fatalf("patch modelcall 2: %v", err)
        }
    }

    // Seed tool call with snapshots referencing payload IDs
    {
        snapReq := `{"payloadId":"pt-req"}`
        snapRes := `{"payloadId":"pt-res"}`
        tc := &tcw.ToolCall{MessageID: "m1", OpID: "op1", Attempt: 1, ToolName: "sys.echo", ToolKind: "general", Status: "completed", Has: &tcw.ToolCallHas{}}
        tc.TurnID = strPtr("t1"); tc.Has.TurnID = true
        tc.RequestSnapshot = &snapReq; tc.Has.RequestSnapshot = true
        tc.ResponseSnapshot = &snapRes; tc.Has.ResponseSnapshot = true
        if _, err := h.apis.ToolCall.Patch(nil, tc); err != nil {
            t.Fatalf("patch toolcall: %v", err)
        }
    }

    // Message scoped operations with payload enrichment
    {
        res, err := http.Get(srv.URL+"/v2/api/agently/messages/m1/operations?includePayloads=1")
        if err != nil { t.Fatalf("GET ops by message: %v", err) }
        defer res.Body.Close()
        if res.StatusCode != http.StatusOK { t.Fatalf("status=%d", res.StatusCode) }
        var out struct{
            Status string `json:"status"`
            Data struct{
                ModelCalls []map[string]interface{} `json:"modelCalls"`
                ToolCalls  []map[string]interface{} `json:"toolCalls"`
            } `json:"data"`
        }
        if err := json.NewDecoder(res.Body).Decode(&out); err != nil { t.Fatalf("decode: %v", err) }
        if out.Status != "ok" { t.Fatalf("status=%s", out.Status) }
        if len(out.Data.ModelCalls) == 0 || len(out.Data.ToolCalls) == 0 { t.Fatalf("expected both model and tool calls") }
        // Ensure payloads present in enriched rows
        if _, ok := out.Data.ModelCalls[0]["request"]; !ok { t.Fatalf("model request missing") }
        if _, ok := out.Data.ModelCalls[0]["response"]; !ok { t.Fatalf("model response missing") }
        if _, ok := out.Data.ToolCalls[0]["request"]; !ok { t.Fatalf("tool request missing") }
        if _, ok := out.Data.ToolCalls[0]["response"]; !ok { t.Fatalf("tool response missing") }
    }

    // Pagination: limit=1 should cap both groups
    {
        res, err := http.Get(srv.URL+"/v2/api/agently/messages/m1/operations?limit=1")
        if err != nil { t.Fatalf("GET ops limited: %v", err) }
        defer res.Body.Close()
        var out struct{ Status string; Data struct{ ModelCalls []interface{}; ToolCalls []interface{} } }
        if err := json.NewDecoder(res.Body).Decode(&out); err != nil { t.Fatalf("decode: %v", err) }
        if len(out.Data.ModelCalls) != 1 { t.Fatalf("expected 1 model call, got %d", len(out.Data.ModelCalls)) }
        if len(out.Data.ToolCalls) != 1 { t.Fatalf("expected 1 tool call, got %d", len(out.Data.ToolCalls)) }
    }

    // Turn scoped operations
    {
        res, err := http.Get(srv.URL+"/v2/api/agently/turns/t1/operations?includePayloads=1")
        if err != nil { t.Fatalf("GET ops by turn: %v", err) }
        defer res.Body.Close()
        if res.StatusCode != http.StatusOK { t.Fatalf("status=%d", res.StatusCode) }
        var out struct{ Status string; Data struct{ ModelCalls []map[string]interface{}; ToolCalls []map[string]interface{} } }
        if err := json.NewDecoder(res.Body).Decode(&out); err != nil { t.Fatalf("decode: %v", err) }
        if out.Status != "ok" { t.Fatalf("status=%s", out.Status) }
        if len(out.Data.ModelCalls) == 0 || len(out.Data.ToolCalls) == 0 { t.Fatalf("expected both model and tool calls by turn") }
    }
}

func strPtr(s string) *string { return &s }
