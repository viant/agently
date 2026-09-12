# Planned-class mock report preview extension

Status: proposed  
Examples: synthetic, business-neutral samples for public distribution.  
Implementation: [preview builder guide](../doc/report-preview-builder.md) and
[shared MCP datasource guide](../doc/mock-mcp-datasources.md).  
Updated: 2026-09-11

## 1. Required outcome

A report preview must be runnable from a self-contained folder containing:

1. one report definition in `report.yaml`; and
2. one JSON fixture per declared datasource under `ds/`.

The preview host must not require report-specific JavaScript. It must load the report
definition, expose the JSON files through a generic mock datasource provider, and run
the ordinary report compilation and rendering path.

The extension is renderer-agnostic. Web preview, PDF rendering, CLI inspection, and
other hosts must be able to consume the same folder without changing its contents.

## 2. Canonical folder contract

```text
reports/
  operations-comparison/
    report.yaml
    ds/
      primary.json
      advanced_summary.json
      operationsTotal.json
      operationsTeam.json
      operationsFacilityType.json
      operationsProcessType.json
      operationsRegion.json
      operationsDistrict.json
    variants/
      empty/
        ds/
          primary.json
          advanced_summary.json
          operationsTotal.json
          operationsTeam.json
          operationsFacilityType.json
          operationsProcessType.json
          operationsRegion.json
          operationsDistrict.json
      error/
        ds/
          operationsTotal.json
```

Rules:

- `report.yaml` is the authoritative report definition.
- `ds/` is required.
- A datasource fixture is named `<datasource-id>.json`.
- Datasource IDs and filenames are case-sensitive and must match exactly.
- A report definition cannot refer to undeclared fixture files.
- Every required datasource must have a fixture in the base `ds/` folder.
- A variant overlays the base folder by datasource ID; omitted files inherit the base
  fixture.
- Files outside the report folder cannot be resolved by relative paths.
- The folder contains data and declarative definitions only. Executable scripts are
  prohibited.

## 3. Report definition extension

The report engine's existing report grammar remains authoritative. Mock preview adds
one ignorable extension section named `x-mock-preview`; it does not define a second
report grammar.

```yaml
kind: reporting.report
id: operationsComparison
title: Operations Comparison

# Existing report definition: sections, blocks, datasets, filters, options,
# formats, visual bindings, and export configuration.
report:
  # Engine-owned report definition omitted from this example.

x-mock-preview:
  version: 1
  dataRoot: ./ds
  defaultVariant: default
  primaryDataSource: primary
  maxPageSize: 1000
  dataSources:
    primary:
      file: primary.json
      resultName: primary
      inputContract: {shape: tabular, resultsPath: data}
      query:
        projection: true
        filtering: true
        grouping: true
        sorting: true
        pagination: offset
      dimensions:
        - operationsDate
        - region
      measures:
        processedUnits: {aggregation: sum}
        operatingCost: {aggregation: sum}
    operationsTotal:
      file: operationsTotal.json
      resultName: operationsTotal
      inputContract: {shape: records, rowPath: rows, hasMorePath: hasMore}
      query:
        projection: true
        filtering: true
        grouping: false
        sorting: true
        pagination: offset
      dimensions:
        - comparisonGroup
      measures:
        completedUnits: {aggregation: sum}
        totalUnits: {aggregation: sum}
        completionRate: {aggregation: none}
```

### 3.1 Extension rules

- Hosts that do not support mock preview may ignore `x-mock-preview`.
- Preview-capable hosts must reject an unsupported extension version.
- `dataRoot` is resolved relative to `report.yaml`.
- The map key under `dataSources` is the report datasource ID.
- `file` defaults to `<datasource-id>.json` and normally should be omitted.
- `resultName` identifies the tabular result inside the datasource response.
- `inputContract.shape` is either `tabular` or `records`.
- `tabular` consumes the public MCP-style column/matrix response without reshaping
  the fixture.
- `records` consumes ordinary JSON object rows from the declared `rowPath`.
- Query capabilities are opt-in. An omitted or false capability must fail explicitly
  when requested; it must not be silently emulated.
