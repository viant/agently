package patch

import (
	"context"
	_ "embed"
	"fmt"
	"reflect"
	"strings"
	"sync"

	mem "github.com/viant/agently/genai/memory"
	svc "github.com/viant/agently/genai/tool/service"
)

// Name of the system/patch action service.
const Name = "system/patch"

// Service exposes filesystem patching capabilities as a Fluxor action service.
// It is stateless – every method call operates with its own ephemeral Session.
type Service struct {
	mu       sync.Mutex
	sessions map[string]*Session // conversationID -> session
}

// New creates the patch service instance.
func New() *Service { return &Service{sessions: map[string]*Session{}} }

// Name returns service identifier.
func (s *Service) Name() string { return Name }

//go:embed doc/apply_spec.md
var applySpec string

//go:embed doc/diff_spec.md
var diffSpec string

//go:embed doc/snapshot_spec.md
var snapshotSpec string

//go:embed doc/commit_spec.md
var commitSpec string

//go:embed doc/rollback_spec.md
var rollbackSpec string

// Methods returns service method catalogue.
func (s *Service) Methods() svc.Signatures {
	return []svc.Signature{
		{
			Name:        "apply",
			Description: "Apply a patch to files. Requires workdir. Supports unified diff or simplified patch.",
			Input:       reflect.TypeOf(&ApplyInput{}),
			Output:      reflect.TypeOf(&ApplyOutput{}),
		},
		{
			Name:        "diff",
			Description: "Compute a patch between old and new content. Optional path and contextLines.",
			Input:       reflect.TypeOf(&DiffInput{}),
			Output:      reflect.TypeOf(&DiffOutput{}),
		},
		{
			Name:        "snapshot",
			Description: "List uncommitted changes captured by the current patch session.",
			Input:       reflect.TypeOf(&EmptyInput{}),
			Output:      reflect.TypeOf(&SnapshotOutput{}),
		},
		{
			Name:        "commit",
			Description: "Persist current patch session changes and clear the session.",
			Input:       reflect.TypeOf(&EmptyInput{}),
			Output:      reflect.TypeOf(&EmptyOutput{}),
		},
		{
			Name:        "rollback",
			Description: "Discard current patch session changes and clear the session.",
			Input:       reflect.TypeOf(&EmptyInput{}),
			Output:      reflect.TypeOf(&EmptyOutput{}),
		},
	}
}

// Method maps method names to executable handlers.
func (s *Service) Method(name string) (svc.Executable, error) {
	switch strings.ToLower(name) {
	case "apply":
		return s.apply, nil
	case "diff":
		return s.diff, nil
	case "snapshot":
		return s.snapshot, nil
	case "commit":
		return s.commit, nil
	case "rollback":
		return s.rollback, nil
	default:
		return nil, svc.NewMethodNotFoundError(name)
	}
}

// -------------------------------------------------------------------------
// method executors
// -------------------------------------------------------------------------

func (s *Service) apply(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*ApplyInput)
	if !ok {
		return svc.NewInvalidInputError(in)
	}
	output, ok := out.(*ApplyOutput)
	if !ok {
		return svc.NewInvalidOutputError(out)
	}
	output.Status = "ok"
	err := s.applyPatch(ctx, input, output)
	if err != nil {
		output.Error = err.Error()
		output.Status = "error"
	}
	return nil
}

func (s *Service) applyPatch(ctx context.Context, input *ApplyInput, output *ApplyOutput) error {
	// Enforce workdir to avoid accidental writes outside the intended workspace.
	// This prevents creating directories like "/internal" when workdir is missing.
	if strings.TrimSpace(input.Workdir) == "" {
		return fmt.Errorf("workdir is required for system/patch:apply")
	}
	convID := mem.ConversationIDFromContext(ctx)
	if convID == "" {
		convID = "_global"
	}
	s.mu.Lock()
	sess := s.sessions[convID]
	if sess == nil {
		var err error
		sess, err = NewSessionFor(convID)
		if err != nil {
			s.mu.Unlock()
			return err
		}
		s.sessions[convID] = sess
	}
	s.mu.Unlock()

	if err := sess.ApplyPatch(ctx, input.Patch, input.Workdir); err != nil {
		// Do not auto-rollback; leave the session active so the caller
		// can inspect snapshot and explicitly decide to commit or rollback.
		// Bubble up the error to the caller.
		return err
	}

	// Compute basic stats for user feedback.
	output.Stats = patchStats(input.Patch)
	// Session remains open for further apply calls until commit/rollback.
	return nil
}

