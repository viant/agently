package tool

import (
	"encoding/json"
	"errors"
	"strings"

	mcptool "github.com/viant/fluxor-mcp/mcp/tool"
	"github.com/viant/forge/backend/types"
)

type FeedSpec struct {
	ID          string          `yaml:"id,omitempty" json:"id,omitempty"`
	Title       string          `yaml:"title,omitempty" json:"title,omitempty"`
	Priority    int             `yaml:"priority,omitempty" json:"priority,omitempty"`
	Match       MatchSpec       `yaml:"match" json:"match"`
	Activation  ActivationSpec  `yaml:"activation" json:"activation"`
	DataSources DataSources     `yaml:"dataSource,omitempty" json:"dataSource,omitempty"`
	UI          types.Container `yaml:"ui" json:"ui"`
}

type DataSources map[string]*DataSource

type DataSource struct {
	types.DataSource `yaml:",inline"`
	Source           string `json:"source" yaml:"source"`
	Name             string `json:"name" yaml:"name"`
}

func (d *DataSource) HasSource() bool {
	if d.Source != "" {
		return d.Source != ""
	}
	return false
}

func (s DataSources) FeedDataSource() (*DataSource, error) {
	for key, candidate := range s {
		if candidate.HasSource() {
			candidate.Name = key
			return candidate, nil
		}
	}
	return nil, errors.New("no sources defined in datasource found")
}

// Normalize returns normalized datasourcse
func (s DataSources) Normalize() map[string]*types.DataSource {
	var result = make(map[string]*types.DataSource, len(s))
	for key := range s {
		candidate := s[key].DataSource
		result[key] = &candidate
	}
	return result
}

// Transform returns a normalized map of DataSources with keys suffixed by the provided hash,
// and a mapping from original name -> transformed name. Any internal DataSourceRef fields
// that reference other data sources are also rewritten using the mapping.
func (s DataSources) Transform(hash string) (map[string]*types.DataSource, map[string]string) {
	if len(s) == 0 {
		return map[string]*types.DataSource{}, map[string]string{}
	}
	nameMap := make(map[string]string, len(s))
	for name := range s {
		nameMap[name] = name + hash
	}
	out := make(map[string]*types.DataSource, len(s))
	for name, ds := range s {
		if ds == nil {
			continue
		}
		copied := ds.DataSource // value copy
		if newRef, ok := nameMap[copied.DataSourceRef]; ok {
			copied.DataSourceRef = newRef
		}
		out[nameMap[name]] = &copied
	}
	return out, nameMap
}

// RewriteContainerDataSourceRefs returns a copy of the container with every dataSourceRef
// rewritten according to the provided name mapping. Keys are original names; values are transformed names.
func RewriteContainerDataSourceRefs(c types.Container, nameMap map[string]string) types.Container {
	var result types.Container
	raw, err := json.Marshal(c)
	if err != nil {
		return c
	}
	var any interface{}
	if err := json.Unmarshal(raw, &any); err != nil {
		return c
	}
	var rewrite func(node interface{})
	rewrite = func(node interface{}) {
		switch v := node.(type) {
		case map[string]interface{}:
			if refVal, ok := v["dataSourceRef"].(string); ok {
				if newName, ok2 := nameMap[refVal]; ok2 {
					v["dataSourceRef"] = newName
				}
			}
			for _, child := range v {
				rewrite(child)
			}
		case []interface{}:
			for _, item := range v {
				rewrite(item)
			}
		}
	}
	rewrite(any)
	if buf, err := json.Marshal(any); err == nil {
		_ = json.Unmarshal(buf, &result)
		return result
	}
	return c
}

type MatchSpec struct {
	Service string `yaml:"service,omitempty" json:"service,omitempty"`
	Method  string `yaml:"method,omitempty" json:"method,omitempty"`
}

func (m *MatchSpec) Name() mcptool.Name {
	return mcptool.NewName(m.Service, m.Method)
}

type ActivationSpec struct {
	// Kind selects the activation mode: "history" or "tool_call".
	Kind string `yaml:"kind,omitempty" json:"kind,omitempty"`
	// Scope controls how many recorded calls are considered when kind==history:
	//  - "last" (default): only the most recent matching call is used
	//  - "all": aggregate data from all matching calls in the turn
	Scope string `yaml:"scope,omitempty" json:"scope,omitempty"`
	// Optional explicit tool to invoke when kind==tool_call. When omitted,
	// match.service/method may be used as a fallback by the consumer.
	Service string                 `yaml:"service,omitempty" json:"service,omitempty"`
	Method  string                 `yaml:"method,omitempty" json:"method,omitempty"`
	Args    map[string]interface{} `yaml:"args,omitempty" json:"args,omitempty"`
}

// FeedSpecs is a convenience slice alias for helpers.
type FeedSpecs []*FeedSpec

func (s FeedSpecs) Index() map[mcptool.Name]*FeedSpec {
	result := make(map[mcptool.Name]*FeedSpec)
	for _, feed := range s {
		result[feed.Match.Name()] = feed
	}
	return result
}

// MatchSpec collects all MatchSpec entries in declaration order.
func (s FeedSpecs) MatchSpec() []MatchSpec {
	if len(s) == 0 {
		return nil
	}
	out := make([]MatchSpec, 0, len(s))
	for _, fs := range s {
		if fs == nil {
			continue
		}
		out = append(out, MatchSpec{
			Service: strings.TrimSpace(fs.Match.Service),
			Method:  strings.TrimSpace(fs.Match.Method),
		})
	}
	return out
}

// Matches reports whether any spec matches the provided canonical tool name.
func (s FeedSpecs) Matches(name mcptool.Name) bool {
	for _, m := range s.MatchSpec() {
		if m.Matches(name) {
			return true
		}
	}
	return false
}

// Matches compares against an mcptool.Name; no wildcard allowed.
func (m MatchSpec) Matches(name mcptool.Name) bool {
	ms := strings.TrimSpace(m.Service)
	mm := strings.TrimSpace(m.Method)
	if ms == "" || mm == "" {
		return false
	}
	return strings.EqualFold(ms, name.Service()) && strings.EqualFold(mm, name.Method())
}

// InvokeServiceMethod returns the effective (service, method) to invoke for this spec,
// preferring Activation overrides and falling back to Match when empty.
func (f *FeedSpec) InvokeServiceMethod() (string, string) {
	if f == nil {
		return "", ""
	}
	svc := strings.TrimSpace(f.Activation.Service)
	mth := strings.TrimSpace(f.Activation.Method)
	if svc == "" {
		svc = strings.TrimSpace(f.Match.Service)
	}
	if mth == "" {
		mth = strings.TrimSpace(f.Match.Method)
	}
	return svc, mth
}

// ShallInvokeTool indicates whether this spec should trigger a direct tool invocation.
func (f *FeedSpec) ShallInvokeTool() bool {
	if f == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(f.Activation.Kind), "tool_call")
}

// ShallUseHistory indicates whether this spec should evaluate history
// (default when kind is empty) or explicitly when kind==history.
func (f *FeedSpec) ShallUseHistory() bool {
	if f == nil {
		return false
	}
	k := strings.TrimSpace(f.Activation.Kind)
	return k == "" || strings.EqualFold(k, "history")
}
