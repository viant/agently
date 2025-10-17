package extensionrepo

import (
	"context"
	"sort"
	"strings"

	"github.com/viant/afs"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/agently/internal/workspace"
	"github.com/viant/agently/internal/workspace/repository/base"
)

// Repository persists FeedSpec entries in the workspace under "extensions/".
// Each file stores a single FeedSpec document.
type Repository struct {
	primary *baserepo.Repository[tool.FeedSpec] // feeds
}

func (r *Repository) GetRaw(ctx context.Context, name string) ([]byte, error) {
	return r.primary.GetRaw(ctx, name)
}

func (r *Repository) Add(ctx context.Context, name string, data []byte) error {
	return r.primary.Add(ctx, name, data)
}

func (r *Repository) Delete(ctx context.Context, name string) error {
	return r.primary.Delete(ctx, name)
}

// New constructs a repository bound to the workspace extensions kind.
func New(fs afs.Service) *Repository {
	return &Repository{
		primary: baserepo.New[tool.FeedSpec](fs, workspace.KindFeeds),
	}
}

// List returns unique basenames from feeds/ (preferred) union extensions/ (legacy),
// preserving order with feeds first when duplicates exist.
func (r *Repository) List(ctx context.Context) ([]string, error) {
	return r.primary.List(ctx)
}

// Load tries feeds/ first then fallbacks to extensions/.
func (r *Repository) Load(ctx context.Context, name string) (*tool.FeedSpec, error) {
	return r.primary.Load(ctx, name)
}

// FindMatches returns all FeedSpec entries matching the provided service/method
// pair using simple wildcard semantics ("*" matches any). Results are ordered by
// specificity (exact service+method first), then by Priority (desc), then stable name order.
func (r *Repository) FindMatches(ctx context.Context, service, method string) ([]*tool.FeedSpec, error) {
	names, err := r.List(ctx)
	if err != nil {
		return nil, err
	}

	type item struct {
		name   string
		score  int // specificity score: 2=exact both, 1=one exact, 0=both wild
		prio   int
		record *tool.FeedSpec
	}
	var items []item

	for _, name := range names {
		rec, err := r.Load(ctx, name)
		if err != nil {
			return nil, err
		}
		if rec == nil {
			continue
		}
		if !matchField(rec.Match.Service, service) || !matchField(rec.Match.Method, method) {
			continue
		}
		sc := 0
		if isExact(rec.Match.Service) {
			sc++
		}
		if isExact(rec.Match.Method) {
			sc++
		}
		items = append(items, item{name: name, score: sc, prio: rec.Priority, record: rec})
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].score != items[j].score {
			return items[i].score > items[j].score
		}
		if items[i].prio != items[j].prio {
			return items[i].prio > items[j].prio
		}
		return items[i].name < items[j].name
	})

	var out []*tool.FeedSpec
	for _, it := range items {
		out = append(out, it.record)
	}
	return out, nil
}

// FindFirst returns the best matching metadata or false when none match.
func (r *Repository) FindFirst(ctx context.Context, service, method string) (*tool.FeedSpec, bool, error) {
	matches, err := r.FindMatches(ctx, service, method)
	if err != nil {
		return nil, false, err
	}
	if len(matches) == 0 {
		return nil, false, nil
	}
	return matches[0], true, nil
}

func matchField(pattern, value string) bool {
	p := strings.TrimSpace(pattern)
	if p == "" || p == "*" {
		return true
	}
	return strings.EqualFold(p, value)
}

func isExact(pattern string) bool {
	p := strings.TrimSpace(pattern)
	return p != "" && p != "*"
}
