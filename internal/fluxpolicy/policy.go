package fluxpolicy

import (
	"context"
	"strings"
)

const (
	ModeAsk  = "ask"
	ModeAuto = "auto"
	ModeDeny = "deny"
)

type AskFunc func(ctx context.Context, action string, args map[string]interface{}, p *Policy) bool

type Policy struct {
	Mode      string
	AllowList []string
	BlockList []string
	Ask       AskFunc
}

type Config struct {
	Mode      string   `json:"mode,omitempty" yaml:"mode,omitempty"`
	AllowList []string `json:"allow,omitempty" yaml:"allow,omitempty"`
	BlockList []string `json:"block,omitempty" yaml:"block,omitempty"`
}

func ToConfig(p *Policy) *Config {
	if p == nil {
		return nil
	}
	return &Config{Mode: p.Mode, AllowList: append([]string(nil), p.AllowList...), BlockList: append([]string(nil), p.BlockList...)}
}

func FromConfig(c *Config) *Policy {
	if c == nil {
		return nil
	}
	return &Policy{Mode: c.Mode, AllowList: append([]string(nil), c.AllowList...), BlockList: append([]string(nil), c.BlockList...)}
}

func (p *Policy) IsAllowed(action string) bool {
	if p == nil {
		return true
	}
	normalized := strings.ToLower(action)
	for _, b := range p.BlockList {
		if normalized == strings.ToLower(b) {
			return false
		}
	}
	if len(p.AllowList) == 0 {
		return true
	}
	for _, a := range p.AllowList {
		if normalized == strings.ToLower(a) {
			return true
		}
	}
	return false
}

type ctxKeyT struct{}

var ctxKey ctxKeyT

func WithPolicy(ctx context.Context, p *Policy) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ctxKey, p)
}

func FromContext(ctx context.Context) *Policy {
	if ctx == nil {
		return nil
	}
	if v, ok := ctx.Value(ctxKey).(*Policy); ok {
		return v
	}
	return nil
}
