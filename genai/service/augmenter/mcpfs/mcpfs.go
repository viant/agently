package mcpfs

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/viant/afs/storage"
	"github.com/viant/agently/genai/memory"
	mcpmgr "github.com/viant/agently/internal/mcp/manager"
	mcpuri "github.com/viant/agently/internal/mcp/uri"
	mcpschema "github.com/viant/mcp-protocol/schema"
)

// Service implements embedius fs.Service for MCP resources.
// It lists and downloads resources via a per-conversation MCP client manager.
type Service struct {
	mgr *mcpmgr.Manager
}

// New returns an MCP-backed fs service.
func New(mgr *mcpmgr.Manager) *Service {
	return &Service{mgr: mgr}
}

// List returns MCP resources under the given location prefix.
// Accepts formats: mcp://server/path or mcp:server:/path
func (s *Service) List(ctx context.Context, location string) ([]storage.Object, error) {
	if s == nil || s.mgr == nil {
		return nil, fmt.Errorf("mcpfs: manager not configured")
	}
	server, prefix := mcpuri.Parse(location)
	if strings.TrimSpace(server) == "" {
		return nil, fmt.Errorf("mcpfs: invalid location: %s", location)
	}
	convID := memory.ConversationIDFromContext(ctx)
	if strings.TrimSpace(convID) == "" {
		if tm, ok := memory.TurnMetaFromContext(ctx); ok {
			convID = tm.ConversationID
		}
	}
	if strings.TrimSpace(convID) == "" {
		return nil, fmt.Errorf("mcpfs: missing conversation id in context")
	}
	cli, err := s.mgr.Get(ctx, convID, server)
	if err != nil {
		return nil, fmt.Errorf("mcpfs: get client: %w", err)
	}
	if _, err := cli.Initialize(ctx); err != nil {
		return nil, fmt.Errorf("mcpfs: init: %w", err)
	}

	var out []storage.Object
	var cursor *string
	for {
		res, err := cli.ListResources(ctx, cursor)
		if err != nil {
			return nil, fmt.Errorf("mcpfs: list resources: %w", err)
		}
		for _, r := range res.Resources {
			if prefix != "" && !strings.HasPrefix(r.Uri, prefix) {
				continue
			}
			out = append(out, newObject(server, r))
		}
		if res.NextCursor == nil || strings.TrimSpace(*res.NextCursor) == "" {
			break
		}
		cursor = res.NextCursor
	}
	return out, nil
}

// Download reads the MCP resource contents for the given object.
func (s *Service) Download(ctx context.Context, object storage.Object) ([]byte, error) {
	if s == nil || s.mgr == nil {
		return nil, fmt.Errorf("mcpfs: manager not configured")
	}
	mcpURL := object.URL()
	server, uri := mcpuri.Parse(mcpURL)
	if strings.TrimSpace(server) == "" || strings.TrimSpace(uri) == "" {
		return nil, fmt.Errorf("mcpfs: invalid mcp url: %s", mcpURL)
	}
	convID := memory.ConversationIDFromContext(ctx)
	if strings.TrimSpace(convID) == "" {
		if tm, ok := memory.TurnMetaFromContext(ctx); ok {
			convID = tm.ConversationID
		}
	}
	if strings.TrimSpace(convID) == "" {
		return nil, fmt.Errorf("mcpfs: missing conversation id in context")
	}
	cli, err := s.mgr.Get(ctx, convID, server)
	if err != nil {
		return nil, fmt.Errorf("mcpfs: get client: %w", err)
	}
	if _, err := cli.Initialize(ctx); err != nil {
		return nil, fmt.Errorf("mcpfs: init: %w", err)
	}

	res, err := cli.ReadResource(ctx, &mcpschema.ReadResourceRequestParams{Uri: uri})
	if err != nil {
		return nil, fmt.Errorf("mcpfs: read resource: %w", err)
	}
	var data []byte
	for _, c := range res.Contents {
		if c.Text != "" {
			data = append(data, []byte(c.Text)...)
			continue
		}
		if c.Blob != "" {
			if dec, err := base64.StdEncoding.DecodeString(c.Blob); err == nil {
				data = append(data, dec...)
			}
		}
	}
	return data, nil
}

// -------------------- helpers --------------------

// object implements storage.Object over an MCP resource entry.
type object struct {
	server string
	uri    string
	name   string
	size   int64
	isDir  bool
	url    string
	mod    time.Time
	src    interface{}
}

func newObject(server string, r mcpschema.Resource) storage.Object {
	size := int64(0)
	if r.Size != nil {
		size = int64(*r.Size)
	}
	name := r.Name
	if name == "" {
		name = path.Base(r.Uri)
	}
	return &object{
		server: server,
		uri:    r.Uri,
		name:   name,
		size:   size,
		url:    "mcp:" + server + ":" + r.Uri,
		mod:    time.Now(),
		isDir:  false,
		src:    r,
	}
}

// NewObjectFromURI builds a minimal storage.Object for a given mcp URL.
// It is useful for direct downloads when a full Resource descriptor is not available.
func NewObjectFromURI(mcpURL string) storage.Object {
	server, uri := mcpuri.Parse(mcpURL)
	name := path.Base(uri)
	return &object{
		server: server,
		uri:    uri,
		name:   name,
		size:   0,
		url:    mcpURL,
		mod:    time.Now(),
		isDir:  false,
		src:    nil,
	}
}

// ---- os.FileInfo ----
func (o *object) Name() string       { return o.name }
func (o *object) Size() int64        { return o.size }
func (o *object) Mode() os.FileMode  { return 0o444 }
func (o *object) ModTime() time.Time { return o.mod }
func (o *object) IsDir() bool        { return o.isDir }
func (o *object) Sys() interface{}   { return nil }

// ---- storage.Object ----
func (o *object) URL() string          { return o.url }
func (o *object) Wrap(src interface{}) { o.src = src }
func (o *object) Unwrap(dst interface{}) error {
	// Best-effort assign when types match
	if dst == nil || o.src == nil {
		return nil
	}
	if p, ok := dst.(*mcpschema.Resource); ok {
		if v, ok := o.src.(mcpschema.Resource); ok {
			*p = v
			return nil
		}
	}
	return nil
}