// commit finalises the active session and clears it.
func (s *Service) commit(ctx context.Context, in, out interface{}) error {
	if _, ok := in.(*EmptyInput); !ok {
		return svc.NewInvalidInputError(in)
	}
	if _, ok := out.(*EmptyOutput); !ok {
		return svc.NewInvalidOutputError(out)
	}

	convID := mem.ConversationIDFromContext(ctx)
	if convID == "" {
		convID = "_global"
	}
	s.mu.Lock()
	sess := s.sessions[convID]
	s.mu.Unlock()
	if sess == nil {
		return nil
	}
	err := sess.Commit(ctx)
	s.mu.Lock()
	delete(s.sessions, convID)
	s.mu.Unlock()
	return err
}

// rollback aborts the active session and clears it.
func (s *Service) rollback(ctx context.Context, in, out interface{}) error {
	if _, ok := in.(*EmptyInput); !ok {
		return svc.NewInvalidInputError(in)
	}
	if _, ok := out.(*EmptyOutput); !ok {
		return svc.NewInvalidOutputError(out)
	}

	convID := mem.ConversationIDFromContext(ctx)
	if convID == "" {
		convID = "_global"
	}
	s.mu.Lock()
	sess := s.sessions[convID]
	s.mu.Unlock()
	if sess == nil {
		return nil
	}
	err := sess.Rollback(ctx)
	s.mu.Lock()
	delete(s.sessions, convID)
	s.mu.Unlock()
	return err
}

func (s *Service) diff(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*DiffInput)
	if !ok {
		return svc.NewInvalidInputError(in)
	}
	output, ok := out.(*DiffOutput)
	if !ok {
		return svc.NewInvalidOutputError(out)
	}

	// Context is not used in GenerateDiff, but we're keeping it for consistency
	res, stats, err := GenerateDiff([]byte(input.OldContent), []byte(input.NewContent), input.Path, input.ContextLines)
	if err != nil {
		return err
	}
	output.Patch = res
	output.Stats = stats
	return nil
}

// snapshot returns the list of changes tracked by the active session without mutating it.
func (s *Service) snapshot(ctx context.Context, in, out interface{}) error {
	if _, ok := in.(*EmptyInput); !ok {
		return svc.NewInvalidInputError(in)
	}
	output, ok := out.(*SnapshotOutput)
	if !ok {
		return svc.NewInvalidOutputError(out)
	}

	convID := mem.ConversationIDFromContext(ctx)
	if convID == "" {
		convID = "_global"
	}
	s.mu.Lock()
	sess := s.sessions[convID]
	s.mu.Unlock()

	output.Status = "ok"
	if sess == nil {
		output.Changes = nil
		output.Workdir = ""
		output.Status = "noFound"
		return nil
	}

	changes, err := sess.Snapshot(ctx)
	if err != nil {
		output.Status = "error"
		output.Error = err.Error()
	}
	if len(changes) == 0 {
		output.Status = "noFound"
	}
	output.Changes = changes
	output.Workdir = sess.Workdir
	return nil
}

// patchStats extracts basic statistics from a unified-diff string.
func patchStats(p string) DiffStats {
	stats := DiffStats{}
	for _, l := range strings.Split(p, "\n") {
		switch {
		case strings.HasPrefix(l, "+") && !strings.HasPrefix(l, "+++"):
			stats.Added++
		case strings.HasPrefix(l, "-") && !strings.HasPrefix(l, "---"):
			stats.Removed++
		}
	}
	return stats
}