- The report definition remains unchanged when switching between mock and live
  transport.

## 4. Datasource JSON contract

Each file contains the raw response body that the real datasource would return. For
Business Reporting, this is the public `BusinessReportingRun` tabular contract—not a
renderer-specific array of objects.

```json
{
  "status": "ok",
  "data": [
    {
      "name": "operationsTotal",
      "columns": [
        {
          "name": "comparisonGroup",
          "type": "string",
          "role": "dimension",
          "nullable": false
        },
        {
          "name": "completedUnits",
          "type": "integer",
          "role": "measure",
          "format": "compactNumber",
          "nullable": false
        },
        {
          "name": "totalUnits",
          "type": "integer",
          "role": "measure",
          "format": "compactNumber",
          "nullable": false
        },
        {
          "name": "completionRate",
          "type": "number",
          "role": "measure",
          "format": "percentFraction",
          "nullable": true
        }
      ],
      "rows": [
        ["Baseline", 202000, 142000000, 0.0014225352],
        ["Current", 1300000, 130400000, 0.0099693252]
      ],
      "hasMore": false
    }
  ],
  "meta": {
    "groupId": "operations",
    "reportId": "summary",
    "family": "operations_summary",
    "source": "operations_summary_result",
    "cached": false,
    "resultSet": "operationsTotal"
  }
}
```

### 4.1 One-to-one transport parity

- The file body must be valid input to the same response decoder used for a live
  datasource call.
- `data[].columns` and `data[].rows` retain the wire-level tabular representation.
- A row is an array whose values correspond by index to `columns`.
- The mock loader must not require object-shaped rows.
- Column name, type, role, format, nullability, and order must match the live public
  contract.
- `hasMore` has the same meaning as the live response.
- `status`, `message`, and `meta` must remain available to the host.
- The mock provider may create a new response after applying a query, but that
  response must use the same envelope and tabular encoding.
- MCP protocol wrappers, such as JSON-RPC IDs or text-content framing, are not stored
  in the fixture. The fixture starts at the public tool's decoded response body.

### 4.2 Ordinary record JSON

A datasource may instead use ordinary object-row JSON when the live provider is not
tabular or a smaller hand-authored fixture is more useful:

```json
{
  "status": "ok",
  "rows": [
    {"region": "North", "processedUnits": 1840000, "operatingCost": 28120.5},
    {"region": "Central", "processedUnits": 920000, "operatingCost": 9400.0}
  ],
  "hasMore": false,
  "meta": {"source": "mock-records"}
}
```

Its report declaration supplies extraction paths:

```yaml
inputContract:
  shape: records
  rowPath: rows
  hasMorePath: hasMore
```

Rules:

- `rowPath` may resolve to an array of objects or to `$` for a bare top-level array.
- Every returned object key used by the report must have column metadata in the
  report definition.
- Missing object properties decode as null only when the column is nullable.
- Extra object properties remain unavailable unless declared as columns.
- Record fixtures and tabular fixtures produce the same normalized logical row model
  before filtering, grouping, projection, sorting, and paging.
- Query results are encoded in the datasource's declared output contract. A mock of
  an MCP tabular datasource must therefore return tabular output even if its stored
  fixture uses records.
- `records` is a convenience input form, not permission to change the production
  datasource's public response contract.

## 5. Mock datasource provider interface

The extension requires a provider with a renderer-neutral interface:

```text
loadReport(reportLocation, variant?) -> ReportPackage
describeDataSource(dataSourceId) -> DataSourceDescription
execute(dataSourceId, QueryRequest) -> TabularResponse
```

`ReportPackage` contains the parsed report definition and datasource declarations.
`TabularResponse` is the same response body shape used by the live datasource.

The renderer receives no filesystem paths and must not know whether the provider is
mock, MCP, HTTP, Datly, or another transport.

## 6. Query request

The provider must accept a normalized query independent of UI controls:

