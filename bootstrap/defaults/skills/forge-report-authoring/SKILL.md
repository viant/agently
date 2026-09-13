---
name: forge-report-authoring
description: Generate interactive Forge reports from supplied tabular data using forge-data and forge-report fences, including charts, tables and shared filters.
---

# Forge report authoring

Use the workspace `analytics_dashboard` output template when template tools are exposed: call `template:list`, then `template:get` with `includeDocument: true`. This skill supplements that contract; it does not replace the reporting runtime or create an HTML dashboard.

Use the complete transaction example below before emitting report metadata. The same example is available in [references/fences.md](references/fences.md); no filesystem search is necessary.

- Emit each materialized dataset first as `forge-data` version 2. Use raw numeric values and lowercase dataset IDs.
- Follow with `forge-report` version 1, `mode: start`, `grammar: dashboard-v1`, and supported dashboard blocks. Finish with `mode: commit`.
- Keep `scope` stable, `forge-data.reportRef` equal to `forge-report.id`, and block `dataSourceRef` equal to the dataset ID. Sequences strictly increase across both fence types.
- Use `kind`, not `type`, for blocks. Chart type belongs inside `chart`. A line chart uses `chart.xAxis.dataKey` and `chart.series.valueKey`.
- Include a table so the reader can inspect all source rows. Do not fabricate data unless the user requests synthetic examples. Label synthetic data clearly.
- Filters require both a `dashboard.filters` control and matching `filterBindings` on each affected block; a filter control alone does not filter the report.
- Do not claim a generated report was rendered, exported, or validated unless the host/tool result confirms it. Valid fences may still expose a host integration defect.
- For a repair after a committed report, emit a complete replacement report under a new report ID and scope unless the host explicitly supports continuing that assembly. Do not replay sequence 1 into the same committed assembly.

Workspace windows are a separate delivery route: inspect the host window metadata and registration contracts before writing a persistent window. These report fences create an inline report, not a registered workspace window.

## Embedded transaction example

# Complete synthetic example

This is a schema illustration. Replace its rows and IDs with the user's report. Keep the three fences separate and emit bare JSON inside them.

```forge-data
{"version":2,"scope":"support_example","reportRef":"support_weekly","id":"weekly_rows","sequence":1,"format":"json","mode":"replace","data":[{"week":"2026-09-01","completed":12},{"week":"2026-09-08","completed":18}]}
```

```forge-report
{"version":1,"scope":"support_example","id":"support_weekly","sequence":2,"mode":"start","grammar":"dashboard-v1","title":"Support activity — synthetic example","blocks":[{"id":"week_filter","kind":"dashboard.filters","title":"Week","items":[{"id":"week","field":"week","label":"Week","options":[{"label":"September 1","value":"2026-09-01"},{"label":"September 8","value":"2026-09-08"}]}]},{"id":"completed_chart","kind":"dashboard.timeline","title":"Completed tickets","dataSourceRef":"weekly_rows","filterBindings":{"week":"week"},"chart":{"type":"line","xAxis":{"dataKey":"week"},"series":{"valueKey":"completed"}}},{"id":"weekly_detail","kind":"dashboard.table","title":"Weekly detail","dataSourceRef":"weekly_rows","filterBindings":{"week":"week"},"columns":[{"key":"week","label":"Week"},{"key":"completed","label":"Completed"}]}]}
```

```forge-report
{"version":1,"scope":"support_example","id":"support_weekly","sequence":3,"mode":"commit"}
```

The filter's `field: week` names dashboard state. Each binding maps that state key to the row field `week`. Without a selected week all rows are visible. Do not mark one option as default unless the report should initially restrict its data.

For repository-based authoring, the authoritative implementation references are `forge/doc/reporting.md`, `forge/src/reporting/inlineReportCompiler.js`, and `forge/src/reporting/dashboardReportAdapter.js`. `dashboard-v1` is the established workspace output-template grammar; canonical `report-document-v1` is also supported but has different block shapes and must not be mixed with dashboard blocks.
