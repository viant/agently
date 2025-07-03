package core

import (
    "context"
    "testing"

    plan "github.com/viant/agently/genai/agent/plan"
    "github.com/stretchr/testify/assert"
)

func TestEnsureJSONResponse(t *testing.T) {
    ctx := context.Background()

    testCases := []struct {
        name        string
        text        string
        wantSteps   int
        wantErr     bool
    }{
        {
            name: "vertex-claude fenced json plan",
            text: "I'll help you find out how many tables are in the database. " +
                "Let me search for SQL files or database files in the knowledge directory.\n\n" +
                "```json\n{" +
                "\"steps\":[{" +
                "\"type\": \"tool\"," +
                " \"reason\": \"Search for SQL and database files to find table information\"," +
                " \"name\": \"system_exec-execute\"," +
                " \"args\": {\"commands\": [\"rg --files /Users/awitas/go/src/github.com/viant/agently/ag/agents/chat/knowledge | grep -E \\\'.*(sql|db)$\\'\"]}}]}\n```",
            wantSteps: 1,
            wantErr:   false,
        },
    }

    for _, tc := range testCases {
        var p plan.Plan
        err := EnsureJSONResponse(ctx, tc.text, &p)
        if tc.wantErr {
            assert.Error(t, err, tc.name)
            continue
        }
        assert.NoError(t, err, tc.name)
        assert.EqualValues(t, tc.wantSteps, len(p.Steps), tc.name)
    }
}
