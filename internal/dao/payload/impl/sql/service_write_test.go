//go:build cgo

package sql

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/internal/testutil/dbtest"
	w "github.com/viant/agently/pkg/agently/payload"
	"github.com/viant/datly"
	"github.com/viant/datly/view"
	_ "modernc.org/sqlite"
)

func newDatlySvc(t *testing.T, dbPath string) *Service {
	t.Helper()
	srv, err := datly.New(context.Background())
	if err != nil {
		t.Fatalf("datly new: %v", err)
	}
	if err := srv.AddConnectors(context.Background(), view.NewConnector("agently", "sqlite", dbPath)); err != nil {
		t.Fatalf("add connector: %v", err)
	}
	if err := Register(context.Background(), srv); err != nil {
		t.Fatalf("register: %v", err)
	}
	return New(context.Background(), srv)
}

type expected struct {
	Id, Kind, Storage, Mime string
	Size                    int
}

func toExpected(v *PayloadView) expected {
	return expected{Id: v.Id, Kind: v.Kind, Storage: v.Storage, Mime: v.MimeType, Size: v.SizeBytes}
}

func Test_PayloadWrite_InsertUpdate_DataDriven(t *testing.T) {
	type testCase struct {
		name     string
		seed     []dbtest.ParameterizedSQL
		patch    []*w.Payload
		verifyID string
		expected expected
	}

	_, filename, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filename))))))
	ddlPath := filepath.Join(repoRoot, "script", "schema.ddl")

	jsonMime := "application/json"
	cases := []testCase{
		{
			name: "insert inline payload",
			seed: nil,
			patch: []*w.Payload{func() *w.Payload {
				p := &w.Payload{Has: &w.PayloadHas{}}
				p.SetId("p1")
				p.SetKind("model_request")
				p.SetMimeType(jsonMime)
				p.SetSizeBytes(12)
				p.SetStorage("inline")
				p.SetInlineBody([]byte("{}"))
				p.SetCompression("none")
				return p
			}()},
			verifyID: "p1",
			expected: expected{Id: "p1", Kind: "model_request", Storage: "inline", Mime: jsonMime, Size: 12},
		},
		{
			name: "update to object storage",
			seed: []dbtest.ParameterizedSQL{
				{SQL: "INSERT INTO call_payload (id, tenant_id, kind, mime_type, size_bytes, storage, compression, inline_body) VALUES (?,?,?,?,?,?,?,?)", Params: []interface{}{"p2", "t1", "tool_request", jsonMime, 2, "inline", "none", []byte("x")}},
			},
			patch: []*w.Payload{func() *w.Payload {
				p := &w.Payload{Has: &w.PayloadHas{}}
				p.SetId("p2")
				p.SetKind("tool_request")
				p.SetMimeType(jsonMime)
				p.SetSizeBytes(3)
				p.SetStorage("object")
				p.SetURI("gs://bucket/obj.json")
				p.SetCompression("none")
				return p
			}()},
			verifyID: "p2",
			expected: expected{Id: "p2", Kind: "tool_request", Storage: "object", Mime: jsonMime, Size: 3},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, dbPath, cleanup := dbtest.CreateTempSQLiteDB(t, "agently-v2-payload-post")
			defer cleanup()
			dbtest.LoadDDLFromFile(t, db, ddlPath)
			dbtest.ExecAll(t, db, tc.seed)

			svc := newDatlySvc(t, dbPath)
			out, err := svc.Patch(context.Background(), tc.patch...)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.NotNil(t, out) {
				return
			}

			rows, err := svc.List(context.Background(), WithIDs(tc.verifyID))
			if !assert.Nil(t, err) {
				return
			}
			if !assert.True(t, len(rows) == 1) {
				return
			}
			actual := toExpected(rows[0])
			assert.EqualValues(t, tc.expected, actual)
		})
	}
}
