// Package reportpreview bootstraps the standalone, local report preview app.
package reportpreview

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	preview "github.com/viant/agently/preview/report/provider"
)

//go:embed index.html
var indexHTML []byte

type Config struct {
	Folder  string
	Variant string
	Addr    string
}

// Handler loads only the folder supplied by the host. Browser parameters never
// select filesystem paths. Query work is bounded to eight concurrent requests.
func Handler(config Config) (http.Handler, error) {
	_, err := preview.Load(config.Folder, config.Variant)
	if err != nil {
		return nil, err
	}
	sem := make(chan struct{}, 8)
	mux := http.NewServeMux()
	mockServer, err := MockServer(config)
	if err != nil {
		return nil, err
	}
	mux.Handle("/mcp", mockServer.HTTPHandler())
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	})
	mux.HandleFunc("/api/package", func(w http.ResponseWriter, r *http.Request) {
		p, e := load(config, r)
		if e != nil {
			writeError(w, e)
			return
		}
		writeJSON(w, p)
	})
	mux.HandleFunc("/api/query", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		p, e := load(config, r)
		if e != nil {
			writeError(w, e)
			return
		}
		var q preview.Query
		if e = decodeBody(w, r, &q); e != nil {
			writeError(w, e)
			return
		}
		result, e := p.Execute(q.DataSource, q)
		if e != nil {
			writeError(w, e)
			return
		}
		writeJSON(w, result)
	})
	mux.HandleFunc("/api/compile", func(w http.ResponseWriter, r *http.Request) {
		p, e := load(config, r)
		if e != nil {
			writeError(w, e)
			return
		}
		params, e := parameters(r)
		if e != nil {
			writeError(w, e)
			return
		}
		c, e := p.Compile(params, r.URL.Query().Get("full") == "true")
		if e != nil {
			writeError(w, e)
			return
		}
		writeJSON(w, c)
	})
	mux.HandleFunc("/report.html", func(w http.ResponseWriter, r *http.Request) {
		p, e := load(config, r)
		if e != nil {
			writeError(w, e)
			return
		}
		params, e := parameters(r)
		if e != nil {
			writeError(w, e)
			return
		}
		c, e := p.Compile(params, r.URL.Query().Get("full") == "true")
		if e != nil {
			writeError(w, e)
			return
		}
		b, e := c.HTML()
		if e != nil {
			writeError(w, e)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(b)
	})
	mux.HandleFunc("/report.pdf", func(w http.ResponseWriter, r *http.Request) {
		p, e := load(config, r)
		if e != nil {
			writeError(w, e)
			return
		}
		params, e := parameters(r)
		if e != nil {
			writeError(w, e)
			return
		}
		c, e := p.Compile(params, r.URL.Query().Get("full") != "false")
		if e != nil {
			writeError(w, e)
			return
		}
		b, e := c.PDF()
		if e != nil {
			writeError(w, e)
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `inline; filename="report.pdf"`)
		w.Write(b)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		if host != "localhost" && net.ParseIP(host) != nil && !net.ParseIP(host).IsLoopback() || host != "localhost" && net.ParseIP(host) == nil {
			http.Error(w, "local host required", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host && origin != "https://"+r.Host {
			http.Error(w, "same-origin requests required", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodPost && !(r.URL.Path == "/mcp" && (r.Method == http.MethodDelete || r.Method == http.MethodOptions)) {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; frame-src 'self' blob:; img-src 'self' data:; object-src 'none'; base-uri 'none'; frame-ancestors 'self'")
		if r.URL.Path == "/mcp" {
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			mux.ServeHTTP(w, r)
			return
		}
		select {
		case sem <- struct{}{}:
			defer func() { <-sem }()
			mux.ServeHTTP(w, r)
		case <-r.Context().Done():
			return
		}
	}), nil
}
func load(c Config, r *http.Request) (*preview.Package, error) {
	if r.URL.Query().Get("report") != "" {
		return nil, fmt.Errorf("report folder is selected by the CLI, not the browser")
	}
	variant := r.URL.Query().Get("variant")
	if variant == "" {
		variant = c.Variant
	}
	p, err := preview.Load(c.Folder, variant)
	if err != nil {
		return nil, err
	}
	if err = p.UseMCP(r.Context(), "http://"+r.Host); err != nil {
		return nil, err
	}
	return p, nil
}
func parameters(r *http.Request) (preview.Object, error) {
	p := preview.Object{}
	if raw := r.URL.Query().Get("parameters"); raw != "" {
		if len(raw) > 64<<10 {
			return nil, fmt.Errorf("parameters exceed 64 KB")
		}
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			return nil, err
		}
	}
	return p, nil
}
func decodeBody(w http.ResponseWriter, r *http.Request, dest any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dest); err != nil {
		return err
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return fmt.Errorf("expected one JSON request")
	}
	return nil
}
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnprocessableEntity)
	var d *preview.Diagnostic
	if errors.As(err, &d) {
		_ = json.NewEncoder(w).Encode(d)
	} else {
		_ = json.NewEncoder(w).Encode(preview.Diagnostic{Code: "mockPreviewMalformedPackage", Message: err.Error(), Path: "report.yaml"})
	}
}
func Serve(ctx context.Context, c Config, output io.Writer) error {
	if c.Addr == "" {
		c.Addr = "127.0.0.1:8095"
	}
	host, _, err := net.SplitHostPort(c.Addr)
	if err != nil {
		return err
	}
	if host != "localhost" && (net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback()) {
		return fmt.Errorf("preview must bind to a loopback address")
	}
	handler, err := Handler(c)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", c.Addr)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 120 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 128 << 10}
	fmt.Fprintf(output, "Synthetic report preview: http://%s\n", listener.Addr())
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = server.Shutdown(shutdown)
		case <-done:
		}
	}()
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// DecodeQuery is shared by the CLI and tests; unknown fields must never be ignored.
func DecodeQuery(reader io.Reader) (preview.Query, error) {
	var q preview.Query
	d := json.NewDecoder(io.LimitReader(reader, 1<<20))
	d.DisallowUnknownFields()
	err := d.Decode(&q)
	if err == nil {
		var extra any
		if d.Decode(&extra) != io.EOF {
			err = fmt.Errorf("expected one query object")
		}
	}
	return q, err
}
