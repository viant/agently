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
