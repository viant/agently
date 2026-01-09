package conversation

import (
	"context"
	"encoding/json"
	"path"
	"strings"

	"sort"
	"strconv"

	"github.com/minio/highwayhash"
	extx "github.com/viant/agently/genai/tool"
	"github.com/viant/agently/internal/conv"
	"github.com/viant/agently/pkg/agently/tool"
	"github.com/viant/agently/pkg/agently/tool/invoker"
	mcpname "github.com/viant/agently/pkg/mcpname"
	"github.com/viant/forge/backend/types"
)

// computeToolFeed computes ToolFeed for a transcript turn using configured FeedSpec.
// It supports activation.kind history/tool_call and scope all/last.
func (t *TranscriptView) computeToolFeed(ctx context.Context) ([]*tool.Feed, error) {
	input := InputFromContext(ctx)
	if input == nil || len(input.FeedSpec) == 0 {
		return nil, nil
	}

	specs := extx.FeedSpecs(input.FeedSpec)
	toolCallsByFeedID := map[string][]*ToolCallView{}
	for _, m := range t.Message {
		if m == nil || m.ToolCall == nil {
			continue
		}
		toolName := mcpname.Canonical(strings.TrimSpace(m.ToolCall.ToolName))
		key := mcpname.Name(toolName)
		for _, feed := range specs {
			if feed == nil {
				continue
			}
			if !feedMatchesTool(feed, key) {
				continue
			}
			if feed.ShallUseHistory() {
				if feed.Activation.Scope == "last" {
					toolCallsByFeedID[feed.ID] = nil
				}
				toolCallsByFeedID[feed.ID] = append(toolCallsByFeedID[feed.ID], m.ToolCall)
				continue
			}
			if len(toolCallsByFeedID[feed.ID]) > 0 {
				continue
			}
			if feed.ShallInvokeTool() { // invoke one per match
				inv := invoker.From(ctx)
				if inv == nil {
					// Tool invocation is optional (best-effort). When the invoker is not configured
					// we skip invoked feeds rather than failing transcript hydration.
					continue
				}
				svc, method := feed.InvokeServiceMethod()
				if strings.TrimSpace(svc) == "" {
					svc = key.Service()
				}
				if strings.TrimSpace(method) == "" {
					method = key.Method()
				}
				output, err := inv.Invoke(ctx, svc, method, feed.Activation.Args)
				if err != nil {
					return nil, err
				}
				var payload string
				switch actual := output.(type) {
				case string:
					payload = actual
					resp := map[string]interface{}{}
					if json.Unmarshal([]byte(actual), &resp) == nil {
						if _, ok := resp["status"]; ok && len(resp) == 1 {
							continue
						}
					}
				default:
					d, _ := json.Marshal(actual)
					payload = string(d)
				}
				toolCallName := svc + ":" + method
				call := &ToolCallView{ToolName: toolCallName, ResponsePayload: &ResponsePayloadView{}, RequestPayload: &ResponsePayloadView{}}
				call.ResponsePayload.InlineBody = &payload
				emptyBody := ""
				call.RequestPayload.InlineBody = &emptyBody
				toolCallsByFeedID[feed.ID] = append(toolCallsByFeedID[feed.ID], call)
			}
		}
	}
	var result []*tool.Feed
	for _, feed := range input.FeedSpec {
		if feed == nil {
			continue
		}
		toolCalls, ok := toolCallsByFeedID[feed.ID]
		if !ok || len(toolCalls) == 0 {
			continue
		}
		defaultTraceID := lastModelCallTraceID(t.Message)
		// Build a per-turn synthetic root so multiple independent DataSources can be derived
		// from the same tool call input/output using selectors.
		hash := hashSuffixFromTurnID(t.Id)
		rootName := feed.ID + "-root" + hash

		// Merge tool call input/output across matching calls into a single JSON-like root.
		// This supports activation.scope=all by combining slices and maps recursively.
		var mergedInput interface{}
		var mergedOutput interface{}
		for _, toolCall := range toolCalls {
			var toolCallInput interface{}
			var toolCallOutput interface{}
			if toolCall.RequestPayload != nil && toolCall.RequestPayload.InlineBody != nil {
				toolCallInput = parseJSONOrString(*toolCall.RequestPayload.InlineBody)
			}
			if toolCall.ResponsePayload != nil && toolCall.ResponsePayload.InlineBody != nil {
				toolCallOutput = parseJSONOrString(*toolCall.ResponsePayload.InlineBody)
			}
			mergedInput = mergeJSONLike(mergedInput, toolCallInput)
			mergedOutput = mergeJSONLike(mergedOutput, toolCallOutput)
		}
		// Ensure output/input are never nil so UI selectors like "output.commands"
		// don't panic when traversing null upstream roots.
		if mergedInput == nil {
			mergedInput = map[string]interface{}{}
		}
		if mergedOutput == nil {
			mergedOutput = map[string]interface{}{}
		}

		rootData := map[string]interface{}{
			"input":  mergedInput,
			"output": mergedOutput,
		}
		// Built-in: Explorer feed is expected to provide entries and ops,
		// even when legacy workspace specs don't explicitly declare them.
		if strings.EqualFold(strings.TrimSpace(feed.ID), "explorer") {
			rootData["entries"] = deriveExplorerEntries(mergedOutput)
			rootData["ops"] = deriveExplorerOps(toolCalls, defaultTraceID)
		} else {
			if feedWantsDataKey(feed, "entries") {
				rootData["entries"] = deriveExplorerEntries(mergedOutput)
			}
			if feedWantsDataKey(feed, "ops") {
				rootData["ops"] = deriveExplorerOps(toolCalls, defaultTraceID)
			}
		}

		// Add a per-turn suffix to data source names and rewire UI refs.
		mapping := make(map[string]string, len(feed.DataSources))
		for name := range feed.DataSources {
			mapping[name] = name + hash
		}
		normalizedDS := make(map[string]*types.DataSource, len(feed.DataSources))
		for name, ds := range feed.DataSources {
			if ds == nil {
				continue
			}
			copied := ds.DataSource // value copy
			// Rewrite existing refs, otherwise bind to synthetic root.
			if strings.TrimSpace(copied.DataSourceRef) == "" {
				copied.DataSourceRef = rootName
			} else if newRef, ok := mapping[copied.DataSourceRef]; ok {
				copied.DataSourceRef = newRef
			}
			// Promote `source:` into selectors.data (used by ToolFeed.jsx to derive collections)
			if copied.Selectors == nil {
				copied.Selectors = &types.Selectors{}
			}
			if strings.TrimSpace(copied.Selectors.Data) == "" && strings.TrimSpace(ds.Source) != "" {
				copied.Selectors.Data = strings.TrimSpace(ds.Source)
			}
			normalizedDS[mapping[name]] = &copied
		}

		rewrittenUI := extx.RewriteContainerDataSourceRefs(feed.UI, mapping)
		toolFeed := &tool.Feed{
			ID:          feed.ID,
			UI:          &rewrittenUI,
			DataSources: normalizedDS,
			Data:        extx.DataFeed{Name: rootName, Data: rootData, RawSelector: "root"},
		}
		result = append(result, toolFeed)
	}

	// Stable sort feeds by ID to keep tab order deterministic (e.g., Plan, Terminal)
	sort.SliceStable(result, func(i, j int) bool {
		return strings.Compare(result[i].ID, result[j].ID) < 0
	})

	for _, m := range t.Message {
		if m.ToolCall == nil {
			continue
		}
		// Reduce payload size when successful; keep payloads for failed calls so errors are visible to clients.
		status := strings.ToLower(strings.TrimSpace(m.ToolCall.Status))
		if status != "failed" {
			m.ToolCall.ResponsePayload = nil
			m.ToolCall.RequestPayload = nil
		}
	}
	return result, nil
}

