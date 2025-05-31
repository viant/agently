package agently

import (
    "context"
    "encoding/json"
    "fmt"
    "github.com/viant/agently/genai/extension/fluxor/llm/agent"
    "github.com/viant/agently/genai/tool"
    "io"
    "os"
)

// RunCmd executes full agentic workflow from JSON payload.
type RunCmd struct {
    Location  string `short:"l" long:"location" description:"agent definition path"`
    InputFile string `short:"i" long:"input"    description:"JSON file with QueryInput (stdin if empty)"`
    Policy    string `long:"policy" description:"tool policy: auto|ask|deny" default:"auto"`
}

func (r *RunCmd) Execute(_ []string) error {
    var reader io.Reader = os.Stdin
    if r.InputFile != "" {
        f, err := os.Open(r.InputFile)
        if err != nil {
            return fmt.Errorf("open input: %w", err)
        }
        defer f.Close()
        reader = f
    }

    var q agent.QueryInput
    if err := json.NewDecoder(reader).Decode(&q); err != nil {
        return fmt.Errorf("decode input: %w", err)
    }
    if r.Location != "" {
        q.Location = r.Location
    }

    svc := executorSingleton()
    ctx := tool.WithPolicy(context.Background(), buildPolicy(r.Policy))

    out, err := svc.Conversation().Accept(ctx, &q)
    if err != nil {
        return err
    }
    bytes, _ := json.MarshalIndent(out, "", "  ")
    fmt.Println(string(bytes))
    return nil
}
