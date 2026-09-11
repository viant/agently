# Report preview app

For a complete authoring walkthrough, see the
[report preview builder guide](../doc/report-preview-builder.md).

A Go preview application inside agently. Its CLI bootstraps a local HTTP server;
no Node build, agently-core server, separately managed MCP server, or report-specific
JavaScript is required. The app starts a shared mock MCP endpoint at `/mcp`, and
report datasource queries use `viant/mcp` to connect to it. Each report is a self-contained `report.yaml` plus `ds/*.json` folder.

The public demos contain synthetic, business-neutral examples: `demo` covers regional
operations and project targets; `demo2` covers order fulfillment and process timing.
Names, identifiers, and values are illustrative and do not identify real customers.

From the agently repository root:

```sh
go run ./preview/cmd/report-preview serve ./preview/report/examples/demo
# Open http://127.0.0.1:8095

go build -o /tmp/report-preview ./preview/cmd/report-preview
/tmp/report-preview serve /path/to/report --variant empty --addr 127.0.0.1:8096
```

The folder supplied at startup is the host's allowlist. Browser requests cannot
change that folder. Fixture changes are loaded on the next render/query; restart
only when changing app code. Ctrl-C gracefully stops the server.

## CLI

```sh
go run ./preview/cmd/report-preview validate ./preview/report/examples/demo
go run ./preview/cmd/report-preview describe ./preview/report/examples/demo --datasource operations

go run ./preview/cmd/report-preview query ./preview/report/examples/demo \
  --datasource operations --request request.json

go run ./preview/cmd/report-preview compile ./preview/report/examples/demo \
  --full --out compiled.json

go run ./preview/cmd/report-preview export ./preview/report/examples/demo --out report.pdf
go run ./preview/cmd/report-preview export ./preview/report/examples/demo --out report.html
go run ./preview/cmd/report-preview export ./preview/report/examples/demo \
  --out operations.csv --block operationsTable
go run ./preview/cmd/report-preview export ./preview/report/examples/demo \
  --out operations.xlsx --block operationsTable
```

Use `--parameters parameters.json` for report parameter values, `--variant name`
for overlays, and `--request -` to read a query from stdin. Flags may appear before
or after the folder. Structured diagnostics go to stderr with a nonzero exit code.
CSV and XLSX use Forge's existing single-table exporter contract; `--block` selects
one table when the report has multiple tables. Full exports drain validated query
pages from offset zero. A partial fixture with upstream `hasMore=true` is previewable
but cannot be represented as a complete export.

Example `request.json`:

```json
{
  "projection": {
    "dimensions": ["region"],
    "measures": [{"field": "processedUnits", "aggregation": "sum", "alias": "total"}]
  },
  "filter": {"field": "operationsDate", "op": "gte", "value": "2026-08-02"},
  "orderBy": [{"field": "total", "direction": "desc"}],
  "page": {"limit": 10, "offset": 0}
}
```

## Authoring and compiler boundary

The business-neutral `demo` and `demo2` folders use the `ReportDefinition` sample
shape. An explicit generic adapter translates their sections, KPI groups, tables,
and charts to Forge's existing `report-document-v1` blocks. It does not introduce
new rendering semantics. Native Forge authoring is available under `report`:

```yaml
kind: reporting.report
report:
  id: operations
  title: Operations
  blocks:
    - id: evidence
      kind: tableBlock
      datasetRef: operationsEvidence
      columns:
        - {key: region, label: Region}
        - {key: processedUnits, label: Processed Units, format: compactNumber}
# dataSources, datasets, parameters, and x-mock-preview accompany this section
# exactly as in the example folders.
```

Each datasource declares `service: {endpoint: mockReports, uri: operations, method: POST}`
(with its own tool name), and `endpoints.mockReports` declares
`{type: mcp, transport: streamable, baseURL: /mcp}`. The shared MCP client resolves
these services and fetches dataset rows through actual MCP tool calls. Transport
failures never fall back to local fixture execution.

