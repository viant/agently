# Report preview builder

The report preview builder runs a report from a local folder containing YAML and
JSON exposed by an MCP server. It provides a browser preview, a datasource query inspector, and exports
through Forge's reporting compiler. Report authors edit declarative files; they do
not need to write Go or JavaScript for each report.

This guide describes the standalone Go app in [`preview/report`](../report)
and its [`cmd/report-preview`](../cmd/report-preview) CLI. For the interactive
ReportBuilder component and broader reporting architecture, see [Reporting](../../../forge/doc/reporting.md).
The preview app's browser controls select fixture variants, supply parameter values,
and inspect queries; report layout is authored in YAML.

## Start a preview

Run these commands from the agently repository root, using the Go version declared
in `go.mod`:

```sh
go run ./preview/cmd/report-preview serve ./preview/report/examples/demo
```

Open [http://127.0.0.1:8095](http://127.0.0.1:8095). The app starts its mock MCP endpoint at `/mcp` automatically. No Node build or separately managed MCP server is required.

| Example | Report | Fixture coverage |
| --- | --- | --- |
| [`demo`](../report/examples/demo/report.yaml) | Operations Overview | Regional operations and project targets; tabular and record responses |
| [`demo2`](../report/examples/demo2/report.yaml) | Order Fulfillment | Orders, planned/actual scenarios, and nullable process timing; record responses |

All example names and values are synthetic and business-neutral.

To run the second example on another port:

```sh
go run ./preview/cmd/report-preview serve ./preview/report/examples/demo2 --addr 127.0.0.1:8096
```

For a reusable executable:

```sh
go build -o /tmp/report-preview ./preview/cmd/report-preview
/tmp/report-preview serve ./preview/report/examples/demo
```

The server binds to loopback only. Stop it with Ctrl-C. Report and fixture edits
are loaded on the next render or query; restart the server after changing Go code
or the embedded browser UI.

## Create a report folder

A package contains a report definition and a base fixture for each datasource:

```text
my-report/
  report.yaml
  ds/
    operations.json
  variants/
    empty/
      ds/
        operations.json
```

Datasource IDs and filenames are case-sensitive. The datasource `operations` uses
`ds/operations.json`; rename both when changing its ID. Variant files overlay the
base by datasource ID, and omitted files inherit the base. Variants cannot replace
`report.yaml`. Keep scripts and credentials out of report folders.

### Minimal report.yaml

This complete example uses native Forge blocks under `report` and an ordinary
record fixture. It groups completed tasks by region and displays the result in a
table.

```yaml
kind: reporting.report

report:
  id: regional-operations
  title: Regional Operations
  blocks:
    - id: regionalTable
      kind: tableBlock
      title: Completed tasks by region
      datasetRef: regionalSummary
      columns:
        - {key: region, label: Region}
        - {key: completedTasks, label: Completed Tasks, format: compactNumber}

parameters:
  - {name: region, type: string, label: Region, multiple: true}

endpoints:
  mockReports:
    type: mcp
    transport: streamable
    baseURL: /mcp

dataSources:
  operations:
    service: {endpoint: mockReports, uri: operations, method: POST}
    resultContract: {shape: records, rowPath: rows, hasMorePath: hasMore}
    columns:
      - {name: region, type: string, role: dimension, nullable: false}
      - {name: completedTasks, type: integer, role: measure, format: compactNumber, nullable: false}
    parameterBindings:
      - {parameter: region, field: region, operator: in}

datasets:
  regionalSummary:
    dataSource: operations
    query:
      projection:
        dimensions: [region]
        measures:
          - {field: completedTasks, aggregation: sum}
      orderBy:
        - {field: completedTasks, direction: desc}
      page: {limit: 20, offset: 0}

x-mock-preview:
  version: 1
  dataRoot: ./ds
  primaryDataSource: operations
  maxPageSize: 1000
  dataSources:
    operations:
      inputContract: {shape: records, rowPath: rows, hasMorePath: hasMore}
      query:
        projection: true
        filtering: true
        grouping: true
        sorting: true
        pagination: offset
      dimensions: [region]
      measures:
        completedTasks: {aggregation: sum}
```

### ds/operations.json

```json
{
  "status": "ok",
  "rows": [
    {"region": "North", "completedTasks": 120},
    {"region": "Central", "completedTasks": 90},
    {"region": "North", "completedTasks": 30}
  ],
  "hasMore": false,
  "meta": {"source": "synthetic-example"}
}
```

Save the two files, then validate and serve the folder:

```sh
go run ./preview/cmd/report-preview validate ./my-report
go run ./preview/cmd/report-preview serve ./my-report
```

The resulting table contains North with 150 completed tasks, followed by Central
with 90. The report title and table are normal Forge reporting elements.

## Understand the declarations

| Section | Purpose |
| --- | --- |
| `report` | Native Forge report identity, title, and blocks |
| `parameters` | Named input values, types, defaults, and optional allowed values |
| `endpoints` | Named MCP endpoints (`type`, `baseURL`, and `transport`) |
| `dataSources` | Service endpoint/tool references, response contracts, column metadata, and parameter bindings |
| `datasets` | Named queries against datasources; blocks reference their results |
| `x-mock-preview` | Fixture locations, input shapes, query capabilities, and permitted aggregations |

A datasource is the raw input. A dataset is the result of one query against that
input. Several datasets can use the same datasource independently, for example a
summary, a trend, and a paged evidence table. A block's `datasetRef` identifies the
dataset, not the fixture filename.

The bundled examples use the `ReportDefinition` convenience shape with `metadata`
and `sections`. Its adapter translates `kpiGroup`, `chart`, and `table` blocks into
Forge's native block grammar. Choose either that example shape or native `report`
blocks for a report; native blocks remain owned by Forge's reporting engine.

## Connect datasources through MCP

Datasources use Forge's existing `service.endpoint`, `service.uri`, and
`service.method` declaration shape. The shared Go MCP adapter resolves the endpoint
name, connects with `viant/mcp`, and treats `service.uri` as the MCP tool name:

```yaml
endpoints:
  mockReports:
    type: mcp
    transport: streamable
    baseURL: /mcp

dataSources:
  operations:
    service: {endpoint: mockReports, uri: operations, method: POST}
    # resultContract, columns, and parameterBindings follow
```

`baseURL` is the complete MCP endpoint URL; the tool name is not appended to it.
A relative `/mcp` resolves against the preview host. An absolute URL connects to a
separately running MCP server. The example tools receive `{query, variant}` and
return the public JSON response body in standard MCP content. A transport failure
fails the request; the host never falls back to directly reading fixture rows.

```text
Report dataset + datasource.service
  -> shared viant/mcp client
  -> POST /mcp: tools/call
  -> shared filesystem mock server
  -> validated filtering / grouping / projection / sorting / pagination
  -> public JSON response
  -> report compiler
  -> HTML / PDF / CSV / XLSX
```

The browser's HTTP endpoints coordinate rendering, but all runtime datasource
queries cross MCP. CLI `query`, `compile`, and `export` start an ephemeral local
mock endpoint and use that same transport. `validate` and `describe` inspect the
local package without requiring a transport connection.

To run the mock report datasource server independently:

```sh
go run ./preview/cmd/mock-mcp --root ./preview/report/examples/demo --report --addr 127.0.0.1:8097
```

Set the report's `endpoints.mockReports.baseURL` to
`http://127.0.0.1:8097/mcp` to use it. The `--report` option installs the report's
typed query/aggregation rules. The preview's embedded endpoint remains available,
but datasource resolution follows the declared URL.

The server is shared infrastructure, not a report renderer. Generic fixtures for
window preview can be served without `--report`:

```sh
go run ./preview/cmd/mock-mcp --root ./window-fixtures --addr 127.0.0.1:8097
```

See [Shared mock MCP datasources](mock-mcp-datasources.md) for fixture layout,
discovery, dynamic queries, and integration APIs. The [native window preview](window-preview.md) now reuses the same server and
datasource client.

## Choose a fixture format

**Records:** set `inputContract.shape: records` and an explicit `rowPath`. Declare
all usable fields in the datasource's `columns`. Undeclared object properties are
unavailable to queries. Missing values are accepted only for nullable columns.
Use `rowPath: $` for a bare top-level array.

**Tabular:** set `inputContract.shape: tabular`, `resultsPath: data`, and select a
named result with `resultName`. The fixture contains a decoded public response body:

```json
{
  "status": "ok",
  "data": [{
    "name": "operations",
    "columns": [
      {"name": "region", "type": "string", "role": "dimension", "nullable": false},
      {"name": "completedTasks", "type": "integer", "role": "measure", "format": "compactNumber", "nullable": false}
    ],
    "rows": [["North", 120], ["Central", 90]],
    "hasMore": false
  }],
  "meta": {"source": "synthetic-example"}
}
```

When switching the minimal example to tabular input and output, update both its
`inputContract` and datasource `resultContract` to
`{shape: tabular, resultsPath: data, resultName: operations}` and set the mock
datasource's `resultName: operations`.

`inputContract` describes the stored fixture; `resultContract` describes query
response encoding. They can differ, allowing record input to produce tabular
responses. Do not include JSON-RPC or MCP text-content wrappers in fixtures.
Column types, formats, nullability, and matrix widths are validated.

## Use parameters and inspect queries

In the browser, enter report parameters as JSON and click **Render report**. For
the minimal example or Operations Overview:

```json
{"region": ["North", "Central"]}
```

For Order Fulfillment, `scenario` accepts `Planned` or `Actual` and defaults to
`Actual`. Parameter bindings become typed predicates on the corresponding
datasource. Multiple-value parameters require JSON arrays.

For CLI compilation and exports, save parameter values to `parameters.json` and
pass `--parameters parameters.json`. For `serve`, set parameters in the browser
or its `parameters` URL query value; the CLI parameters-file flag is not applied
to server startup.

The **Datasource inspector** executes a normalized query against the selected
datasource. It does not automatically apply the report parameter bindings. Supply
any desired predicates explicitly. For the minimal example, use:

```json
{
  "projection": {
    "dimensions": ["region"],
    "measures": [{"field": "completedTasks", "aggregation": "sum", "alias": "total"}]
  },
  "filter": {"field": "region", "op": "in", "value": ["North", "Central"]},
  "orderBy": [{"field": "total", "direction": "desc"}],
  "page": {"limit": 10, "offset": 0}
}
```

Save this as `request.json` to inspect the same query from the CLI:

```sh
go run ./preview/cmd/report-preview describe ./my-report --datasource operations
go run ./preview/cmd/report-preview query ./my-report --datasource operations --request request.json
```

The output includes the encoded response, normalized rows, column metadata,
`hasMore`, and a deterministic fingerprint. Use `--request -` to read from stdin.

### Query rules

- Capabilities are opt-in; an omitted capability fails when requested.
- Processing order is filter, grouping/aggregation, projection, stable sorting,
  then paging. Filters can use fields not projected into the output.
- Predicates support nested `and`, `or`, and `not`; leaf operators include `eq`,
  `neq`, `in`, `notIn`, `lt`, `lte`, `gt`, `gte`, `between`, `isNull`, `isNotNull`,
  `contains`, `startsWith`, and `endsWith`.
- Comparisons are typed. Numeric strings are not numbers. Use null operators
  instead of ordinary comparisons against null. `between` includes both bounds.
- Measures must declare their permitted aggregation. Supported aggregations are
  `sum`, `min`, `max`, `count`, and `countDistinct`. `average` requires an explicit
  `rowLevel: true` declaration. Non-additive measures should use `none`.
- For ungrouped column selection, use `projection: {fields: [region, completedTasks]}`.
  Use `dimensions`/`measures` for grouped report queries; do not mix the two forms.
- Aliases change returned column names. Sort projected aliases by their output
  name. Nulls default to last in either direction; set `nulls: first` explicitly.
- Page offsets must be nonnegative and limits positive. Limits are capped by the
  configured maximum. Decimal strings preserve precision but do not support
  numeric operations in this provider.

## Exercise variants

Select a variant in the browser or pass `--variant` to the CLI:

```sh
go run ./preview/cmd/report-preview serve ./preview/report/examples/demo2 --variant nulls
```

| Variant | Expected behavior |
| --- | --- |
| `default` | Base fixtures |
| `empty` | Valid empty results |
| `error` | Stable datasource error response; a required source fails report readiness |
| `partial` | Available rows with upstream `hasMore: true`; preview works, full export fails |
| `nulls` | Nullable values; demo2 includes timing examples |
| `large` | Enough distinct rows to exercise multiple query pages |

These names are conventions, not automatic data generators. Add the corresponding
variant folder and any fixture overrides. The Operations Overview schema has no
nullable fields, so its `nulls` variant inherits the base data.

A valid error fixture has the form:

```json
{"status": "error", "message": "Synthetic datasource failure"}
```

All base fixtures must remain present and valid. An optional datasource is declared
with `optional: true` in its mock declaration; its failure leaves unrelated datasets
available and produces a preview diagnostic.

## Compile and export

```sh
# Canonical artifacts, using authored query page sizes:
go run ./preview/cmd/report-preview compile ./my-report --out compiled.json

# Canonical artifacts with complete queried datasets:
go run ./preview/cmd/report-preview compile ./my-report --full --out compiled-full.json

# Document exports always resolve complete queried datasets:
go run ./preview/cmd/report-preview export ./my-report --out report.pdf
go run ./preview/cmd/report-preview export ./my-report --out report.html

# Tabular exports select one report table block:
go run ./preview/cmd/report-preview export ./my-report --out tasks.csv --block regionalTable
go run ./preview/cmd/report-preview export ./my-report --out tasks.xlsx --block regionalTable
```

The output extension selects the export format. CSV and XLSX use Forge's
single-table exporter contract; `--block` is required to disambiguate reports with
multiple tables. Output parent directories must already exist.

The browser honors authored page sizes. **Download full PDF** and CLI exports drain
validated pages from offset zero and include every matching row. An upstream partial
fixture cannot establish a complete result, so full export returns
`mockPreviewIncompleteData` instead of presenting incomplete data as complete.

The compiler produces `ReportSpec`, `ReportFill`, and `ReportPrint`. Browser HTML
and PDF consume the same compiled print content, including formatted values and
chart SVG. The browser does not need a PDF plugin. Synthetic labels, variants,
fixture paths, and host diagnostics stay outside final report pages.

## Troubleshoot validation

CLI failures return a nonzero exit code and a structured diagnostic on stderr.
The browser displays the diagnostic above the report.

| Diagnostic | What to check |
| --- | --- |
| `mockPreviewMalformedPackage` | YAML structure, extension version, required `ds` directory, and declared contracts |
| `mockPreviewMissingDataSource` | Datasource IDs, matching fixture filenames, and dataset references |
| `mockPreviewMalformedTabularResult` | Status, result names, columns, row arrays, and boolean `hasMore` |
| `mockPreviewRowWidthMismatch` | Every matrix row must match its column count |
| `mockPreviewTypeMismatch` | Value type, nullability, parameter type, and integer precision |
| `mockPreviewUnknownColumn` | Projection, filter, block, or parameter-binding field references |
| `mockPreviewUnsupportedCapability` | Enable the requested operation in the datasource's mock query declaration |
| `mockPreviewUnsafeAggregation` | Match the declared aggregation and avoid summing non-additive values |
| `mockPreviewInvalidSort` / `mockPreviewInvalidPage` | Sort field/direction/null placement or page limit/offset |
| `mockPreviewPathEscape` | Paths and symlinks must remain inside the selected report folder |
| `mockPreviewIncompleteData` | Replace the partial fixture before requesting a complete export |

Warnings identify unused columns, unused fixture files, and variant overrides
identical to their base. A query failure is not permission to bypass validation.

## Limits and integration

Default limits are 2 MB per report YAML, 25 MB per fixture, 100,000 decoded rows,
500 columns, 100 predicate leaves, 20 grouping dimensions, 1,000 rows per page,
and eight concurrent datasource queries. Go hosts can lower them through
`provider.LoadWithLimits`; resource limits cannot be raised above these defaults.

The CLI selects the allowed report folder. Browser parameters cannot select
arbitrary filesystem locations or establish identity. The host is local-only,
checks request origins, and provides no authentication or authorization evidence.
The mock server does not call live datasources, interpolate environment variables,
or generate random data. The preview host connects to the MCP URL declared by each
datasource.

| Implementation | Responsibility |
| --- | --- |
| [`cmd/report-preview/main.go`](../cmd/report-preview/main.go) | Process entry point and shutdown signals |
| [`preview/report/cli.go`](../report/cli.go) | CLI commands and flags |
| [`preview/report/app.go`](../report/app.go) | Local HTTP host and preview endpoints |
| [`preview/report/provider`](../report/provider) | Folder loading, query execution, compilation, and exports |
| [`preview/mcp/mock`](../mcp/mock) | Shared filesystem-backed MCP server and basic queries |
| [`preview/datasource`](../datasource) | Shared `viant/mcp` client and endpoint resolver |
| [`backend/reporting/fenced`](../../../forge/backend/reporting/fenced) | Shared Forge report compiler |
| [`backend/reporting/export`](../../../forge/backend/reporting/export) | Shared PDF and tabular exporters |

See the [app reference](../report/README.md) and
[mock preview specification](../report/SPEC.md) for additional details.
To verify implementation changes, run:

```sh
go test -race ./preview/...
go vet ./preview/report/... ./preview/cmd/report-preview
```