```json
{
  "dataSource": "primary",
  "projection": {
    "dimensions": ["operationsDate", "region"],
    "measures": [
      {"field": "processedUnits", "aggregation": "sum"},
      {"field": "operatingCost", "aggregation": "sum"}
    ]
  },
  "filter": {
    "and": [
      {"field": "organizationId", "op": "eq", "value": 45322},
      {"field": "operationsDate", "op": "between", "value": ["2026-08-01", "2026-08-31"]},
      {"field": "region", "op": "in", "value": ["North", "Central"]}
    ]
  },
  "orderBy": [
    {"field": "operationsDate", "direction": "asc"},
    {"field": "processedUnits", "direction": "desc"}
  ],
  "page": {"limit": 100, "offset": 0}
}
```

### 6.1 Evaluation order

The provider must evaluate every query in this order:

1. validate datasource and request;
2. decode the selected tabular result into typed logical rows;
3. apply predicates;
4. apply grouping and measure aggregation when requested;
5. apply column projection and aliases;
6. apply stable sorting;
7. calculate total matching row count when enabled;
8. apply offset and limit;
9. encode the result back into the tabular response contract.

Filtering after paging, paging before sorting, or projecting away fields needed by a
filter or aggregation is invalid behavior.

## 7. Projection and grouping

### 7.1 Projection

- A query may select a subset of declared columns.
- Returned columns must appear in requested order.
- Unknown columns are rejected.
- A field cannot be returned twice unless one occurrence has an explicit unique
  alias.
- An alias changes only the returned column name; it does not change fixture schema.
- Projection must preserve type, role, format, provenance, and nullability metadata.

### 7.2 Grouping

- Requested dimensions form the grouping key in request order.
- Measures must declare an allowed aggregation in `report.yaml`.
- Initial required aggregations are `sum`, `min`, `max`, `count`, and
  `countDistinct`.
- `average` is permitted only when the fixture contains authoritative numerator and
  denominator inputs or the datasource explicitly declares row-level values.
- Ratios, rates, growth rates, unit costs, return on investment, shares, and other non-additive measures must use
  `aggregation: none` unless the report definition provides a deterministic typed
  finalizer with its required additive inputs.
- The provider must never sum a non-additive measure.
- When grouping is disabled, the requested dimensions and measures may only project
  existing fixture columns; they cannot change grain.
- Group output order is not implied. A stable `orderBy` must be applied when report
  behavior depends on order.

## 8. Filtering

The required predicate tree supports `and`, `or`, and `not` groups and these leaf
operators:

```text
eq, neq
in, notIn
lt, lte, gt, gte
between
isNull, isNotNull
contains, startsWith, endsWith
```

Rules:

- Operators are validated against column type.
- `between` is inclusive.
- `in` and `notIn` require an array.
- String matching is case-sensitive by default. A datasource may declare
  `caseSensitive: false`.
- Null is distinct from an empty string and zero.
- An ordinary comparison against null is invalid; use `isNull` or `isNotNull`.
- Date values use civil `YYYY-MM-DD` comparison unless the declared type is a
  timestamp.
- Numeric strings are not silently coerced into numbers.
- Undeclared filter fields and unsupported operators fail the request.
- Empty predicate groups are rejected rather than interpreted as unrestricted
  authorization.

Mock filtering is functional behavior only. It is not authorization and must not be
used as evidence that organization or project access was enforced.

## 9. Sorting and pagination

- `orderBy` supports multiple fields with `asc` or `desc` direction.
- Sort fields may reference projected dimensions, projected measures, or declared
  hidden sort fields.
- Null ordering is explicit and defaults to `last` for both directions.
- Sorting must be stable. Original decoded row position is the final tie-breaker.
- Version 1 requires offset pagination with non-negative `offset` and positive
  `limit`.
- `limit` is capped by `x-mock-preview.maxPageSize`.
- `hasMore` is true when another matching row exists after the returned page.
- An optional `totalRows` may be returned in `meta` when the datasource declares
  `includeTotalRows: true`.
- The same request and fixture must produce identical pages across runs.
- Cursor pagination may be added in a later extension version; it must not be
  simulated with an unstable row index.

## 10. Multiple datasources

