// Package datasource connects Forge datasource services to MCP endpoints.
// Both report and window hosts can reuse it.
package datasource

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/viant/forge/backend/types"
	streamable "github.com/viant/jsonrpc/transport/client/http/streamable"
	"github.com/viant/mcp-protocol/schema"
	mcpclient "github.com/viant/mcp/client"
)

type Endpoint struct {
	Type      string `json:"type" yaml:"type"`
	BaseURL   string `json:"baseURL" yaml:"baseURL"`
	Transport string `json:"transport,omitempty" yaml:"transport,omitempty"`
}
type Client struct {
	client    *mcpclient.Client
	transport *streamable.Client
}
type RemoteError struct{ Body json.RawMessage }

func (e *RemoteError) Error() string { return string(e.Body) }
func Resolve(service *types.Service, endpoints map[string]Endpoint, hostURL string) (string, string, error) {
	if service == nil || service.Endpoint == "" || service.URI == "" {
		return "", "", fmt.Errorf("MCP datasource requires service.endpoint and service.uri")
	}
	endpoint, ok := endpoints[service.Endpoint]
	if !ok {
		return "", "", fmt.Errorf("unknown endpoint %q", service.Endpoint)
	}
	if endpoint.Type != "mcp" {
		return "", "", fmt.Errorf("endpoint %q must have type mcp", service.Endpoint)
	}
	if endpoint.Transport != "" && endpoint.Transport != "streamable" {
		return "", "", fmt.Errorf("only streamable MCP transport is supported")
	}
	u, err := url.Parse(endpoint.BaseURL)
	if err != nil {
		return "", "", err
	}
	if !u.IsAbs() {
		base, e := url.Parse(hostURL)
		if e != nil || base.Host == "" {
			return "", "", fmt.Errorf("relative MCP endpoint requires host URL")
		}
		u = base.ResolveReference(u)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Fragment != "" {
		return "", "", fmt.Errorf("invalid MCP endpoint URL")
	}
	return u.String(), service.URI, nil
}
func Dial(ctx context.Context, endpoint string) (*Client, error) {
	tr, err := streamable.New(ctx, endpoint, streamable.WithHTTPClient(&http.Client{Timeout: 60 * time.Second}), streamable.WithHandshakeTimeout(10*time.Second))
	if err != nil {
		return nil, err
	}
	cli := mcpclient.New("forge-datasource", "1.0.0", tr)
	if _, err = cli.Initialize(ctx); err != nil {
		_ = tr.Close()
		return nil, err
	}
	return &Client{cli, tr}, nil
}
func (c *Client) Close() { c.client.Close(); _ = c.transport.Close() }
func (c *Client) ListTools(ctx context.Context) (*schema.ListToolsResult, error) {
	return c.client.ListTools(ctx, nil)
}
func (c *Client) Call(ctx context.Context, tool string, args map[string]any) (json.RawMessage, error) {
	params, err := schema.NewCallToolRequestParams(tool, args)
	if err != nil {
		return nil, err
	}
	r, err := c.client.CallTool(ctx, params)
	if err != nil {
		return nil, err
	}
	var body []byte
	if r.StructuredContent != nil {
		body, err = json.Marshal(r.StructuredContent)
	} else {
		for _, v := range r.Content {
			switch text := v.(type) {
			case schema.TextContent:
				if text.Type == "text" {
					body = []byte(text.Text)
				}
			case map[string]any:
				if text["type"] == "text" {
					s, _ := text["text"].(string)
					body = []byte(s)
				}
			}
			if len(body) > 0 {
				break
			}
		}
	}
	if err != nil {
		return nil, err
	}
	if !json.Valid(body) {
		return nil, fmt.Errorf("MCP tool %s returned no JSON response body", tool)
	}
	if r.IsError != nil && *r.IsError {
		return nil, &RemoteError{Body: body}
	}
	return body, nil
}
