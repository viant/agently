//go:build cgo

package conversation

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/memory"
	"github.com/viant/datly/view"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestService(t *testing.T) {
	t.Skip("skipping integration test that requires sqlite3 environment")
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
	convID := "550e8400-e29b-41d4-a716-446655440000" // Using a fixed UUID for testing
	_, err = db.Exec("INSERT INTO conversation (id, summary, agent_name) VALUES (?, ?, ?)", convID, "Test Conversation", "TestAgent")
	if err != nil {
		t.Fatalf("Failed to insert test conversation: %v", err)
	}
	t.Logf("Created test conversation with ID: %s", convID)

	// Create the connector and service
	connector := view.NewConnector("agently", "sqlite", dbLocation)
	srv, err := New(context.Background(), connector)
	if !assert.Nil(t, err) {
		t.Error(err)
		return
	}
	assert.NotNil(t, srv)

	// Set the database connection for direct queries
	srv.db = db

	// Add a test message
	err = srv.AddMessage(context.Background(), convID, memory.Message{
		Role:    "user",
		Content: "hello world",
	})
	if !assert.Nil(t, err) {
		t.Error("expected no error when adding message")
		return
	}
	t.Logf("Added message to conversation %s", convID)

	// Check if the message was inserted into the database
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM message WHERE conversation_id = ?", convID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query message count: %v", err)
	}
	t.Logf("Found %d messages in the database for conversation %s", count, convID)

	// If there are messages in the database, print them
	if count > 0 {
		rows, err := db.Query("SELECT id, role, content FROM message WHERE conversation_id = ?", convID)
		if err != nil {
			t.Fatalf("Failed to query messages: %v", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id, role, content string
			if err := rows.Scan(&id, &role, &content); err != nil {
				t.Fatalf("Failed to scan message row: %v", err)
			}
			t.Logf("Message in DB: ID=%s, Role=%s, Content=%s", id, role, content)
		}
	}

	// Get the messages using the service
	t.Logf("[DEBUG_LOG] About to call GetMessages with convID: %s", convID)
	messages, err := srv.GetMessages(context.Background(), convID)
	if !assert.Nil(t, err) {
		t.Error("expected no error when reading message")
		return
	}
	t.Logf("Retrieved %d messages using service", len(messages))

	// Debug: Query the database directly to see if we can get messages
	rows, err := db.Query("SELECT id, role, content FROM message WHERE conversation_id = ?", convID)
	if err != nil {
		t.Fatalf("[DEBUG_LOG] Failed to query messages directly: %v", err)
	}
	defer rows.Close()
	var directMessages []string
	for rows.Next() {
		var id, role, content string
		if err := rows.Scan(&id, &role, &content); err != nil {
			t.Fatalf("[DEBUG_LOG] Failed to scan message row: %v", err)
		}
		directMessages = append(directMessages, fmt.Sprintf("ID=%s, Role=%s, Content=%s", id, role, content))
	}
	t.Logf("[DEBUG_LOG] Direct query found %d messages: %v", len(directMessages), directMessages)

	// Check that we got at least one message
	if !assert.NotEmpty(t, messages, "Expected at least one message") {
		return
	}

	// Check the content of the first message
	assert.Equal(t, "user", messages[0].Role)
	assert.Equal(t, "hello world", messages[0].Content)
}