- Every report dataset resolves through its declared datasource ID.
- Each datasource executes independently with its own query, paging state, and
  result name.
- A datasource file may contain multiple named tabular results only when the live
  datasource does the same.
- `resultName` selects one result for query execution; missing or duplicate names are
  errors.
- The provider cannot join files implicitly.
- Cross-datasource composition belongs to the report definition or an explicitly
  declared transformation supported identically by live and mock providers.
- A report may load primary, summary, evidence, and drill datasets concurrently.
- Failure of an optional datasource must remain scoped to blocks that consume it.
  Failure of a required datasource fails report readiness.

## 11. Variants and states

Required conventional variants are:

```text
default    populated representative data
empty      valid responses with zero rows
error      stable datasource error responses
partial    populated response with hasMore=true
nulls      valid nullable values at meaningful positions
large      enough rows to exercise paging and table virtualization
```

Variant rules:

- A variant changes datasource responses, not the report definition.
- Error fixtures use the live response's stable public error shape.
- A variant may override only affected datasource files.
- The selected variant must be visible in preview diagnostics but omitted from final
  report and PDF content.
- Synthetic fixtures must be visibly labeled in preview mode.

## 12. Web, PDF, and other renderers

All renderers use the same sequence:

```text
report.yaml
    +
mock datasource provider -> queried tabular responses
    |
report compiler/runtime
    +-- web renderer
    +-- PDF renderer
    +-- image/screenshot renderer
    +-- CLI inspector
```

Requirements:

- PDF preview must not use a separate mock-data format.
- Web and PDF must receive the same compiled report definition and queried rows.
- Renderer-specific layout differences are allowed; data selection and values are
  not.
- Export must use the full queried result required by the report, not only the
  currently visible browser page.
- A renderer cannot bypass datasource query validation.

## 13. Preview selection and launch contract

A host may expose a URL or CLI, but launch parameters only select local artifacts:

```text
report-preview serve ./reports/operations-comparison --variant default
report-preview query ./reports/operations-comparison --datasource operationsTotal --request request.json

/report-preview?report=operations-comparison&variant=default
```

Optional `organizationId`, `projectIds`, date, filter, or option parameters become
initial query values. They do not establish user identity or authorization.

The report folder or report ID must be allowlisted by the preview host. Arbitrary
filesystem paths supplied by browser query parameters are prohibited.

## 14. Authentication and authorization boundary

- Mock preview does not authenticate a user.
- Mock preview does not prove subscription, organization, project, or contributor
  authorization.
- A mock principal may be supplied only to exercise conditional presentation and
  must be labeled synthetic.
- Live MCP execution must continue to resolve the authenticated principal server-side
  and authorize scope before running Datly/DQL.
- Query-string scope is never trusted as authorization.
- No token, cookie, credential, or local cloud credential may be stored in a report
  folder.

### Remote definition interoperability

Remote bootstrap calls the manifest-declared MCP tool with `describe` and generic
`groupId + reportId`. Two response levels are supported:

1. Production describe: `status: ok` plus exactly one report containing a field
   catalog and optional named result sets. Agently generates a neutral generic report
   and maps projection, conjunctive scope parameters, ordering, limit, and offset into
   the same tool's `run` request.
2. Optional authored extension: `previewDefinition` contains the complete generic
   report package generated by the report owner. Agently validates its identity and
   prefers it over generic fallback.

The production result-set descriptor may expose only public names, dimensions,
measures, and columns. Internal component IDs, SQL names, source factors, cache keys,
and authorization implementation are not part of the preview contract. `run` remains
responsible for authorization and must fail closed.

## 15. Validation requirements

A validator must fail a report folder when:

- `report.yaml` is absent or invalid;
- the extension version is unsupported;
- a datasource ID is unsafe, duplicated, or missing its JSON file;
- a required fixture is absent;
- a response status or tabular result is malformed;
- column names are empty or duplicated;
- row width differs from column count;
- a value violates the declared column type or nullability;
- report fields reference missing datasource columns;
- a requested capability is not declared;
- an aggregation is invalid for a measure;
- a non-additive measure is configured with an unsafe aggregation;
- paging limits are invalid;
- a variant changes the report definition;
- a path escapes the report folder.

