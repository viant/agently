package patch_test

import (
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/viant/afs"
	"github.com/viant/fluxor/service/action/system/patch"
	"strings"
	"testing"
)

func TestSession_Snapshot(t *testing.T) {
	fs := afs.New()
	ctx := context.Background()

	type tc struct {
		name         string
		initialFiles map[string]string
		ops          func(context.Context, *patch.Session) error
		expect       func() []patch.Change
	}

	cases := []tc{
		{
			name:         "create",
			initialFiles: map[string]string{},
			ops: func(ctx context.Context, s *patch.Session) error {
				return s.Add(ctx, "mem://localhost/new.txt", []byte("l1\nl2\n"))
			},
			expect: func() []patch.Change {
				diff, _, _ := patch.GenerateDiff(nil, []byte("l1\nl2\n"), "mem://localhost/new.txt", 3)
				return []patch.Change{{
					Kind:    "create",
					OrigURL: "",
					URL:     "mem://localhost/new.txt",
					Diff:    diff,
				}}
			},
		},
		{
			name: "update",
			initialFiles: map[string]string{
				"mem://localhost/a.txt": "A\nB\n",
			},
			ops: func(ctx context.Context, s *patch.Session) error {
				return s.Update(ctx, "mem://localhost/a.txt", []byte("A\nC\n"))
			},
			expect: func() []patch.Change {
				diff, _, _ := patch.GenerateDiff([]byte("A\nB\n"), []byte("A\nC\n"), "mem://localhost/a.txt", 3)
				return []patch.Change{{
					Kind:    "updated",
					OrigURL: "mem://localhost/a.txt",
					URL:     "mem://localhost/a.txt",
					Diff:    diff,
				}}
			},
		},
		{
			name: "delete",
			initialFiles: map[string]string{
				"mem://localhost/d.txt": "X\nY\n",
			},
			ops: func(ctx context.Context, s *patch.Session) error {
				return s.Delete(ctx, "mem://localhost/d.txt")
			},
			expect: func() []patch.Change {
				diff, _, _ := patch.GenerateDiff([]byte("X\nY\n"), nil, "mem://localhost/d.txt", 3)
				return []patch.Change{{
					Kind:    "delete",
					OrigURL: "mem://localhost/d.txt",
					URL:     "",
					Diff:    diff,
				}}
			},
		},
		{
			name: "move-only",
			initialFiles: map[string]string{
				"mem://localhost/m.txt": "Hello\n",
			},
			ops: func(ctx context.Context, s *patch.Session) error {
				return s.Move(ctx, "mem://localhost/m.txt", "mem://localhost/n.txt")
			},
			expect: func() []patch.Change {
				// Move without content change should yield empty diff
				return []patch.Change{{
					Kind:    "updated",
					OrigURL: "mem://localhost/m.txt",
					URL:     "mem://localhost/n.txt",
					Diff:    "",
				}}
			},
		},
		{
			name: "move-and-update",
			initialFiles: map[string]string{
				"mem://localhost/o.txt": "Hi\n",
			},
			ops: func(ctx context.Context, s *patch.Session) error {
				if err := s.Move(ctx, "mem://localhost/o.txt", "mem://localhost/p.txt"); err != nil {
					return err
				}
				return s.Update(ctx, "mem://localhost/p.txt", []byte("Hi\nThere\n"))
			},
			expect: func() []patch.Change {
				diff, _, _ := patch.GenerateDiff([]byte("Hi\n"), []byte("Hi\nThere\n"), "mem://localhost/p.txt", 3)
				return []patch.Change{{
					Kind:    "updated",
					OrigURL: "mem://localhost/o.txt",
					URL:     "mem://localhost/p.txt",
					Diff:    diff,
				}}
			},
		},
		{
			name:         "create-move-delete-cancels",
			initialFiles: map[string]string{},
			ops: func(ctx context.Context, s *patch.Session) error {
				if err := s.Add(ctx, "mem://localhost/t.txt", []byte("tmp\n")); err != nil {
					return err
				}
				if err := s.Move(ctx, "mem://localhost/t.txt", "mem://localhost/u.txt"); err != nil {
					return err
				}
				return s.Delete(ctx, "mem://localhost/u.txt")
			},
			expect: func() []patch.Change { return []patch.Change{} },
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// setup initial files
			for path, content := range c.initialFiles {
				if err := fs.Upload(ctx, path, 0644, strings.NewReader(content)); err != nil {
					t.Fatalf("setup: upload %s: %v", path, err)
				}
			}

			sess, err := patch.NewSession()
			if err != nil {
				t.Fatalf("new session: %v", err)
			}

			if err := c.ops(ctx, sess); err != nil {
				t.Fatalf("apply ops: %v", err)
			}

			got, err := sess.Snapshot(ctx)
			if err != nil {
				t.Fatalf("snapshot: %v", err)
			}

			assert.EqualValues(t, c.expect(), got)

			// cleanup
			for path := range c.initialFiles {
				_ = fs.Delete(ctx, path)
			}
		})
	}
}