func feedMatchesTool(feed *extx.FeedSpec, name mcpname.Name) bool {
	if feed == nil {
		return false
	}
	// Backward compatibility: older workspaces scoped the Explorer feed to grepFiles only,
	// but the UI and transcript feeds expect Explorer to cover resources list/read/search.
	if strings.EqualFold(strings.TrimSpace(feed.ID), "explorer") &&
		strings.EqualFold(strings.TrimSpace(feed.Match.Service), "resources") &&
		strings.EqualFold(strings.TrimSpace(feed.Match.Method), "grepFiles") {
		m := strings.ToLower(strings.TrimSpace(name.Method()))
		return strings.EqualFold(strings.TrimSpace(name.Service()), "resources") && (m == "grepfiles" || m == "read" || m == "list")
	}
	return feed.Match.Matches(name)
}

func lastModelCallTraceID(messages []*MessageView) string {
	if len(messages) == 0 {
		return ""
	}
	for i := len(messages) - 1; i >= 0; i-- {
		m := messages[i]
		if m == nil || m.ModelCall == nil || m.ModelCall.TraceId == nil {
			continue
		}
		if s := strings.TrimSpace(*m.ModelCall.TraceId); s != "" {
			return s
		}
	}
	return ""
}

func feedWantsDataKey(feed *extx.FeedSpec, key string) bool {
	if feed == nil || len(feed.DataSources) == 0 {
		return false
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	for _, ds := range feed.DataSources {
		if ds == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(ds.Source), key) {
			return true
		}
	}
	return false
}

