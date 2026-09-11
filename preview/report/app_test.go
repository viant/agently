package reportpreview

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestHTTP(t *testing.T) {
	h, e := Handler(Config{Folder: filepath.Join("examples", "demo")})
	if e != nil {
		t.Fatal(e)
	}
	server := httptest.NewServer(h)
	defer server.Close()
	for _, tc := range []struct {
		method, path, body string
		status             int
		content            string
	}{
		{"GET", "/", "", 200, "Synthetic mock preview"},
		{"GET", "/api/package", "", 200, "operations"},
		{"POST", "/api/query", `{"dataSource":"operations","page":{"limit":1}}`, 200, "hasMore"},
		{"POST", "/api/query", `{"dataSource":"operations","page":{"limit":1},"typo":true}`, 422, "unknown field"},
		{"GET", "/api/compile", "", 200, "reportSpec"},
		{"GET", "/report.pdf", "", 200, "%PDF-"},
		{"GET", "/api/package?report=/etc", "", 422, "folder"},
		{"GET", "/api/package?variant=../../outside", "", 422, "mockPreviewPathEscape"},
		{"GET", "/report.pdf?variant=error", "", 422, "mockPreviewDataSourceError"},
		{"GET", "/report.pdf?variant=partial", "", 422, "mockPreviewIncompleteData"},
	} {
		t.Run(tc.path+tc.body, func(t *testing.T) {
			r, _ := http.NewRequest(tc.method, server.URL+tc.path, strings.NewReader(tc.body))
			response, e := server.Client().Do(r)
			if e != nil {
				t.Fatal(e)
			}
			defer response.Body.Close()
			body, _ := io.ReadAll(response.Body)
			if response.StatusCode != tc.status || !strings.Contains(string(body), tc.content) {
				t.Fatalf("%d: %.500s", response.StatusCode, body)
			}
		})
	}
	for _, host := range []string{"evil.example", "192.168.1.1"} {
		r := httptest.NewRequest("GET", "http://"+host+"/api/package", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden {
			t.Fatal("untrusted Host accepted")
		}
	}
	r := httptest.NewRequest("POST", "http://localhost/api/query", strings.NewReader(`{}`))
	r.Header.Set("Origin", "http://evil.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatal("cross-origin request accepted")
	}
}
func TestCLI(t *testing.T) {
	for _, args := range [][]string{{"validate", "examples/demo"}, {"validate", "--variant", "empty", "examples/demo"}, {"describe", "examples/demo", "--datasource", "operations"}, {"compile", "examples/demo2", "--full"}} {
		var out, stderr bytes.Buffer
		if e := RunCLI(context.Background(), args, &out, &stderr); e != nil {
			t.Fatal(e)
		}
		if !json.Valid(out.Bytes()) {
			t.Fatal("expected JSON")
		}
	}
	var out bytes.Buffer
	if e := RunCLI(context.Background(), []string{"export", "examples/demo", "--out", filepath.Join(t.TempDir(), "report.pdf")}, &out, &out); e != nil {
		t.Fatal(e)
	}
}
func TestStrictQuery(t *testing.T) {
	for _, raw := range []string{`{"typo":true}`, `{"projection":{"dimensions":[{"field":"name","typo":true}]}}`, `{} {}`} {
		if _, e := DecodeQuery(strings.NewReader(raw)); e == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}
