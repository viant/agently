// Package mock exposes local JSON fixtures as MCP tools using viant/mcp.
// It has no dependency on report or window rendering.
package mock

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/viant/jsonrpc"
	"github.com/viant/jsonrpc/transport"
	protoclient "github.com/viant/mcp-protocol/client"
	"github.com/viant/mcp-protocol/logger"
	"github.com/viant/mcp-protocol/schema"
	protoserver "github.com/viant/mcp-protocol/server"
	mcpsrv "github.com/viant/mcp/server"
)

type Tool struct {
	File        string                 `json:"file"`
	Description string                 `json:"description,omitempty"`
	InputSchema schema.ToolInputSchema `json:"inputSchema,omitempty"`
}
type Call struct {
	Tool      string
	Variant   string
	Arguments map[string]any
	Body      json.RawMessage
}
type Transform func(context.Context, Call) (any, error)
type Config struct {
	Root          string
	DataRoot      string
	Variant       string
	Tools         map[string]Tool
	Transform     Transform
	MaxFileBytes  int64
	MaxConcurrent int
}
type Server struct {
	config Config
	tools  map[string]Tool
	server *mcpsrv.Server
	work   chan struct{}
}

var safeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

func New(config Config) (*Server, error) {
	root, err := filepath.Abs(config.Root)
	if err != nil {
		return nil, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	config.Root = root
	if config.DataRoot == "" {
		config.DataRoot = "ds"
	}
	if config.MaxFileBytes == 0 {
		config.MaxFileBytes = 25 << 20
	}
	if config.MaxFileBytes < 1 || config.MaxFileBytes > 25<<20 {
		return nil, fmt.Errorf("fixture limit must be between 1 byte and 25 MB")
	}
	if config.MaxConcurrent == 0 {
		config.MaxConcurrent = 8
	}
	if config.MaxConcurrent < 1 || config.MaxConcurrent > 8 {
		return nil, fmt.Errorf("concurrency must be 1..8")
	}
	if config.Variant == "" {
		config.Variant = "default"
	}
	if !safeName.MatchString(config.Variant) {
		return nil, fmt.Errorf("unsafe default variant")
	}
	tools := map[string]Tool{}
	if len(config.Tools) == 0 {
		dir, e := securePath(root, config.DataRoot)
		if e != nil {
			return nil, e
		}
		entries, e := os.ReadDir(dir)
		if e != nil {
			return nil, e
		}
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".json") {
				id := strings.TrimSuffix(entry.Name(), ".json")
				tools[id] = Tool{File: entry.Name(), Description: "Synthetic local fixture for " + id}
			}
		}
	} else {
		for name, t := range config.Tools {
			tools[name] = t
		}
	}
	if len(tools) == 0 {
		return nil, fmt.Errorf("no JSON fixture tools in %s", config.DataRoot)
	}
	s := &Server{config: config, tools: tools, work: make(chan struct{}, config.MaxConcurrent)}
	for name, t := range tools {
		if !safeName.MatchString(name) || name == "." || name == ".." {
			return nil, fmt.Errorf("unsafe tool name %q", name)
		}
		if t.File == "" {
			t.File = name + ".json"
			tools[name] = t
		}
		if _, err := s.read(t, "default"); err != nil {
			return nil, fmt.Errorf("tool %s: %w", name, err)
		}
	}
	s.server, err = mcpsrv.New(mcpsrv.WithImplementation(schema.Implementation{Name: "forge-mock-datasources", Version: "1.0.0"}), mcpsrv.WithNewHandler(s.newHandler), mcpsrv.WithStreamableURI("/mcp"), mcpsrv.WithRootRedirect(false))
	if err != nil {
		return nil, err
	}
	s.server.UseStreamableHTTP(true)
	return s, nil
}
func (s *Server) HTTPHandler() http.Handler { return s.server.HTTP(context.Background(), "").Handler }
func (s *Server) newHandler(_ context.Context, notifier transport.Notifier, log logger.Logger, ops protoclient.Operations) (protoserver.Handler, error) {
	base := protoserver.NewDefaultHandler(notifier, log, ops)
	names := make([]string, 0, len(s.tools))
	for name := range s.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		tool := s.tools[name]
		input := tool.InputSchema
		if input.Type == "" {
			input = QueryInputSchema()
		}
		base.Registry.RegisterToolWithSchema(name, tool.Description, input, nil, func(ctx context.Context, r *schema.CallToolRequest) (*schema.CallToolResult, *jsonrpc.Error) {
			select {
			case s.work <- struct{}{}:
				defer func() { <-s.work }()
			case <-ctx.Done():
				return toolError(ctx.Err()), nil
			}
			args := r.Params.Arguments
			if args == nil {
				args = map[string]any{}
			}
			variant := s.config.Variant
			if v, ok := args["variant"]; ok {
				var valid bool
				variant, valid = v.(string)
				if !valid {
					return toolError(fmt.Errorf("variant must be a string")), nil
				}
				if variant == "" {
					variant = s.config.Variant
				}
			}
			body, err := s.read(tool, variant)
			if err != nil {
				return toolError(err), nil
			}
			var value any
			if s.config.Transform != nil {
				value, err = s.config.Transform(ctx, Call{Tool: name, Variant: variant, Arguments: args, Body: body})
			} else {
				if input, ok := args["query"]; ok {
					b, e := json.Marshal(input)
					if e != nil {
						return toolError(e), nil
					}
					var q Query
					decoder := json.NewDecoder(bytes.NewReader(b))
					decoder.DisallowUnknownFields()
					if e = decoder.Decode(&q); e != nil {
						return toolError(e), nil
					}
					value, err = ApplyQuery(body, q)
				} else {
					err = json.Unmarshal(body, &value)
				}
			}
			if err != nil {
				return toolError(err), nil
			}
			return toolResult(value, false), nil
		})
	}
	return base, nil
}
func (s *Server) read(t Tool, variant string) (json.RawMessage, error) {
	if !safeName.MatchString(variant) || variant == "." || variant == ".." {
		return nil, fmt.Errorf("unsafe variant")
	}
	path := filepath.Join(s.config.DataRoot, t.File)
	if variant != "default" {
		variantDir, err := securePath(s.config.Root, filepath.Join("variants", variant))
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(variantDir)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("variant directory missing")
		}
		overlay := filepath.Join("variants", variant, "ds", t.File)
		if _, err := os.Lstat(filepath.Join(s.config.Root, overlay)); err == nil {
			path = overlay
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	resolved, err := securePath(s.config.Root, path)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(resolved)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("fixture must be regular file")
	}
	body, err := io.ReadAll(io.LimitReader(file, s.config.MaxFileBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > s.config.MaxFileBytes {
		return nil, fmt.Errorf("fixture size limit exceeded")
	}
	if !json.Valid(body) {
		return nil, fmt.Errorf("fixture contains invalid JSON")
	}
	return body, nil
}
func securePath(root, relative string) (string, error) {
	if filepath.IsAbs(relative) {
		return "", fmt.Errorf("absolute fixture paths are prohibited")
	}
	path, err := filepath.EvalSymlinks(filepath.Join(root, relative))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("fixture path escapes root")
	}
	return path, nil
}
func toolResult(value any, failed bool) *schema.CallToolResult {
	body, err := json.Marshal(value)
	if err != nil {
		return toolError(err)
	}
	result := &schema.CallToolResult{Content: []schema.CallToolResultContentElem{schema.TextContent{Type: "text", Text: string(body)}}}
	if obj, ok := value.(map[string]any); ok {
		result.StructuredContent = obj
	}
	if failed {
		result.IsError = &failed
	}
	return result
}
func toolError(err error) *schema.CallToolResult {
	body, _ := json.Marshal(err)
	value := map[string]any{}
	_ = json.Unmarshal(body, &value)
	if len(value) == 0 {
		value = map[string]any{"code": "mockDataSourceError", "message": err.Error()}
	}
	return toolResult(value, true)
}
