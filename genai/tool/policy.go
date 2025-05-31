package tool

import "context"

// Mode constants for tool execution behaviour.
const (
    ModeAsk  = "ask"  // ask user for every tool call
    ModeAuto = "auto" // execute automatically
    ModeDeny = "deny" // refuse tool execution
)

// AskFunc is invoked when policy.Mode==ModeAsk. It should return true to
// approve the call, false to reject. It can optionally mutate the policy (for
// example to switch to auto mode after user confirmation).
type AskFunc func(ctx context.Context, name string, args map[string]interface{}, p *Policy) bool

// Policy controls runtime behaviour of tool execution.
type Policy struct {
    Mode       string              // ask, auto or deny
    AllowList  []string            // optional set of allowed tools (empty => all)
    BlockList  []string            // optional set of blocked tools
    Ask        AskFunc            // optional callback when Mode==ask
}

// IsAllowed checks whether a tool name is permitted by Allow/Block lists.
func (p *Policy) IsAllowed(name string) bool {
    if p == nil {
        return true
    }
    for _, b := range p.BlockList {
        if b == name {
            return false
        }
    }
    if len(p.AllowList) == 0 {
        return true
    }
    for _, a := range p.AllowList {
        if a == name {
            return true
        }
    }
    return false
}

// context key to carry policy
type policyKeyT struct{}

var policyKey = policyKeyT{}

// WithPolicy attaches policy to context.
func WithPolicy(ctx context.Context, p *Policy) context.Context {
    return context.WithValue(ctx, policyKey, p)
}

// FromContext retrieves policy from context; may be nil.
func FromContext(ctx context.Context) *Policy {
    val := ctx.Value(policyKey)
    if val == nil {
        return nil
    }
    if p, ok := val.(*Policy); ok {
        return p
    }
    return nil
}
