// Package preview loads declarative local reports and executes deterministic fixture queries.
package preview

import (
	"bytes"
	"encoding/json"
	"github.com/viant/agently/preview/datasource"
	"github.com/viant/forge/backend/types"
)

type Object = map[string]any

type Diagnostic struct {
	Code         string `json:"code"`
	Message      string `json:"message"`
	Path         string `json:"path"`
	DataSource   string `json:"dataSource,omitempty"`
	RequestField string `json:"requestField,omitempty"`
}

func (d *Diagnostic) Error() string { return d.Code + ": " + d.Message }
func fail(code, ds, field, message string) error {
	return &Diagnostic{"mockPreview" + code, message, "x-mock-preview.dataSources." + ds, ds, field}
}

type Contract struct {
	Shape       string `json:"shape"`
	ResultsPath string `json:"resultsPath"`
	ResultName  string `json:"resultName"`
	RowPath     string `json:"rowPath"`
	HasMorePath string `json:"hasMorePath"`
}
type Capabilities struct {
	Projection bool   `json:"projection"`
	Filtering  bool   `json:"filtering"`
	Grouping   bool   `json:"grouping"`
	Sorting    bool   `json:"sorting"`
	Pagination string `json:"pagination"`
}
type Measure struct {
	Aggregation string `json:"aggregation"`
	RowLevel    bool   `json:"rowLevel"`
}
type Declaration struct {
	File             string             `json:"file"`
	ResultName       string             `json:"resultName"`
	InputContract    Contract           `json:"inputContract"`
	Query            Capabilities       `json:"query"`
	Dimensions       []string           `json:"dimensions"`
	Measures         map[string]Measure `json:"measures"`
	IncludeTotalRows bool               `json:"includeTotalRows"`
	CaseSensitive    *bool              `json:"caseSensitive"`
	HiddenSortFields []string           `json:"hiddenSortFields"`
	Optional         bool               `json:"optional"`
}
type Extension struct {
	Version           int                    `json:"version"`
	DataRoot          string                 `json:"dataRoot"`
	DefaultVariant    string                 `json:"defaultVariant"`
	PrimaryDataSource string                 `json:"primaryDataSource"`
	MaxPageSize       int                    `json:"maxPageSize"`
	DataSources       map[string]Declaration `json:"dataSources"`
}
type Source struct {
	Service           *types.Service `json:"service,omitempty"`
	Columns           []Object       `json:"columns"`
	ResultContract    Contract       `json:"resultContract"`
	ParameterBindings []Binding      `json:"parameterBindings"`
	MCPRequest        *MCPRequest    `json:"mcpRequest,omitempty"`
}

// MCPRequest declares a generic tool-call argument template for servers whose
// public contract does not use the preview query envelope.
type MCPRequest struct {
	Arguments   Object `json:"arguments"`
	RequestPath string `json:"requestPath,omitempty"`
}
type Binding struct {
	Parameter string `json:"parameter"`
	Field     string `json:"field"`
	Operator  string `json:"operator"`
}
type Dataset struct {
	DataSource string `json:"dataSource"`
	Query      Query  `json:"query"`
}
type Field struct {
	Field       string `json:"field"`
	Alias       string `json:"alias,omitempty"`
	Aggregation string `json:"aggregation,omitempty"`
}

func (f *Field) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		return json.Unmarshal(b, &f.Field)
	}
	type plain Field
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	return decoder.Decode((*plain)(f))
}
func (f Field) name() string {
	if f.Alias != "" {
		return f.Alias
	}
	return f.Field
}

type Projection struct {
	Fields     []Field `json:"fields,omitempty"`
	Dimensions []Field `json:"dimensions"`
	Measures   []Field `json:"measures"`
}
type Predicate struct {
	And   []Predicate `json:"and,omitempty"`
	Or    []Predicate `json:"or,omitempty"`
	Not   *Predicate  `json:"not,omitempty"`
	Field string      `json:"field,omitempty"`
	Op    string      `json:"op,omitempty"`
	Value any         `json:"value,omitempty"`
}
type Order struct {
	Field     string `json:"field"`
	Direction string `json:"direction"`
	Nulls     string `json:"nulls,omitempty"`
}
type Page struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}
type Query struct {
	DataSource string      `json:"dataSource,omitempty"`
	Projection *Projection `json:"projection,omitempty"`
	Filter     *Predicate  `json:"filter,omitempty"`
	OrderBy    []Order     `json:"orderBy,omitempty"`
	Page       *Page       `json:"page,omitempty"`
}
type Package struct {
	Endpoints      map[string]datasource.Endpoint `json:"endpoints,omitempty"`
	executeRemote  func(string, Query) (*Result, error)
	Definition     Object             `json:"definition"`
	Extension      Extension          `json:"extension"`
	Sources        map[string]Source  `json:"sources"`
	Datasets       map[string]Dataset `json:"datasets"`
	Variant        string             `json:"variant"`
	Warnings       []string           `json:"warnings,omitempty"`
	definitionHash string
	limits         Limits
	queries        chan struct{}
	fixtures       map[string]*fixture
}
type fixture struct {
	raw           any
	columns       []Object
	rows          []Object
	hasMore       bool
	hash          string
	errorResponse bool
}
type Result struct {
	Response    any      `json:"response"`
	Columns     []Object `json:"columns"`
	Rows        []Object `json:"rows"`
	HasMore     bool     `json:"hasMore"`
	Fingerprint string   `json:"fingerprint"`
}

func str(v any) string   { s, _ := v.(string); return s }
func clone[T any](v T) T { b, _ := json.Marshal(v); var r T; _ = json.Unmarshal(b, &r); return r }

func (q *Query) UnmarshalJSON(b []byte) error {
	type plain Query
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	return d.Decode((*plain)(q))
}
