package extensionfinder

import (
	"sort"
	"strings"
	"sync"

	toolext "github.com/viant/agently/genai/tool"
)

// Finder keeps FeedSpec entries in-memory and supports matching by
// service/method with wildcard semantics.
type Finder struct {
	mu     sync.RWMutex
	byName map[string]*toolext.FeedSpec
}

func New() *Finder { return &Finder{byName: map[string]*toolext.FeedSpec{}} }

func (f *Finder) Add(name string, m *toolext.FeedSpec) {
	if m == nil || name == "" {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.byName[name] = m
}

func (f *Finder) Remove(name string) {
	if name == "" {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.byName, name)
}

func (f *Finder) Get(name string) (*toolext.FeedSpec, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	v, ok := f.byName[name]
	return v, ok
}

func (f *Finder) All() []*toolext.FeedSpec {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]*toolext.FeedSpec, 0, len(f.byName))
	for _, v := range f.byName {
		out = append(out, v)
	}
	return out
}

// FindMatches returns ordered matches for service/method with wildcards.
func (f *Finder) FindMatches(service, method string) []*toolext.FeedSpec {
	f.mu.RLock()
	defer f.mu.RUnlock()
	type item struct {
		name        string
		score, prio int
		rec         *toolext.FeedSpec
	}
	var items []item
	for name, rec := range f.byName {
		if rec == nil {
			continue
		}
		if !match(rec.Match.Service, service) || !match(rec.Match.Method, method) {
			continue
		}
		sc := 0
		if exact(rec.Match.Service) {
			sc++
		}
		if exact(rec.Match.Method) {
			sc++
		}
		items = append(items, item{name: name, score: sc, prio: rec.Priority, rec: rec})
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
	out := make([]*toolext.FeedSpec, 0, len(items))
	for _, it := range items {
		out = append(out, it.rec)
	}
	return out
}

func match(pattern, value string) bool {
	p := strings.TrimSpace(pattern)
	if p == "" || p == "*" {
		return true
	}
	return p == value
}

func exact(pattern string) bool { p := strings.TrimSpace(pattern); return p != "" && p != "*" }