func deriveExplorerEntries(output interface{}) []map[string]interface{} {
	output = unwrapToolOutput(output)
	outMap, ok := interfaceToMap(output)
	if !ok || len(outMap) == 0 {
		return nil
	}
	if items, ok := outMap["items"].([]interface{}); ok && len(items) > 0 {
		return normalizeEntrySlice(items)
	}
	if files, ok := outMap["files"].([]interface{}); ok && len(files) > 0 {
		return normalizeGrepFiles(files)
	}
	// resources.read-style output: normalize a single entry from uri/path fields.
	if uri := trimString(outMap["uri"]); uri != "" || trimString(outMap["URI"]) != "" {
		obj := map[string]interface{}{
			"uri":      uri,
			"path":     trimString(outMap["path"]),
			"name":     "",
			"size":     outMap["size"],
			"modified": outMap["modified"],
		}
		if obj["uri"] == "" {
			obj["uri"] = trimString(outMap["URI"])
		}
		if obj["path"] == "" {
			obj["path"] = trimString(outMap["Path"])
		}
		return []map[string]interface{}{normalizeEntry(obj)}
	}
	return nil
}

func deriveExplorerOps(toolCalls []*ToolCallView, defaultTraceID string) []map[string]interface{} {
	if len(toolCalls) == 0 {
		return nil
	}
	type opAgg struct {
		items []map[string]interface{} // {name, uri}
		seen  map[string]bool          // uri|name
	}
	type traceAgg map[string]*opAgg // operation -> agg
	aggByTrace := map[string]traceAgg{}
	traceOrder := make([]string, 0, 4)
	allowed := map[string]bool{"list": true, "read": true, "search": true}
	defaultTraceID = strings.TrimSpace(defaultTraceID)
	for _, toolCall := range toolCalls {
		if toolCall == nil {
			continue
		}
		traceID := ""
		if toolCall.TraceId != nil {
			traceID = strings.TrimSpace(*toolCall.TraceId)
		}
		if traceID == "" {
			traceID = defaultTraceID
		}
		if _, ok := aggByTrace[traceID]; !ok {
			aggByTrace[traceID] = traceAgg{}
			traceOrder = append(traceOrder, traceID)
		}
		canonical := mcpname.Canonical(strings.TrimSpace(toolCall.ToolName))
		name := mcpname.Name(canonical)
		operation := strings.ToLower(strings.TrimSpace(name.Method()))
		switch operation {
		case "grepfiles":
			operation = "search"
		case "list":
			operation = "list"
		case "read":
			operation = "read"
		}
		if !allowed[operation] {
			continue
		}
		a := aggByTrace[traceID][operation]
		if a == nil {
			a = &opAgg{seen: map[string]bool{}}
			aggByTrace[traceID][operation] = a
		}
		var req interface{}
		var out interface{}
		if toolCall.RequestPayload != nil && toolCall.RequestPayload.InlineBody != nil {
			req = parseJSONOrString(*toolCall.RequestPayload.InlineBody)
		}
		if toolCall.ResponsePayload != nil && toolCall.ResponsePayload.InlineBody != nil {
			out = unwrapToolOutput(parseJSONOrString(*toolCall.ResponsePayload.InlineBody))
		}
		entries := deriveExplorerEntries(out)
		if len(entries) == 0 {
			entries = deriveEntriesFromRequest(operation, req)
		}
		for _, e := range entries {
			uri := trimString(e["uri"])
			leaf := trimString(e["name"])
			if leaf == "" {
				leaf = path.Base(trimString(e["path"]))
			}
			hits := e["hits"]
			key := uri + "|" + leaf
			if a.seen[key] {
				continue
			}
			a.seen[key] = true
			item := map[string]interface{}{"name": leaf, "uri": uri}
			if hits != nil {
				item["hits"] = hits
			}
			a.items = append(a.items, item)
		}
	}

	order := []string{"list", "read", "search"}
	result := make([]map[string]interface{}, 0, len(toolCalls))
	appendOp := func(traceID, operation string, a *opAgg) {
		if a == nil {
			return
		}
		sort.SliceStable(a.items, func(i, j int) bool {
			ni := trimString(a.items[i]["name"])
			nj := trimString(a.items[j]["name"])
			if ni != nj {
				return ni < nj
			}
			ui := trimString(a.items[i]["uri"])
			uj := trimString(a.items[j]["uri"])
			return ui < uj
		})

		names := make([]string, 0, len(a.items))
		uris := make([]string, 0, len(a.items))
		for _, it := range a.items {
			n := trimString(it["name"])
			u := trimString(it["uri"])
			if n != "" {
				names = append(names, n)
			}
			if u != "" {
				uris = append(uris, u)
			}
		}
		sort.Strings(names)
		sort.Strings(uris)
		resources := joinLeafNames(names, 20)
		primaryURI := ""
		if len(uris) > 0 {
			primaryURI = uris[0]
		}
		traceShort := traceID
		if len(traceShort) > 8 {
			traceShort = traceShort[:8]
		}
		result = append(result, map[string]interface{}{
			"traceId":   traceID,
			"trace":     traceShort,
			"operation": operation,
			"resources": resources,
			"count":     len(names),
			"uri":       primaryURI,
			"items":     append([]map[string]interface{}(nil), a.items...),
			"uris":      append([]string(nil), uris...),
		})
	}

	for _, traceID := range traceOrder {
		traceAgg := aggByTrace[traceID]
		if len(traceAgg) == 0 {
			continue
		}
		seenOp := map[string]bool{}
		for _, op := range order {
			if a, ok := traceAgg[op]; ok {
				appendOp(traceID, op, a)
				seenOp[op] = true
			}
		}
		var remaining []string
		for op := range traceAgg {
			if !seenOp[op] {
				remaining = append(remaining, op)
			}
		}
		sort.Strings(remaining)
		for _, op := range remaining {
			appendOp(traceID, op, traceAgg[op])
		}
	}
	return result
}