Warnings should cover unusually large fixtures, unused columns, unused datasource
files, and variant files identical to the base fixture.

## 16. Determinism and type handling

- YAML parsing must use safe mode with no executable tags.
- JSON numbers, booleans, strings, arrays, objects, and nulls retain their types.
- Integer precision beyond the host's safe numeric range requires a declared decimal
  string representation.
- Civil dates remain strings until interpreted through declared date metadata.
- Timestamps require an explicit timezone or UTC suffix.
- The provider must not call the network, current clock, random generator, or live
  datasource while executing a fixture query.
- Query fingerprints are built from canonical request JSON and fixture content hash.
- Identical package, variant, and request inputs produce identical responses.

## 17. Performance and safety limits

Defaults, overridable downward by the host:

```text
maximum report YAML size          2 MB
maximum datasource file size     25 MB
maximum decoded rows             100,000 per datasource
maximum columns                  500 per tabular result
maximum predicate leaves         100
maximum grouping dimensions      20
maximum page size                1,000
maximum concurrent datasource queries 8
```

The provider must reject exceeded limits with stable diagnostics. It must not execute
code, interpolate environment variables, follow remote URLs, or resolve symbolic
links outside the report folder.

## 18. Diagnostics

Every failure includes:

```json
{
  "code": "mockPreviewUnknownColumn",
  "message": "Datasource operationsTotal does not declare column projectId.",
  "path": "x-mock-preview.dataSources.operationsTotal",
  "dataSource": "operationsTotal",
  "requestField": "filter.and.0.field"
}
```

Diagnostics may expose local fixture paths in developer preview. They must not be
rendered into final report content or PDF output.

Required stable codes include malformed package, missing datasource, malformed
tabular result, row-width mismatch, type mismatch, unknown field, unsupported
operator, unsupported capability, unsafe aggregation, invalid sort, invalid page,
and path escape.

## 19. Acceptance criteria

- A new report preview is added with YAML and JSON only; no report-specific code is
  added to the preview host.
- Renaming a datasource without renaming its file fails validation.
- Each JSON file can be replaced by a captured decoded live MCP response without
  changing the loader.
- The same normalized request produces contract-equivalent mock and live response
  shapes.
- Filtering supports nested predicates and typed comparisons.
- Projection preserves requested column order and metadata.
- Grouping aggregates only declared additive measures.
- Sorting is stable and deterministic.
- Offset/limit pagination and `hasMore` are correct after filtering, grouping,
  projection, and sorting.
- Primary and named secondary datasources can be queried independently.
- Empty, error, partial, null, and large variants render through the standard runtime.
- Web and PDF consume the same report definition and datasource results.
- Mock mode is visually identifiable in preview but adds no noise to final output.
- Query parameters cannot grant authorization or access arbitrary files.
- Live behavior is unchanged when no mock provider is selected.

## 20. Non-goals

- Replacing MCP, Datly, DQL, or production authorization.
- Defining a new chart, table, report, or PDF grammar.
- Embedding report fixtures in JavaScript.
- Adding report-specific query handlers to the preview host.
- Inferring missing measures or fabricating unavailable semantics.
- Joining independent datasource fixtures implicitly.
- Treating screenshot parity as proof of numeric or authorization parity.

## 21. Included examples

Two report folders accompany this specification:

```text
forge-rep-prev/demo/
  report.yaml
  ds/operations.json          decoded MCP tabular response
  ds/projectTargets.json   ordinary object-row JSON

forge-rep-prev/demo2/
  report.yaml
  ds/orders.json       ordinary object-row JSON
  ds/processTiming.json       ordinary object-row JSON
```

`demo` proves that a datasource file can preserve the public MCP tabular envelope.
It also shows that input shape is selected per datasource rather than per report.
`demo2` proves that an entire report can run from regular JSON records while retaining
the same filtering, projection, grouping, sorting, paging, web, and PDF requirements.
