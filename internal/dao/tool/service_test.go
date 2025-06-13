package tool

import (
	"context"
	"database/sql"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/agently/internal/dao/conversation"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"gi
	"github.com/stretchr/testify/assert"
	_ "github.com/mattn/go-sqlite3"
)

func TestService(t *testing.T) {
	ctx := context.Background()

	// Create a temporary directory for the SQLite database
	tempDir, err := ioutil.TempDir("", "agently-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir) // Clean up after the test

	// Create the database file path in the temporary directory
	dbLocation := filepath.Join(tempDir, "agently.db")

	// Read the schema.ddl file
	// Find the schema file relative to the current file
	_, filename, _, _ := runtime.Caller(0)
	// Ensure we have the correct path to the repository root
	repoRoot := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filename))))), "agently")
	schemaPath := filepath.Join(repoRoot, "internal", "script", "schema.ddl")
	schemaBytes, err := ioutil.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("Failed to read schema file: %v", err)
	}
	schemaSQL := string(schemaBytes)

	// Create and initialize the database
	db, err := sql.Open("sqlite3", dbLocation)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Execute each SQL statement in the schema
	for _, statement := range strings.Split(schemaSQL, ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		_, err = db.Exec(statement)
		if err != nil {
			t.Fatalf("Failed to execute SQL: %v\nStatement: %s", err, statement)
		}
	}

	// Create a test conversation directly in the database with a UUID
	convID := "test_conv"
	_, err = db.Exec("INSERT INTO conversation (id, summary, agent_name) VALUES (?, ?, ?)", convID, "Test Conversation", "TestAgent")
	if err != nil {
		t.Fatalf("Failed to insert test conversation: %v", err)
	}

	connector := view.NewConnector("agently", "sqlite", dbLocation)

	convSrv, err := conversation.New(ctx, connector)
	if !assert.NoError(t, err) {
		return
	}

	srv, err := New(ctx, connector)
	if !assert.NoError(t, err) {
		return
	}
	assert.NotNil(t, srv)

	var toolName = "test_tool"

	err = convSrv.AddMessage(ctx, convID, memory.Message{
		Role:     "abc",
		Content:  "test content",
		ToolName: &toolName,
	})
	if !assert.Nil(t, err) {
		return
	}
	var args = `{"arg1": "value1", "arg2": "value2"}`
	err = srv.Add(ctx, &tool.Call{
		ConversationID: convID,
		ToolName:       toolName,
		Arguments:      &args,
	})

	if !assert.Nil(t, err) {
		return
	}
	// Retrieve list (should not error even if empty)
	calls, err := srv.List(ctx, convID)
	assert.NoError(t, err)
	assert.NotNil(t, calls)
}
