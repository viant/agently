package agently

import (
    "context"
    "bufio"
    "github.com/viant/agently/genai/executor"
    "github.com/viant/agently/genai/executor/instance"
    "log"
    "sync"
    "github.com/viant/agently/genai/tool"
    "fmt"
    "os"
    "strings"
)

var (
    cfgMu   sync.RWMutex
    cfgPath string
)

// executorSingleton initialises global executor only once to speed up CLI.
func executorSingleton() *executor.Service {
    // Ensure singleton is initialised once.
    cfgMu.RLock()
    path := cfgPath
    cfgMu.RUnlock()

    if instance.Get() == nil {
        if err := instance.Init(context.Background(), path); err != nil {
            log.Fatalf("executor init error: %v", err)
        }
    }
    return instance.Get()
}

// called from CLI after flag parsing
func setConfigPath(p string) {
    cfgMu.Lock(); cfgPath = p; cfgMu.Unlock()
}

// Helper ---------------------------------------------------------------

func buildPolicy(mode string) *tool.Policy {
    switch strings.ToLower(mode) {
    case tool.ModeDeny:
        return &tool.Policy{Mode: tool.ModeDeny}
    case tool.ModeAsk:
        return &tool.Policy{Mode: tool.ModeAsk, Ask: stdinAsk}
    default:
        return &tool.Policy{Mode: tool.ModeAuto}
    }
}

// stdinAsk is used when Policy.Mode==ask to prompt user.
func stdinAsk(_ context.Context, name string, args map[string]interface{}, p *tool.Policy) bool {
    reader := bufio.NewReader(os.Stdin)
    fmt.Printf("Execute tool %s with args %v? [y/n/all] ", name, args)
    line, _ := reader.ReadString('\n')
    line = strings.ToLower(strings.TrimSpace(line))
    switch line {
    case "y", "yes":
        return true
    case "all":
        p.Mode = tool.ModeAuto
        return true
    default:
        return false
    }
}

