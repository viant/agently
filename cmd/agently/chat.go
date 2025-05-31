package agently

import (
    "bufio"
    "context"
    "fmt"
    "github.com/viant/agently/genai/extension/fluxor/llm/agent"
    "github.com/viant/agently/genai/tool"
    "os"
    "strings"
)

// ChatCmd handles interactive/chat queries.
type ChatCmd struct {
    Location string `short:"l" long:"location" description:"agent definition path"`
    Query    string `short:"q" long:"query"    description:"user query"`
    ConvID   string `short:"c" long:"conv"     description:"conversation ID (optional)"`
    Policy   string `long:"policy" description:"tool policy: auto|ask|deny" default:"auto"`
}

func (c *ChatCmd) Execute(_ []string) error {
    // Fallbacks -------------------------------------------------------
    if c.Location == "" {
        c.Location = "chat" // default agent shipped with embedded config
    }

    svc := executorSingleton()
    ctxBase := context.Background()
    pol := buildPolicy(c.Policy)

    convID := c.ConvID

    askOnce := func(query string) error {
        ctx := tool.WithPolicy(ctxBase, pol)
        in := &agent.QueryInput{ConversationID: convID, Location: c.Location, Query: query}
        out, err := svc.Conversation().Accept(ctx, in)
        if err != nil {
            fmt.Printf("error: %v\n", err)
            return nil
        }
        if strings.TrimSpace(out.Content) == "" {
            fmt.Println("[no response]")
        } else {
            fmt.Println(out.Content)
        }
        convID = in.ConversationID
        return nil
    }

    // Single-turn when -q was provided.
    if c.Query != "" {
        if err := askOnce(c.Query); err != nil {
            return err
        }
        fmt.Printf("[conversation-id] %s\n", convID)
        return nil
    }

    // Interactive loop.
    reader := bufio.NewReader(os.Stdin)
    for {
        fmt.Print("> ")
        line, _ := reader.ReadString('\n')
        line = strings.TrimSpace(line)
        if line == "" || line == "exit" || line == "quit" {
            fmt.Printf("[conversation-id] %s\n", convID)
            break
        }
        if err := askOnce(line); err != nil {
            return err
        }
    }
    return nil
}
