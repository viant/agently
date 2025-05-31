package core

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/viant/agently/genai/llm"
	"github.com/viant/fluxor/model/types"
	"sort"
	"strings"
)

type Rankable struct {
	Name        string `json:"Name"`
	Description string `json:"description"`
}

type Rank struct {
	ItemName  string    `json:"itemName"`
	Item      *Rankable `json:"item,omitempty"`
	Score     float64   `json:"score"`
	Reasoning string    `json:"reasoning,omitempty"`
}

type RankInput struct {
	Query      string      `json:"query"`
	Candidates []*Rankable `json:"candidates"`
	Model      string      `json:"model"`
}

type RankOutput struct {
	Ranked   []*Rank `json:"items"`
	Response *llm.GenerateResponse
	Content  string
}

// rank handles ranking requests
func (s *Service) rank(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*RankInput)
	if !ok {
		return types.NewInvalidInputError(in)
	}
	outputPtr, ok := out.(*RankOutput)
	if !ok {
		return types.NewInvalidOutputError(out)
	}

	err := s.RankItems(ctx, input, outputPtr)
	if err != nil {
		return err
	}

	return nil
}

// RankItems sends a request to the LLM to rank items based on a query
func (s *Service) RankItems(ctx context.Context, in *RankInput, output *RankOutput) error {
	if in.Model == "" {
		return fmt.Errorf("model is required")
	}
	if in.Query == "" {
		return fmt.Errorf("query is required")
	}
	if len(in.Candidates) == 0 {
		return fmt.Errorf("candidates are required")
	}

	prompt := buildRankingPrompt(in.Query, in.Candidates)

	model, err := s.llmFinder.Find(ctx, in.Model)
	if err != nil {
		return fmt.Errorf("failed to get model: %w", err)
	}

	// Create a message for the LLM
	messages := []llm.Message{
		llm.NewUserMessage(prompt),
	}

	// Create the request
	request := &llm.GenerateRequest{
		Messages: messages,
	}

	// Generate the response
	response, err := model.Generate(ctx, request)
	if err != nil {
		return fmt.Errorf("failed to generate content: %w", err)
	}

	// Extract content from response
	var content strings.Builder
	for _, choice := range response.Choices {
		for _, item := range choice.Message.Items {
			if item.Type == llm.ContentTypeText {
				if item.Data != "" {
					content.WriteString(item.Data)
				} else if item.Text != "" {
					content.WriteString(item.Text)
				}
			}
		}
	}
	contentStr := content.String()

	// Parse the ranked items from the LLM response
	ranked, err := extractRankedItems(contentStr, in.Candidates)
	if err != nil {
		return fmt.Errorf("failed to extract ranked items: %w", err)
	}

	output.Ranked = ranked
	output.Response = response
	output.Content = contentStr
	return nil
}

// buildRankingPrompt creates a prompt for the LLM to rank items based on relevance to a query
func buildRankingPrompt(query string, candidates []*Rankable) string {
	var sb strings.Builder

	sb.WriteString("Rank the following items by their relevance to this query: \"")
	sb.WriteString(query)
	sb.WriteString("\"\n\n")

	sb.WriteString("Items to rank:\n")
	for i, item := range candidates {
		sb.WriteString(fmt.Sprintf("%d. Name: %s\n   Description: %s\n\n", i+1, item.Name, item.Description))
	}
	sb.WriteString(`Please analyze each item's relevance to the query and assign a score from 0.0 to 1.0, 
where 1.0 means perfectly relevant and 0.0 means completely irrelevant.

Return your response in this exact JSON format:
{
  "rankings": [
    {
      "itemName": "Name of item",
      "score": 0.95,
      "reasoning": "brief explanation of why this score was assigned"
    },
    ...
  ]
}

Please ensure your response is valid JSON and includes all items.`)

	return sb.String()
}

// extractRankedItems parses the LLM response to extract ranked items
func extractRankedItems(content string, candidates []*Rankable) ([]*Rank, error) {
	// Find JSON content in the response
	startIdx := strings.Index(content, "{")
	endIdx := strings.LastIndex(content, "}")

	if startIdx == -1 || endIdx == -1 || endIdx <= startIdx {
		return nil, fmt.Errorf("could not find valid JSON in response")
	}

	jsonContent := content[startIdx : endIdx+1]
	type RankingResponse struct {
		Rankings []*Rank `json:"rankings"`
	}
	var response RankingResponse
	if err := json.Unmarshal([]byte(jsonContent), &response); err != nil {
		return nil, fmt.Errorf("failed to parse ranking response: %w", err)
	}

	// Map responses to Rank objects
	rankMap := make(map[string]*Rank)
	for _, entry := range response.Rankings {
		for _, candidate := range candidates {
			if strings.EqualFold(entry.ItemName, candidate.Name) {
				rankMap[candidate.Name] = &Rank{
					ItemName:  candidate.Name,
					Score:     entry.Score,
					Reasoning: entry.Reasoning,
					Item:      candidate,
				}
				break
			}
		}
	}

	// Create final ranked list
	var result []*Rank
	for _, candidate := range candidates {
		if rank, ok := rankMap[candidate.Name]; ok {
			result = append(result, rank)
		} else {
			// If item wasn't ranked, add it with a score of 0
			result = append(result, &Rank{
				ItemName: candidate.Name,
				Score:    0.0,
				Item:     candidate,
			})
		}
	}

	// Sort by score (highest to lowest)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Score > result[j].Score
	})

	return result, nil
}
