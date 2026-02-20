package executil

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"sync"

	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/tool"
)

type matchDocumentsOutput struct {
	Documents []matchedDocument `json:"documents"`
}

type matchedDocument struct {
	URI    string  `json:"uri"`
	RootID string  `json:"rootId,omitempty"`
	Score  float32 `json:"score"`
}

func persistDocumentsIfNeeded(ctx context.Context, reg tool.Registry, conv apiconv.Client, turn memory.TurnMeta, toolName, result string) error {
	if conv == nil || strings.TrimSpace(result) == "" {
		return nil
	}
	if isTrivialSystemDocPayload(result) {
		return nil
	}
	if !isMatchDocumentsTool(toolName) {
		return nil
	}
	var payload matchDocumentsOutput
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		return fmt.Errorf("decode matchDocuments result: %w", err)
	}
	if len(payload.Documents) == 0 || reg == nil {
		return nil
	}
	roots, err := fetchAgentRoots(ctx, reg, strings.TrimSpace(turn.Assistant))
	if err != nil {
		return err
	}
	if len(roots) == 0 {
		return nil
	}
	seen := map[string]bool{}
	for _, doc := range payload.Documents {
		loc := strings.TrimSpace(doc.URI)
		if loc == "" {
			continue
		}
		if seen[loc] {
			continue
		}
		seen[loc] = true
		root := findRootByID(roots, doc.RootID)
		relPath := ""
		if root != nil {
			relPath = relativePathForRoot(root.URI, loc)
		} else {
			root, relPath = matchRootForDocument(roots, loc)
		}
		if root == nil {
			continue
		}
		content, err := readDocumentContent(ctx, reg, root, relPath, loc)
		if err != nil || strings.TrimSpace(content) == "" {
			continue
		}
		formatted := formatDocumentForTranscript(loc, content)
		if strings.TrimSpace(formatted) == "" {
			continue
		}
		role := "user"
		if strings.EqualFold(strings.TrimSpace(root.Role), "system") {
			role = "system"
		}
		tags := ResourceDocumentTag
		if role == "system" {
			tags = tags + "," + SystemDocumentTag
		}
		opts := []apiconv.MessageOption{
			apiconv.WithRole(role),
			apiconv.WithType("text"),
			apiconv.WithContextSummary(loc),
			apiconv.WithContent(formatted),
			apiconv.WithRawContent(formatted),
			apiconv.WithTags(tags),
		}
		if role == "system" {
			opts = append(opts, apiconv.WithMode(SystemDocumentMode))
		}
		msg, err := apiconv.AddMessage(ctx, conv, &turn, opts...)
		if err != nil {
			return fmt.Errorf("persist system document message: %w", err)
		}
		if role == "system" {
			if err := ensureSystemDocMetadata(ctx, conv, msg.Id, loc); err != nil {
				return fmt.Errorf("update system doc metadata: %w", err)
			}
		}
	}
	return nil
}

func isTrivialSystemDocPayload(result string) bool {
	trimmed := strings.TrimSpace(result)
	switch strings.ToLower(trimmed) {
	case "", "null", "{}", "[]", "false", "true":
		return true
	}
	return false
}

func isMatchDocumentsTool(toolName string) bool {
	key := strings.ToLower(strings.TrimSpace(toolName))
	key = strings.ReplaceAll(key, "-", ".")
	key = strings.ReplaceAll(key, "_", ".")
	key = strings.ReplaceAll(key, " ", "")
	return key == "resources.matchdocuments"
}

type resourceRoot struct {
	ID   string `json:"id"`
	URI  string `json:"uri"`
	Role string `json:"role"`
}

type resourcesRootsResponse struct {
	Roots []resourceRoot `json:"roots"`
}

type resourcesReadResponse struct {
	Content string `json:"content"`
}

var agentRootsCache = struct {
	mu    sync.RWMutex
	items map[string][]resourceRoot
}{
	items: map[string][]resourceRoot{},
}

func fetchAgentRoots(ctx context.Context, reg tool.Registry, agentID string) ([]resourceRoot, error) {
	id := strings.TrimSpace(agentID)
	if cached := cachedRoots(id); len(cached) > 0 {
		return cached, nil
	}
	var resp resourcesRootsResponse
	if err := callResourcesTool(ctx, reg, "roots", map[string]interface{}{}, &resp); err != nil {
		return nil, err
	}
	if len(resp.Roots) == 0 {
		return nil, nil
	}
	storeRoots(id, resp.Roots)
	return resp.Roots, nil
}

func cachedRoots(agentID string) []resourceRoot {
	if agentID == "" {
		return nil
	}
	agentRootsCache.mu.RLock()
	defer agentRootsCache.mu.RUnlock()
	if roots, ok := agentRootsCache.items[agentID]; ok {
		return roots
	}
	return nil
}

func storeRoots(agentID string, roots []resourceRoot) {
	if agentID == "" || len(roots) == 0 {
		return
	}
	agentRootsCache.mu.Lock()
	agentRootsCache.items[agentID] = append([]resourceRoot(nil), roots...)
	agentRootsCache.mu.Unlock()
}