// unwrapToolOutput normalizes common wrapper response shapes produced by invokers
// and tool runner endpoints so feed logic can work with the actual tool payload.
//
// Supported unwrap patterns (best-effort, repeated):
// - {status: "...", data: <payload>}
// - {data: {Result: "<json>"}} or {data: {result: "<json>"}}
// - {Result: "<json>"} or {result: "<json>"}
func unwrapToolOutput(value interface{}) interface{} {
	cur := value
	for i := 0; i < 6; i++ {
		switch actual := cur.(type) {
		case map[string]interface{}:
			// Prefer data envelope when present
			if v, ok := actual["data"]; ok && v != nil {
				cur = v
				continue
			}
			if v, ok := actual["Data"]; ok && v != nil {
				cur = v
				continue
			}
			// Common tool runner field: Result/result
			if v, ok := actual["Result"]; ok && v != nil {
				cur = v
				continue
			}
			if v, ok := actual["result"]; ok && v != nil {
				cur = v
				continue
			}
			return cur
		case string:
			parsed := parseJSONOrString(actual)
			// Stop if not parseable JSON or unchanged
			if s, ok := parsed.(string); ok && strings.TrimSpace(s) == strings.TrimSpace(actual) {
				return cur
			}
			cur = parsed
			continue
		default:
			return cur
		}
	}
	return cur
}

func deriveEntriesFromRequest(operation string, request interface{}) []map[string]interface{} {
	reqMap, ok := interfaceToMap(request)
	if !ok || len(reqMap) == 0 {
		return nil
	}
	switch operation {
	case "read":
		uri := trimString(reqMap["uri"])
		if uri == "" {
			uri = trimString(reqMap["URI"])
		}
		p := trimString(reqMap["path"])
		if p == "" {
			p = trimString(reqMap["Path"])
		}
		if uri == "" && p == "" {
			return nil
		}
		name := path.Base(p)
		if name == "." || name == "/" {
			name = path.Base(uri)
		}
		return []map[string]interface{}{{"uri": uri, "path": p, "name": name}}
	case "list", "search":
		// Best-effort: show the requested path as a single "resource" when response isn't available.
		p := trimString(reqMap["path"])
		if p == "" {
			p = trimString(reqMap["Path"])
		}
		if p == "" {
			return nil
		}
		return []map[string]interface{}{{"path": p, "name": path.Base(p)}}
	default:
		return nil
	}
}

