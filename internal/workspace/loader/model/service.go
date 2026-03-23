package model

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/viant/agently/genai/llm/provider"
	"github.com/viant/agently/internal/workspace"
	fs2 "github.com/viant/agently/internal/workspace/loader/fs"
)

const (
	defaultExtension = ".yaml"
)

// Service provides model data access operations
type Service struct {
	*fs2.Service[provider.Config]
}

// New creates a new model service instance
func New(options ...fs2.Option[provider.Config]) *Service {
	ret := &Service{
		Service: fs2.New[provider.Config](decodeYaml, options...),
	}
	return ret
}

// Load resolves bare model names against the standard workspace folder before
// delegating to the generic FS loader so that callers can simply refer to
// "o3" instead of "models/o4-mini.yaml".
func (s *Service) Load(ctx context.Context, URL string) (*provider.Config, error) {
	var lastErr error
	for _, candidate := range modelCandidates(URL) {
		cfg, err := s.Service.Load(ctx, candidate)
		if err == nil && cfg != nil {
			return cfg, nil
		}
		if err != nil {
			lastErr = err
		}
		if candidate == URL {
			continue
		}
		URL = candidate
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return s.Service.Load(ctx, URL)
}

func modelCandidates(URL string) []string {
	raw := strings.TrimSpace(URL)
	if raw == "" {
		return []string{URL}
	}
	if strings.Contains(raw, "/") {
		return []string{raw}
	}
	ext := strings.ToLower(filepath.Ext(raw))
	if ext == ".yaml" || ext == ".yml" || ext == ".json" {
		return []string{raw}
	}
	added := map[string]bool{}
	result := make([]string, 0, 4)
	add := func(name string) {
		if strings.TrimSpace(name) == "" || added[name] {
			return
		}
		added[name] = true
		result = append(result, filepath.Join(workspace.KindModel, name))
	}
	addVariants := func(name string) {
		add(name)
		positions := make([]int, 0, strings.Count(name, "-"))
		for idx, r := range name {
			if r == '-' {
				positions = append(positions, idx)
			}
		}
		total := 1 << len(positions)
		for mask := 1; mask < total; mask++ {
			bytes := []byte(name)
			for i, pos := range positions {
				if mask&(1<<i) != 0 {
					bytes[pos] = '_'
				}
			}
			add(string(bytes))
		}
	}
	addVariants(raw)
	addVariants(strings.ReplaceAll(raw, ".", "_"))
	return result
}