The provider resolves declared datasets, and the app passes those rows and native
blocks into `backend/reporting/fenced.Compile`. Forge produces canonical
`ReportSpec`, `ReportFill`, and `ReportPrint`, matching agently-core's reporting
boundary. The app does not modify production execution or authentication.

The browser presents the compiled ReportPrint pages as HTML/SVG, avoiding a PDF
plugin dependency. PDF uses Forge's PDF renderer. Both consume the same compiled
page geometry, formatted values, and chart SVG. The default browser view honors
authored page sizes; full downloads resolve all matching rows. The synthetic badge,
selected variant, fixture paths, and diagnostics remain outside final report pages.

## Provider contract

`provider.Load`, `LoadWithLimits`, `DescribeDataSource`, and `Execute` are reusable
without HTTP for validation. `UseMCP` attaches the shared datasource transport for
runtime queries. CLI query/compile/export start an ephemeral MCP endpoint. Query results include the public response envelope, logical rows,
column metadata, paging status, and a deterministic fingerprint. Fixtures are
immutable across concurrent calls. Fingerprints include the declaration, variant,
fixture content, datasource identity, and canonical request JSON.

Tabular fixtures retain named column/matrix results and envelope metadata. Record
fixtures use explicit extraction paths, discard undeclared keys, and encode query
results according to `resultContract`. Column types are validated without numeric
string coercion. Civil dates remain strings; timestamps require a timezone. An
aggregate that can return null advertises nullable output metadata, while pure
projection preserves source metadata.

Capabilities are opt-in. Nested `and`/`or`/`not`, all specification leaf operators,
aliases, stable multi-field ordering, null placement, offset paging, and declared
sum/min/max/count/countDistinct aggregations are supported. Average requires an
explicit `rowLevel: true` measure declaration. Non-additive measures must declare
`aggregation: none`; typed finalizers and numerator/denominator average plans are
not defined by the supplied version-1 grammar and are not inferred. Decimal strings
are retained exactly; numeric operations on them are rejected rather than rounded.
The fixture query engine uses no network, clock, environment interpolation, or
random data; the preview fetches its results through the declared MCP endpoint.

## Variants and limits

Examples include `empty`, `error`, `partial`, `nulls`, and `large` overlays. Omitted
files inherit their base fixture. `demo` has no nullable fields, so its `nulls`
variant inherits the base; `demo2` demonstrates actual nullable timing values.
Large fixtures contain distinct dates to exercise paging across more than 1,000
rows. Required datasource errors fail readiness; optional failures retain unrelated
datasets and surface a host diagnostic.

The loader rejects duplicate YAML keys, executable tags/aliases, script files,
unsafe IDs, symlink/path escapes, missing fixtures, malformed schemas/rows, unsafe
aggregations, and unsupported capabilities. Every named tabular result is validated.
Warnings identify unused columns/files and redundant variant overrides.

Defaults: 2 MB YAML, 25 MB per fixture, 100,000 rows, 500 columns, 100 predicate
leaves, 20 grouping dimensions, 1,000 rows per page, and eight concurrent queries.
Embedding hosts can lower these through `LoadWithLimits`. The HTTP app also bounds
request bodies, header sizes, timeouts, origins, and concurrent work. It binds only
to loopback. This mock host provides no authentication or authorization evidence.

## Verification

```sh
go test -race ./preview/...
go vet ./preview/report/... ./preview/cmd/report-preview
```

Tests cover both demos, every conventional variant, typed predicates, grouping,
aliases, aggregation precision, wire envelopes, null ordering, immutable concurrent
execution, full exports, native blocks, malformed inputs, path escapes, HTTP host
and origin restrictions, and CLI behavior. The shared compiler also has regression
coverage for PDF-supported line paths and horizontal bars.

The shared server also has a standalone CLI: `go run ./preview/cmd/mock-mcp --root ./fixtures`.
See [shared mock MCP datasources](../doc/mock-mcp-datasources.md).