func findRootByID(roots []resourceRoot, id string) *resourceRoot {
	if len(roots) == 0 || strings.TrimSpace(id) == "" {
		return nil
	}
	target := strings.ToLower(strings.TrimSpace(id))
	for i := range roots {
		candidate := strings.ToLower(strings.TrimSpace(roots[i].ID))
		if candidate == "" {
			candidate = strings.ToLower(strings.TrimSpace(roots[i].URI))
		}
		if candidate == "" {
			continue
		}
		if candidate == target {
			return &roots[i]
		}
	}
	return nil
}

func relativePathForRoot(rootURI, uri string) string {
	root := strings.TrimSuffix(strings.TrimSpace(rootURI), "/")
	target := strings.TrimSpace(uri)
	if root == "" || target == "" {
		return ""
	}
	if !strings.HasPrefix(target, root) {
		return target
	}
	return strings.TrimPrefix(target[len(root):], "/")
}

func matchRootForDocument(roots []resourceRoot, uri string) (*resourceRoot, string) {
	target := strings.TrimSpace(uri)
	if target == "" || len(roots) == 0 {
		return nil, ""
	}
	var (
		selected *resourceRoot
		relPath  string
		maxLen   int
	)
	for i := range roots {
		rootURI := strings.TrimRight(strings.TrimSpace(roots[i].URI), "/")
		if rootURI == "" {
			continue
		}
		if rootURI == target {
			if len(rootURI) >= maxLen {
				selected = &roots[i]
				relPath = ""
				maxLen = len(rootURI)
			}
			continue
		}
		if strings.HasPrefix(target, rootURI+"/") {
			rel := strings.TrimPrefix(target[len(rootURI):], "/")
			if len(rootURI) > maxLen {
				selected = &roots[i]
				relPath = rel
				maxLen = len(rootURI)
			}
		}
	}
	return selected, relPath
}

func readDocumentContent(ctx context.Context, reg tool.Registry, root *resourceRoot, relativePath, uri string) (string, error) {
	if root == nil {
		return "", fmt.Errorf("root not resolved for %s", uri)
	}
	args := map[string]interface{}{
		"rootId": strings.TrimSpace(root.ID),
	}
	if relativePath != "" {
		args["path"] = relativePath
	} else if strings.TrimSpace(uri) != "" {
		args["uri"] = uri
	}
	var resp resourcesReadResponse
	if err := callResourcesTool(ctx, reg, "read", args, &resp); err != nil {
		return "", err
	}
	return resp.Content, nil
}

func callResourcesTool(ctx context.Context, reg tool.Registry, method string, args map[string]interface{}, dest interface{}) error {
	if reg == nil {
		return fmt.Errorf("tool registry not configured")
	}
	if args == nil {
		args = map[string]interface{}{}
	}
	var lastErr error
	for _, toolName := range resourceToolNames(method) {
		raw, err := reg.Execute(ctx, toolName, args)
		if err != nil {
			lastErr = err
			continue
		}
		if dest == nil || strings.TrimSpace(raw) == "" {
			return nil
		}
		if err := json.Unmarshal([]byte(raw), dest); err != nil {
			return fmt.Errorf("decode %s output: %w", toolName, err)
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("resources tool %s not found", strings.TrimSpace(method))
}

func resourceToolNames(method string) []string {
	trimmed := strings.TrimSpace(strings.TrimLeft(method, "."))
	if trimmed == "" {
		return []string{"resources.roots"}
	}
	var names []string
	dotted := "resources." + trimmed
	names = append(names, dotted)
	dashed := strings.ReplaceAll(dotted, ".", "-")
	if dashed != dotted {
		names = append(names, dashed)
	}
	if !strings.HasPrefix(trimmed, "resources") {
		alt := "resources-" + trimmed
		if alt != dashed {
			names = append(names, alt)
		}
	}
	return names
}

func formatDocumentForTranscript(loc, content string) string {
	loc = strings.TrimSpace(loc)
	if loc == "" && strings.TrimSpace(content) == "" {
		return ""
	}
	ext := strings.Trim(strings.TrimPrefix(path.Ext(loc), "."), "")
	return fmt.Sprintf("file: %v\n```%v\n%v\n````\n\n", loc, ext, content)
}

func ensureSystemDocMetadata(ctx context.Context, conv apiconv.Client, messageID, location string) error {
	if conv == nil || strings.TrimSpace(messageID) == "" {
		return nil
	}
	upd := apiconv.NewMessage()
	upd.SetId(messageID)
	upd.SetMode(SystemDocumentMode)
	apiconv.WithTags(ResourceDocumentTag + "," + SystemDocumentTag)(upd)
	if trimmed := strings.TrimSpace(location); trimmed != "" {
		apiconv.WithContextSummary(trimmed)(upd)
	}
	return conv.PatchMessage(ctx, upd)
}
