package patch

// ApplyInput is the payload for Service.Apply
type ApplyInput struct {
	Patch   string `json:"patch" description:"Patch text to apply (either unified-diff or simplified patch format)"`
	Workdir string `json:"workdir" description:"Required. Base directory for all relative patch paths"`
}

// DiffInput is the payload for Service.Diff
type DiffInput struct {
	OldContent   string `json:"old" description:"Original content."`
	NewContent   string `json:"new" description:"Updated content."`
	Path         string `json:"path,omitempty" description:"Optional logical path used in patch headers (e.g., src/file.go)."`
	ContextLines int    `json:"contextLines,omitempty" description:"Patch context lines (default 3)."`
}

// DiffStats summarizes additions/removals in a patch string.
type DiffStats struct {
	Added   int `json:"added,omitempty"`
	Removed int `json:"removed,omitempty"`
}

// ApplyOutput summarises changes applied.
type ApplyOutput struct {
	Stats  DiffStats `json:"stats,omitempty"`
	Status string    `json:"status,omitempty"`
	Error  string    `json:"error,omitempty"`
}

// DiffOutput mirrors DiffResult for JSON tags.
type DiffOutput struct {
	Patch string    `json:"patch"`
	Stats DiffStats `json:"stats"`
}

// EmptyInput/Output used by commit/rollback/snapshot
type EmptyInput struct{}
type EmptyOutput struct{}

// Change represents a single tracked change in the active session.
type Change struct {
	Kind    string `json:"kind"`
	OrigURL string `json:"origUrl,omitempty"`
	URL     string `json:"url,omitempty"`
	Diff    string `json:"diff,omitempty"`
}

// SnapshotOutput lists the current uncommitted changes captured by the active session.
type SnapshotOutput struct {
	Workdir string   `json:"workdir,omitempty"`
	Changes []Change `json:"changes,omitempty"`
	Status  string   `json:"status,omitempty"`
	Error   string   `json:"error,omitempty"`
}
