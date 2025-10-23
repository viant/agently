package message

import (
	"context"
	"reflect"
	"strings"

	apiconv "github.com/viant/agently/client/conversation"
	"github.com/viant/agently/genai/embedder"
	core "github.com/viant/agently/genai/service/core"
	svc "github.com/viant/agently/genai/tool/service"
)

const Name = "internal/message"

// Service provides internal message utilities (hidden from metadata/UI).
type Service struct {
	conv                                                  apiconv.Client
	core                                                  coreGen
	embedder                                              embedder.Finder
	summarizeChunk, matchChunk                            int
	summaryModel, summaryPrompt, defaultModel, embedModel string
}

type coreGen interface {
	Generate(ctx context.Context, input *core.GenerateInput, output *core.GenerateOutput) error
}

// New creates a basic service; summarization/match require dependencies set via options or NewWithDeps.
func New(conv apiconv.Client) *Service { return &Service{conv: conv} }

// NewWithDeps provides full dependencies for summarize/match operations.
func NewWithDeps(conv apiconv.Client, core coreGen, emb embedder.Finder, summarizeChunk, matchChunk int, summaryModel, summaryPrompt, defaultModel, embedModel string) *Service {
	return &Service{conv: conv, core: core, embedder: emb, summarizeChunk: summarizeChunk, matchChunk: matchChunk, summaryModel: summaryModel, summaryPrompt: summaryPrompt, defaultModel: defaultModel, embedModel: embedModel}
}

func (s *Service) Name() string { return Name }

func (s *Service) Methods() svc.Signatures {
	// Note: internal/message:compact is intentionally NOT registered here.
	// Compaction is only used internally by the orchestrator to free space
	// for the Token‑Limit Presentation message. Normal cleanup should be
	// LLM-driven via listCandidates + remove (and optionally summarize).
	return []svc.Signature{
		{Name: "show", Description: "Show a message body with optional transform, range and sed. Make sure that either range, transform or set is set - otherwise you would only see preview", Input: reflect.TypeOf(&ShowInput{}), Output: reflect.TypeOf(&ShowOutput{})},
		{Name: "summarize", Description: "Produce concise, chunked summaries of a large message body plus a combined summary.", Input: reflect.TypeOf(&SummarizeInput{}), Output: reflect.TypeOf(&SummarizeOutput{})},
		{Name: "match", Description: "Retrieve top-K semantically relevant fragments from a large message body using the configured embedding model and cover-tree search.", Input: reflect.TypeOf(&MatchInput{}), Output: reflect.TypeOf(&MatchOutput{})},
		{Name: "listCandidates", Description: "List removable messages with byte/token size and concise preview.", Input: reflect.TypeOf(&ListCandidatesInput{}), Output: reflect.TypeOf(&ListCandidatesOutput{})},
		{Name: "remove", Description: "Create a summary message and archive selected messages (soft-remove).", Input: reflect.TypeOf(&RemoveInput{}), Output: reflect.TypeOf(&RemoveOutput{})},
	}
}

func (s *Service) Method(name string) (svc.Executable, error) {
	switch strings.ToLower(name) {
	case "show":
		return s.show, nil
	case "summarize":
		return s.summarize, nil
	case "match":
		return s.match, nil
	case "listcandidates":
		return s.listCandidates, nil
	case "remove":
		return s.remove, nil
	default:
		return nil, svc.NewMethodNotFoundError(name)
	}
}