func joinLeafNames(names []string, max int) string {
	if len(names) == 0 {
		return ""
	}
	if max <= 0 || len(names) <= max {
		return strings.Join(names, ", ")
	}
	head := append([]string(nil), names[:max]...)
	return strings.Join(head, ", ") + " … (+" + strconv.Itoa(len(names)-max) + ")"
}

func interfaceToMap(value interface{}) (map[string]interface{}, bool) {
	if value == nil {
		return nil, false
	}
	if outMap, ok := value.(map[string]interface{}); ok {
		return outMap, true
	}
	// Some tool outputs may be concrete structs; convert via JSON round-trip.
	buf, err := json.Marshal(value)
	if err != nil || len(buf) == 0 {
		return nil, false
	}
	var outMap map[string]interface{}
	if err := json.Unmarshal(buf, &outMap); err != nil || len(outMap) == 0 {
		return nil, false
	}
	return outMap, true
}

func normalizeEntrySlice(items []interface{}) []map[string]interface{} {
	entries := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		obj, ok := item.(map[string]interface{})
		if !ok || len(obj) == 0 {
			continue
		}
		entries = append(entries, normalizeEntry(obj))
	}
	return entries
}

func normalizeEntry(obj map[string]interface{}) map[string]interface{} {
	uri := trimString(obj["uri"])
	pathValue := trimString(obj["path"])
	name := trimString(obj["name"])
	if name == "" {
		name = path.Base(pathValue)
	}
	hits := obj["hits"]
	if hits == nil {
		hits = obj["Matches"]
	}
	return map[string]interface{}{
		"uri":      uri,
		"path":     pathValue,
		"name":     name,
		"size":     obj["size"],
		"modified": obj["modified"],
		"hits":     hits,
	}
}

func normalizeGrepFiles(files []interface{}) []map[string]interface{} {
	entries := make([]map[string]interface{}, 0, len(files))
	for _, item := range files {
		obj, ok := item.(map[string]interface{})
		if !ok || len(obj) == 0 {
			continue
		}
		uri := trimString(obj["URI"])
		pathValue := trimString(obj["Path"])
		name := path.Base(pathValue)
		entries = append(entries, map[string]interface{}{
			"uri":  uri,
			"path": pathValue,
			"name": name,
			"hits": obj["Matches"],
		})
	}
	return entries
}

func trimString(v interface{}) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func parseJSONOrString(data string) interface{} {
	raw := strings.TrimSpace(data)
	if raw == "" {
		return nil
	}
	var out interface{}
	if err := json.Unmarshal([]byte(raw), &out); err == nil {
		return out
	}
	return raw
}

func mergeJSONLike(prev, next interface{}) interface{} {
	if prev == nil {
		return next
	}
	if next == nil {
		return prev
	}
	switch p := prev.(type) {
	case map[string]interface{}:
		nm, ok := next.(map[string]interface{})
		if !ok {
			return next
		}
		out := make(map[string]interface{}, len(p)+len(nm))
		for k, v := range p {
			out[k] = v
		}
		for k, v := range nm {
			if existing, ok := out[k]; ok {
				out[k] = mergeJSONLike(existing, v)
			} else {
				out[k] = v
			}
		}
		return out
	case []interface{}:
		merged, _ := conv.MergeSlices(p, next)
		return merged
	default:
		// Try slice merge for non-map roots as a generic aggregation fallback.
		merged, _ := conv.MergeSlices(prev, next)
		return merged
	}
}

// hashSuffixFromTurnID computes a HighwayHash64-based integer suffix for a turn ID.
// It returns a string like "-123456789" for stable, compact DS/UI disambiguation.
func hashSuffixFromTurnID(id string) string {
	// Fixed 32-byte key for deterministic hashing (not a secret)
	var key [32]byte
	copy(key[:], []byte("agently:feed:ds:hash:key"))
	h, err := highwayhash.New64(key[:])
	if err != nil {
		// Fallback to raw id in unlikely event of failure
		return "-" + id
	}
	_, _ = h.Write([]byte(id))
	// Use signed base-10 for readability; cast from uint64
	v := int64(h.Sum64())
	return "-" + strconv.FormatInt(v, 10)
}
