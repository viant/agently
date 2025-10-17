package patch

import (
	"context"
)

// internal change entry used for tracking and rollback
type changeEntry struct {
	kind   string // create|updated|delete
	orig   string
	url    string
	backup string // URL to original content snapshot when available
	alive  bool
	diff   string // unified diff for snapshot exposure
}

// helpers to maintain change state
func (s *Session) ensureEntry(orig, url string) *changeEntry {
	if url != "" {
		if e := s.byCurrent[url]; e != nil {
			return e
		}
	}
	if orig != "" {
		if e := s.byOrigin[orig]; e != nil {
			return e
		}
	}
	e := &changeEntry{orig: orig, url: url, alive: true}
	s.changes = append(s.changes, e)
	s.order = append(s.order, e)
	if url != "" {
		s.byCurrent[url] = e
	}
	if orig != "" {
		s.byOrigin[orig] = e
	}
	return e
}

func (s *Session) trackAdd(ctx context.Context, path string) {
	e := s.byCurrent[path]
	if e == nil {
		e = s.ensureEntry("", path)
	}
	e.kind = "create"
	e.alive = true
	// diff: empty old -> current
	newData, _ := s.fs.DownloadWithURL(ctx, path)
	diff, _, _ := GenerateDiff(nil, newData, path, 3)
	e.diff = diff
}

func (s *Session) trackMove(src, dst string) {
	e := s.byCurrent[src]
	if e == nil {
		e = s.ensureEntry(src, dst)
		e.kind = "updated"
		e.alive = true
		// move-only so far: diff empty
		e.diff = ""
		return
	}
	delete(s.byCurrent, src)
	e.url = dst
	s.byCurrent[dst] = e
	if e.kind == "" {
		e.kind = "updated"
	}
	if e.orig == "" && e.kind != "create" {
		e.orig = src
		s.byOrigin[e.orig] = e
	}
}

func (s *Session) trackUpdate(ctx context.Context, path, backup string) {
	e := s.byCurrent[path]
	if e == nil {
		e = s.ensureEntry(path, path)
	}
	if e.kind != "create" && e.backup == "" {
		e.backup = backup
	}
	if e.kind == "" {
		e.kind = "updated"
	}
	if e.orig == "" && e.kind != "create" {
		e.orig = path
		s.byOrigin[e.orig] = e
	}
	e.url = path
	e.alive = true
	// recompute diff based on backup/current
	var oldData []byte
	if e.backup != "" {
		oldData, _ = s.fs.DownloadWithURL(ctx, e.backup)
	}
	newData, _ := s.fs.DownloadWithURL(ctx, path)
	diff, _, _ := GenerateDiff(oldData, newData, path, 3)
	e.diff = diff
}

func (s *Session) trackDelete(ctx context.Context, path, backup string) {
	e := s.byCurrent[path]
	if e != nil && e.kind == "create" {
		// create then delete cancels out
		e.alive = false
		delete(s.byCurrent, path)
		if e.orig != "" {
			delete(s.byOrigin, e.orig)
		}
		return
	}
	if e == nil {
		e = s.ensureEntry(path, "")
	}
	if e.backup == "" {
		e.backup = backup
	}
	if e.orig == "" {
		e.orig = path
		s.byOrigin[e.orig] = e
	}
	e.kind = "delete"
	e.url = ""
	e.alive = true
	// diff old -> nil
	oldData, _ := s.fs.DownloadWithURL(ctx, backup)
	diff, _, _ := GenerateDiff(oldData, nil, path, 3)
	e.diff = diff
}

// Snapshot returns a list of proactive changes captured by the session.
func (s *Session) Snapshot(ctx context.Context) ([]Change, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Change, 0, len(s.order))
	for _, e := range s.order {
		if e == nil || !e.alive || e.kind == "" {
			continue
		}
		out = append(out, Change{
			Kind:    e.kind,
			OrigURL: e.orig,
			URL:     e.url,
			Diff:    e.diff,
		})
	}
	return out, nil
}
