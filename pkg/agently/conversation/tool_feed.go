package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"sort"
	"strconv"

	"github.com/minio/highwayhash"
	extx "github.com/viant/agently/genai/tool"
	"github.com/viant/agently/internal/conv"
	"github.com/viant/agently/pkg/agently/tool"
	"github.com/viant/agently/pkg/agently/tool/invoker"
	selres "github.com/viant/agently/pkg/agently/tool/resolver"
	mcpname "github.com/viant/agently/pkg/mcpname"
)

// computeToolFeed computes ToolFeed for a transcript turn using configured FeedSpec.
// It supports activation.kind history/tool_call and scope all/last.
func (t *TranscriptView) computeToolFeed(ctx context.Context) ([]*tool.Feed, error) {
	input := InputFromContext(ctx)
	if len(input.FeedSpec) == 0 {
		return nil, nil
	}

	feeds := extx.FeedSpecs(input.FeedSpec).Index()
	// Compile matcher index from specs (union of all patterns)
	// Note: for filtering we use extx.FeedSpecs.Matches directly below

	// Collect tool calls in this turn keyed by canonical tool name
	// and capture the set of present tool names
	toolCallsByName := map[mcpname.Name][]*ToolCallView{}
	for _, m := range t.Message {
		if m == nil || m.ToolCall == nil {
			continue
		}
		toolName := mcpname.Canonical(strings.TrimSpace(m.ToolCall.ToolName))
		key := mcpname.Name(toolName)
		if _, ok := feeds[key]; !ok {
			continue
		}
		feed := feeds[key]
		if feed.ShallUseHistory() {
			if feed.Activation.Scope == "last" {
				toolCallsByName[key] = nil
			}
			toolCallsByName[key] = append(toolCallsByName[key], m.ToolCall)
			continue
		}
		if _, ok := toolCallsByName[key]; ok {
			continue
		}
		if feed.ShallInvokeTool() { //invoke one per match
			inv := invoker.From(ctx)
			if inv == nil {
				return nil, errors.New("tool service was empty")
			}
			method := feed.Activation.Method
			if method == "" {
				method = key.Method()
			}
			output, err := inv.Invoke(ctx, key.Service(), method, feed.Activation.Args)
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
			call := &ToolCallView{ToolName: toolName, ResponsePayload: &ResponsePayloadView{}, RequestPayload: &ResponsePayloadView{}}
			call.ResponsePayload.InlineBody = &payload
			emptyBody := ""
			call.RequestPayload.InlineBody = &emptyBody
			toolCallsByName[key] = append(toolCallsByName[key], call)
		}

	}
	var result []*tool.Feed
	for _, feed := range input.FeedSpec {
		key := feed.Match.Name()
		toolCalls, ok := toolCallsByName[key]
		if !ok {
			// Case-insensitive fallback by matching on lower-cased canonical
			lcKey := mcpname.Name(strings.ToLower(string(key)))
			if v, ok2 := toolCallsByName[lcKey]; ok2 {
				toolCalls, ok = v, true
			}
		}
		if !ok {
			continue
		}
		// Add a per-turn suffix to data source names and rewire UI refs using HighwayHash64(turnId)
		hash := hashSuffixFromTurnID(t.Id)
		normalizedDS, mapping := feed.DataSources.Transform(hash)
		rewrittenUI := extx.RewriteContainerDataSourceRefs(feed.UI, mapping)
		toolFeed := &tool.Feed{
			ID:          feed.ID,
			UI:          &rewrittenUI,
			DataSources: normalizedDS,
		}

		feedDataSource, err := feed.DataSources.FeedDataSource()
		if err != nil {
			return nil, err
		}

		var toolFeedData interface{}
		for _, toolCall := range toolCalls {
			var toolCallInput, toolCallOutput interface{}
			if toolCall.RequestPayload != nil && toolCall.RequestPayload.InlineBody != nil {
				toolCallInput, _ = stringToData(*toolCall.RequestPayload.InlineBody)
			}
			if toolCall.ResponsePayload != nil && toolCall.ResponsePayload.InlineBody != nil {
				toolCallOutput, _ = stringToData(*toolCall.ResponsePayload.InlineBody)
			}
			extracted := selres.Select(feedDataSource.Source, toolCallInput, toolCallOutput)
			if extracted != nil {
				if toolFeedData != nil {
					merged, err := conv.MergeSlices(toolFeedData, extracted)
					if err != nil {
						return nil, err
					}
					toolFeedData = merged
				} else {
					toolFeedData = extracted
				}
			}
		}
		// Use hashed data source name when present
		hashedName := feedDataSource.Name
		if newName, ok := mapping[feedDataSource.Name]; ok {
			hashedName = newName
		}
		toolFeed.Data = extx.DataFeed{Name: hashedName, Data: toolFeedData, RawSelector: feedDataSource.Source}
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
		//reduce output size - we already have tool feed
		m.ToolCall.ResponsePayload = nil
		m.ToolCall.RequestPayload = nil
	}
	return result, nil
}

func dataToString(data interface{}) (string, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(jsonData), nil
}

func stringToData(data string) (interface{}, error) {
	var out interface{}
	err := json.Unmarshal([]byte(data), &out)
	return out, err
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
