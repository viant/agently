import { describe, expect, it } from 'vitest';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';

import RichContent, { normalizeDashboardPayload, parseFences, resolveForgeScope, scopeForgeDashboardPayload } from './RichContent';
import { renderMarkdownBlock } from 'agently-core-ui-sdk';

describe('RichContent fence parsing', () => {
  it('parses a closed fenced code block', () => {
    const parts = parseFences('Before\n```go\nfmt.Println("hi")\n```\nAfter');

    expect(parts).toHaveLength(3);
    expect(parts[1]).toMatchObject({
      kind: 'fence',
      lang: 'go',
      body: 'fmt.Println("hi")\n'
    });
  });

  it('parses an unterminated trailing fenced block for streaming content', () => {
    const parts = parseFences('```go\nfmt.Println("streaming")\nfor i := 0; i < 3; i++ {\n');

    expect(parts).toHaveLength(1);
    expect(parts[0]).toMatchObject({
      kind: 'fence',
      lang: 'go'
    });
    expect(parts[0].body).toContain('fmt.Println("streaming")');
    expect(parts[0].body).toContain('for i := 0; i < 3; i++ {');
  });

  it('parses compact json fence form used by some streamed outputs', () => {
    const parts = parseFences('<!-- CHART_SPEC:v1 -->\n```json{"version":"1.0"}\n```');

    expect(parts).toHaveLength(2);
    expect(parts[1]).toMatchObject({
      kind: 'fence',
      lang: 'json',
      body: '{"version":"1.0"}\n'
    });
  });

  it('parses compact forge fences used by streamed planner output', () => {
    const parts = parseFences([
      '```forge-data{"version":1,"id":"recommended_sites","format":"json","mode":"replace","data":[]}',
      '```',
      '',
      '```forge-ui{"version":1,"title":"Review recommended site lists","blocks":[]}',
      '```',
    ].join('\n'));

    expect(parts).toHaveLength(3);
    expect(parts[0]).toMatchObject({
      kind: 'fence',
      lang: 'forge-data',
    });
    expect(parts[0].body).toContain('"recommended_sites"');
    expect(parts[2]).toMatchObject({
      kind: 'fence',
      lang: 'forge-ui',
    });
    expect(parts[2].body).toContain('"Review recommended site lists"');
  });

  it('renders markdown headings as heading tags', () => {
    const html = renderMarkdownBlock('## Cat Story\n\nA short paragraph.');
    expect(html).toContain('<h2>Cat Story</h2>');
    expect(html).toContain('A short paragraph.');
  });

  it('rewrites sandbox markdown links to generated file download URLs', () => {
    const html = renderToStaticMarkup(
      React.createElement(RichContent, {
        content: 'Created [mouse_story.pdf](sandbox:/mnt/data/mouse_story.pdf).',
        generatedFiles: [{ id: 'gf-123', filename: 'mouse_story.pdf', status: 'ready' }]
      })
    );

    expect(html).toContain('/v1/api/generated-files/gf-123/download');
    expect(html).not.toContain('sandbox:/mnt/data/mouse_story.pdf');
  });

  it('separates a collapsed markdown heading from a following pipe table', () => {
    const html = renderToStaticMarkup(
      React.createElement(RichContent, {
        content: '### Daily Trend | Date | Value |\n|---|---:|\n| 2026-04-02 | 1 |\n'
      })
    );

    expect(html).toContain('<h3>Daily Trend</h3>');
    expect(html).not.toContain('Daily Trend | Date | Value |');
    expect(html).toContain('bp6-table-container');
  });

  it('separates a collapsed bold label from a following pipe table', () => {
    const html = renderToStaticMarkup(
      React.createElement(RichContent, {
        content: '**Raw Evidence**| publisher | bids |\n|---|---:|\n| OpenX | 751034737 |\n'
      })
    );

    expect(html).toContain('<strong>Raw Evidence</strong>');
    expect(html).toContain('bp6-table-container');
    expect(html).not.toContain('<th>**Raw Evidence**');
  });

  it('separates a heading glued directly to a following pipe table', () => {
    const html = renderToStaticMarkup(
      React.createElement(RichContent, {
        content: '### Underlying Data — Recommendation2| Publisher | Bids |\n|---|---:|\n| OpenX | 751034737 |\n'
      })
    );

    expect(html).toContain('<h3>Underlying Data');
    expect(html).toContain('bp6-table-container');
    expect(html).not.toContain('Recommendation2| Publisher');
  });

  it('separates a collapsed bold label from a following bullet list', () => {
    const html = renderToStaticMarkup(
      React.createElement(RichContent, {
        content: '**Evidence context**- **Metric window:** 2026-03-31 to 2026-04-06\n- **Compared entities:** Top global publishers by bids'
      })
    );

    expect(html).toContain('<strong>Evidence context</strong>');
    expect(html).toContain('<ul>');
    expect(html).toContain('Metric window:');
    expect(html).toContain('Compared entities:');
  });

  it('normalizes collapsed forecasting bullets and labels', () => {
    const html = renderToStaticMarkup(
      React.createElement(RichContent, {
        content: [
          '**Top Summary**',
          '',
          '- Deal:141952- Best available day: Day1-3-day total inventory:13,656,079,835- Average clearing price: $9.91',
          '',
          '| Day | Status | Inventory |',
          '|---|---|---:|',
          '| Day1 | complete |13,656,079,835 |',
          '',
          '<!-- CHART_SPEC:v1 -->',
          '```json{"version":"1.0","title":"3-Day Deal Forecast Inventory","chart":{"type":"bar","x":{"key":"day"},"series":{"key":"series"},"y":[{"key":"inventory","label":"Inventory"}]},"data":[{"day":"Day1","series":"forecast","inventory":13656079835}],"meta":{"source":"steward-forecasting"}}',
          '```'
        ].join('\n')
      })
    );

    expect(html).toContain('<strong>Top Summary</strong>');
    expect(html).toContain('Deal: 141952');
    expect(html).toContain('Best available day: Day 1');
    expect(html).toContain('3-day total inventory: 13,656,079,835');
    expect(html).toContain('bp6-table-container');
    expect(html).toContain('Day 1');
    expect(html).toContain('app-rich-chart');
  });

  it('repairs the exact compact forecasting output shape seen in live runs', () => {
    const content = [
      '**Top Summary**',
      '',
      '- Deal:141952- Best available day: Day1-3-day total inventory:13,656,079,835- Average clearing price: $9.91- Total uniques:123,307,724- Total HH uniques:19,528,600- Completed days with no data: Day2, Day3',
      '',
      '**Daily Breakdown**',
      '',
      '| Day | Status | Inventory | Clearing Price | Uniques | HH Uniques |',
      '|---|---|---:|---:|---:|---:|',
      '| Day1 | complete |13,656,079,835 | $9.91 |123,307,724 |19,528,600 |',
      '| Day2 | no data | n/a | n/a | n/a | n/a |',
      '| Day3 | no data | n/a | n/a | n/a | n/a |',
      '',
      '**Trend Chart**',
      '',
      '- Inventory is concentrated entirely on Day1.',
      '- Day2 and Day3 completed, but no forecast metrics were returned.',
      '',
      '<!-- CHART_SPEC:v1 -->',
      '```json{',
      ' "version": "1.0",',
      ' "title": "3-Day Deal Forecast Inventory",',
      ' "chart": {',
      ' "type": "bar",',
      ' "x": {"key": "day"},',
      ' "series": {"key": "series"},',
      ' "y": [{"key": "inventory", "label": "Inventory"}]',
      ' },',
      ' "data": [',
      ' {"day": "Day1", "series": "forecast", "inventory":13656079835}',
      ' ],',
      ' "meta": {"source": "steward-forecasting"}',
      '}',
      '```'
    ].join('\n');

    const html = renderToStaticMarkup(
      React.createElement(RichContent, { content })
    );

    expect(html).toContain('Deal: 141952');
    expect(html).toContain('Best available day: Day 1');
    expect(html).toContain('3-day total inventory: 13,656,079,835');
    expect(html).toContain('Total uniques: 123,307,724');
    expect(html).toContain('Total HH uniques: 19,528,600');
    expect(html).toContain('bp6-table-container');
    expect(html).toContain('Day 1');
    expect(html).toContain('app-rich-chart');
  });

  it('repairs compact live forecasting output with digit-led bullet labels', () => {
    const content = [
      '**Top Summary**',
      '',
      '- Deal:142479- Best available day: Day2-3-day total inventory:1,236,852,296- Average clearing price: $6.38- Uniques are available only for Day2.',
      '- Completed days with no data: Day1, Day3',
      '',
      '**Planning Notes**',
      '',
      '- The setup looks uneven across the3-day view, so delivery may be timing-sensitive.',
      '- Use Day2 as the strongest planning benchmark for expected scale and pricing.',
      '',
      '<!-- CHART_SPEC:v1 -->',
      '```json{',
      '"version":"1.0",',
      '"title":"3-Day Deal Forecast Inventory",',
      '"chart":{"type":"bar","x":{"key":"day"},"series":{"key":"series"},"y":[{"key":"inventory","label":"Inventory"}]},',
      '"data":[{"day":"Day2","series":"forecast","inventory":1236852296}],',
      '"meta":{"source":"steward-forecasting"}',
      '}',
      '```'
    ].join('\n');

    const html = renderToStaticMarkup(
      React.createElement(RichContent, { content })
    );

    expect(html).toContain('Deal: 142479');
    expect(html).toContain('Best available day: Day 2');
    expect(html).toContain('3-day total inventory: 1,236,852,296');
    expect(html).toContain('Uniques are available only for Day 2.');
    expect(html).toContain('across the 3-day view');
    expect(html).toContain('app-rich-chart');
  });

  it('renders mixed prose, table, mermaid, and chart content in sequence', () => {
    const html = renderToStaticMarkup(
      React.createElement(RichContent, {
        content: [
          'Intro paragraph.',
          '',
          '| Name | Value |',
          '| --- | --- |',
          '| A | 1 |',
          '',
          '```mermaid',
          'flowchart TD',
          'A[Start] --> B[Done]',
          '```',
          '',
          '```json',
          '{"chart":{"type":"bar","x":{"key":"name"},"y":[{"key":"value"}]},"data":[{"name":"A","value":1}]}',
          '```',
          '',
          'Tail paragraph.',
        ].join('\n')
      })
    );

    expect(html).toContain('Intro paragraph.');
    expect(html).toContain('bp6-table-container');
    expect(html).toContain('app-rich-mermaid');
    expect(html).toContain('app-rich-chart');
    expect(html).toContain('Tail paragraph.');
  });

  it('renders donut chart specs without falling back', () => {
    const html = renderToStaticMarkup(
      React.createElement(RichContent, {
        content: [
          '```json',
          '{"chart":{"type":"donut","x":{"key":"name"},"y":[{"key":"value"}]},"data":[{"name":"A","value":1},{"name":"B","value":2}]}',
          '```'
        ].join('\n')
      })
    );

    expect(html).toContain('app-rich-chart');
  });

  it('renders a forge-ui planner table bound to forge-data', () => {
    const content = [
      '```forge-data',
      JSON.stringify({
        version: 1,
        id: 'recommended_sites',
        format: 'json',
        mode: 'replace',
        data: [
          { site_id: 101, site_name: 'example.com', reason: 'Strong overlap', selected: true },
          { site_id: 202, site_name: 'publisher.net', reason: 'Historical CTR', selected: true },
        ],
      }, null, 2),
      '```',
      '',
      '```forge-ui',
      JSON.stringify({
        version: 1,
        title: 'Recommended sites',
        subtitle: 'Review recommendations before submitting',
        blocks: [
          {
            id: 'site-review',
            kind: 'planner.table',
            title: 'Site review',
            dataSourceRef: 'recommended_sites',
            selection: { mode: 'checkbox', field: 'selected' },
            columns: [
              { key: 'site_id', label: 'Site ID' },
              { key: 'site_name', label: 'Site name' },
              { key: 'reason', label: 'Why recommended' },
            ],
            actions: [
              {
                id: 'submit-sites',
                kind: 'submit',
                label: 'Submit changes',
                callback: { type: 'llm_event', eventName: 'planner_table_submit' },
              },
            ],
          },
        ],
      }, null, 2),
      '```',
    ].join('\n');

    const html = renderToStaticMarkup(
      React.createElement(RichContent, { content })
    );

    expect(html).toContain('Recommended sites');
    expect(html).toContain('Site review');
    expect(html).toContain('example.com');
    expect(html).toContain('publisher.net');
    expect(html).toContain('Submit changes');
  });

  it('keeps planner submit enabled when rows are initially selected', () => {
    const content = [
      '```forge-data',
      JSON.stringify({
        version: 1,
        id: 'recommended_sites',
        format: 'json',
        mode: 'replace',
        data: [
          { site_id: 101, site_name: 'example.com', reason: 'Strong overlap', selected: true },
        ],
      }, null, 2),
      '```',
      '',
      '```forge-ui',
      JSON.stringify({
        version: 1,
        title: 'Recommended sites',
        blocks: [
          {
            id: 'site-review',
            kind: 'planner.table',
            title: 'Site review',
            dataSourceRef: 'recommended_sites',
            selection: { mode: 'checkbox', field: 'selected' },
            columns: [
              { key: 'site_id', label: 'Site ID' },
              { key: 'site_name', label: 'Site name' },
              { key: 'reason', label: 'Why recommended' },
            ],
            actions: [
              {
                id: 'submit-sites',
                kind: 'submit',
                label: 'Submit changes',
                callback: { type: 'llm_event', eventName: 'planner_table_submit' },
              },
            ],
          },
        ],
      }, null, 2),
      '```',
    ].join('\n');

    const html = renderToStaticMarkup(
      React.createElement(RichContent, { content })
    );

    expect(html).toContain('data-forge-submit="site-review"');
    expect(html).not.toContain('data-forge-submit="site-review" disabled=""');
  });

  it('keeps planner submit enabled when no rows are selected', () => {
    const content = [
      '```forge-data',
      JSON.stringify({
        version: 1,
        id: 'recommended_sites',
        format: 'json',
        mode: 'replace',
        data: [
          { site_id: 101, site_name: 'example.com', reason: 'Strong overlap', selected: false },
        ],
      }, null, 2),
      '```',
      '',
      '```forge-ui',
      JSON.stringify({
        version: 1,
        title: 'Recommended sites',
        blocks: [
          {
            id: 'site-review',
            kind: 'planner.table',
            title: 'Site review',
            dataSourceRef: 'recommended_sites',
            selection: { mode: 'checkbox', field: 'selected' },
            columns: [
              { key: 'site_id', label: 'Site ID' },
              { key: 'site_name', label: 'Site name' },
              { key: 'reason', label: 'Why recommended' },
            ],
            actions: [
              {
                id: 'submit-sites',
                kind: 'submit',
                label: 'Submit changes',
                callback: { type: 'llm_event', eventName: 'planner_table_submit' },
              },
            ],
          },
        ],
      }, null, 2),
      '```',
    ].join('\n');

    const html = renderToStaticMarkup(
      React.createElement(RichContent, { content })
    );

    expect(html).toContain('data-forge-submit="site-review"');
    expect(html).not.toContain('data-forge-submit="site-review" disabled=""');
  });

  it('renders forge-ui dashboard blocks from forge-data references', () => {
    const content = [
      '```forge-data',
      JSON.stringify({
        version: 1,
        id: 'summary_metrics',
        format: 'json',
        mode: 'replace',
        data: [
          { spend: 1316.86, pacing_ratio: 0.17, win_rate: 4.02 },
        ],
      }, null, 2),
      '```',
      '',
      '```forge-ui',
      JSON.stringify({
        version: 1,
        title: 'Ad order 2639076 — Parent or Grandparenting',
        subtitle: 'Agency 4257 — Viant Fixed Managed US',
        blocks: [
          {
            id: 'summary',
            kind: 'dashboard.summary',
            dataSourceRef: 'summary_metrics',
            metrics: ['spend', 'pacing_ratio', 'win_rate'],
          },
        ],
      }, null, 2),
      '```',
    ].join('\n');

    const html = renderToStaticMarkup(
      React.createElement(RichContent, { content })
    );

    expect(html).toContain('Ad order 2639076');
    expect(html).toContain('Agency 4257');
    expect(html).toContain('SPEND');
    expect(html).toContain('PACING_RATIO');
    expect(html).toContain('WIN_RATE');
  });

  it('renders multiple forge-ui dashboards in one message', () => {
    const content = [
      '```forge-data',
      JSON.stringify({
        version: 1,
        id: 'baseline_summary',
        format: 'json',
        mode: 'replace',
        data: [
          { spend: 6887, budget: 6912, pacing_ratio: 0.9965 },
        ],
      }, null, 2),
      '```',
      '',
      '```forge-ui',
      JSON.stringify({
        version: 1,
        scope: 'baseline',
        title: 'Preliminary findings',
        subtitle: 'Initial delivery posture',
        blocks: [
          {
            id: 'summary',
            kind: 'dashboard.summary',
            dataSourceRef: 'baseline_summary',
            metrics: [
              { key: 'spend', label: 'Spend' },
              { key: 'budget', label: 'Budget' },
              { key: 'pacing_ratio', label: 'Pacing', format: 'percentFraction' },
            ],
          },
        ],
      }, null, 2),
      '```',
      '',
      '```forge-data',
      JSON.stringify({
        version: 1,
        id: 'followup_findings',
        format: 'json',
        mode: 'replace',
        data: [
          {
            title: 'Why delivery softened',
            narrative: 'PMP deal gating stayed dominant while supply narrowed day over day, so the deeper follow-up focuses on the highest-pressure deal and geo filters.',
          },
        ],
      }, null, 2),
      '```',
      '',
      '```forge-ui',
      JSON.stringify({
        version: 1,
        scope: 'followup',
        title: 'Deep follow-up',
        subtitle: 'What to validate next',
        blocks: [
          {
            id: 'narrative',
            kind: 'dashboard.report',
            title: 'Interpretation',
            sections: [
              {
                id: 'followup-rationale',
                title: 'Narrative',
                body: [
                  'PMP deal gating stayed dominant while supply narrowed day over day, so the deeper follow-up focuses on the highest-pressure deal and geo filters.',
                ],
              },
            ],
          },
        ],
      }, null, 2),
      '```',
    ].join('\n');

    const html = renderToStaticMarkup(
      React.createElement(RichContent, { content, messageId: 'msg-multi-dashboard' })
    );

    expect(html).toContain('Preliminary findings');
    expect(html).toContain('Deep follow-up');
    expect(html).toContain('Initial delivery posture');
    expect(html).toContain('What to validate next');
    expect(html).toContain('PMP deal gating stayed dominant');
  });

  it('renders dashboard report sections when body is a string', () => {
    const content = [
      '```forge-ui',
      JSON.stringify({
        version: 1,
        title: 'Forecast review',
        blocks: [
          {
            id: 'interpretation',
            kind: 'dashboard.report',
            title: 'Interpretation',
            sections: [
              {
                title: 'Pacing interpretation',
                body: 'A true pacing interpretation requires completed forecast reads before grouped cuts can be trusted.',
              },
            ],
          },
        ],
      }, null, 2),
      '```',
    ].join('\n');

    const html = renderToStaticMarkup(
      React.createElement(RichContent, { content, messageId: 'msg-report-string-body' })
    );

    expect(html).toContain('Forecast review');
    expect(html).toContain('Interpretation');
    expect(html).toContain('Pacing interpretation');
    expect(html).toContain('completed forecast reads before grouped cuts can be trusted');
    expect(html).not.toContain('Failed to render dashboard block');
  });

  it('renders dashboard messages from datasource field mappings', () => {
    const content = [
      '```forge-data',
      JSON.stringify({
        version: 1,
        id: 'forecast_findings',
        format: 'json',
        mode: 'replace',
        data: [
          {
            severity: 'high',
            finding: 'The active audience profile is concentrated in one PMP deal.',
            recommendation: 'Validate the deal and test a bounded inventory expansion.',
          },
        ],
      }, null, 2),
      '```',
      '',
      '```forge-ui',
      JSON.stringify({
        version: 1,
        title: 'Forecast review',
        blocks: [
          {
            kind: 'dashboard.messages',
            title: 'Next actions',
            dataSourceRef: 'forecast_findings',
            items: [
              { title: 'Primary restriction', field: 'finding', severity: 'info' },
              { title: 'Recommended next step', field: 'recommendation', severity: 'info' },
            ],
          },
        ],
      }, null, 2),
      '```',
    ].join('\n');

    const html = renderToStaticMarkup(
      React.createElement(RichContent, { content, messageId: 'msg-messages-field-map' })
    );

    expect(html).toContain('Primary restriction');
    expect(html).toContain('The active audience profile is concentrated in one PMP deal');
    expect(html).toContain('Recommended next step');
    expect(html).toContain('Validate the deal and test a bounded inventory expansion');
  });

  it('renders dashboard tables when columns use field mappings', () => {
    const content = [
      '```forge-data',
      JSON.stringify({
        version: 1,
        id: 'segment_competition_audiences',
        format: 'json',
        mode: 'replace',
        data: [
          {
            audience_name: 'CTV_BAU_US_Personas - OM',
            channel_name: 'CTV',
            classification: 'GLOBALLY_RARE_AND_LOCALLY_RESTRICTIVE',
            action: 'REVIEW',
            confidence: 0.93,
            rationale: 'Active taxonomy stack is globally scarce and locally restrictive.',
          },
        ],
      }, null, 2),
      '```',
      '',
      '```forge-ui',
      JSON.stringify({
        version: 1,
        title: 'Segment Competition Analysis',
        blocks: [
          {
            kind: 'dashboard.table',
            title: 'Primary segment competition findings',
            dataSourceRef: 'segment_competition_audiences',
            columns: [
              { field: 'audience_name', label: 'Audience' },
              { field: 'channel_name', label: 'Channel' },
              { field: 'classification', label: 'Classification' },
              { field: 'action', label: 'Action' },
              { field: 'confidence', label: 'Confidence', format: 'percentFraction' },
              { field: 'rationale', label: 'Why it matters' },
            ],
          },
        ],
      }, null, 2),
      '```',
    ].join('\n');

    const html = renderToStaticMarkup(
      React.createElement(RichContent, { content, messageId: 'msg-table-field-map' })
    );

    expect(html).toContain('Primary segment competition findings');
    expect(html).toContain('Audience');
    expect(html).toContain('CTV_BAU_US_Personas - OM');
    expect(html).toContain('GLOBALLY_RARE_AND_LOCALLY_RESTRICTIVE');
    expect(html).toContain('93.0%');
    expect(html).toContain('Active taxonomy stack is globally scarce and locally restrictive');
  });

  it('defaults forge dashboard scope to message identity and namespaces datasource refs', () => {
    const { scopeKey, dashboardKey, scopedDataBlocks, scopedPayload } = scopeForgeDashboardPayload(
      {
        version: 1,
        title: 'Shared Title',
        blocks: [
          { id: 'summary', kind: 'dashboard.summary', dataSourceRef: 'baseline' },
        ],
      },
      [
        { version: 1, id: 'baseline', format: 'json', mode: 'replace', data: [] },
      ],
      'msg-123'
    );

    expect(scopeKey).toBe('msg:msg-123');
    expect(dashboardKey).toBe('forge-ui:msg:msg-123');
    expect(scopedDataBlocks[0].id).toBe('forge-ui:msg:msg-123:ds:baseline');
    expect(scopedPayload.blocks[0].dataSourceRef).toBe('forge-ui:msg:msg-123:ds:baseline');
  });

  it('uses explicit shared scope when provided', () => {
    const { scopeKey, dashboardKey, scopedDataBlocks, scopedPayload } = scopeForgeDashboardPayload(
      {
        version: 1,
        title: 'Shared Title',
        scope: 'blocker-baseline',
        blocks: [
          { id: 'summary', kind: 'dashboard.summary', dataSourceRef: 'baseline' },
        ],
      },
      [
        { version: 1, id: 'baseline', format: 'json', mode: 'replace', data: [] },
      ],
      'msg-123'
    );

    expect(resolveForgeScope({ scope: 'blocker-baseline' }, 'msg-123')).toBe('scope:blocker-baseline');
    expect(scopeKey).toBe('scope:blocker-baseline');
    expect(dashboardKey).toBe('forge-ui:scope:blocker-baseline');
    expect(scopedDataBlocks[0].id).toBe('forge-ui:scope:blocker-baseline:ds:baseline');
    expect(scopedPayload.blocks[0].dataSourceRef).toBe('forge-ui:scope:blocker-baseline:ds:baseline');
  });

  it('preserves badges, summary, timeline, and table blocks in a dashboard payload', () => {
    const normalized = normalizeDashboardPayload({
      type: 'forge_dashboard',
      title: 'Campaign Health',
      subtitle: 'Summary header',
      dataSources: [
        {
          name: 'daily_rows',
          csv: [
            'date,spend,clicks,status',
            '2026-04-08,134.3,12,healthy',
            '2026-04-09,256.42,19,behind',
          ].join('\n'),
        },
      ],
      blocks: [
        {
          id: 'badges',
          kind: 'dashboard.badges',
          items: [
            { label: 'Status', value: 'Healthy', tone: 'success' },
            { label: 'Pacing', value: 'Behind', tone: 'warning' },
          ],
        },
        {
          id: 'summary',
          kind: 'dashboard.summary',
          items: [
            { label: 'Spend', value: 390.72 },
            { label: 'Clicks', value: 31 },
          ],
        },
        {
          id: 'timeline',
          kind: 'dashboard.timeline',
          title: 'Daily spend',
          dataSource: 'daily_rows',
          dateField: 'date',
          series: ['spend'],
          chartType: 'bar',
        },
        {
          id: 'table',
          kind: 'dashboard.table',
          title: 'Daily detail',
          dataSourceRef: 'daily_rows',
          columns: [
            { key: 'date', label: 'Date' },
            { key: 'spend', label: 'Spend' },
            { key: 'clicks', label: 'Clicks' },
            { key: 'status', label: 'Status' },
          ],
        },
      ],
    });

    expect(normalized.title).toBe('Campaign Health');
    expect(normalized.subtitle).toBe('Summary header');
    expect(normalized.blocks.map((block) => block.kind)).toEqual([
      'dashboard.badges',
      'dashboard.summary',
      'dashboard.timeline',
      'dashboard.table',
    ]);
    expect(normalized.blocks[2].chart.type).toBe('bar');
    expect(normalized.blocks[3].columns).toHaveLength(4);
  });

  it('normalizes dashboard summary metrics and table columns that use field keys', () => {
    const normalized = normalizeDashboardPayload({
      type: 'forge_dashboard',
      title: 'Delivery posture',
      dataSources: [
        {
          id: 'delivery_summary',
          collection: [
            {
              td_spend: 6887.2879,
              td_budget: 6911.5626,
              td_pacing_rate: 0.9965,
              td_imps: 337647,
            },
          ],
        },
      ],
      blocks: [
        {
          id: 'summary',
          kind: 'dashboard.summary',
          dataSourceRef: 'delivery_summary',
          metrics: [
            { field: 'td_spend', label: 'Spend', format: 'currency' },
            { field: 'td_budget', label: 'Budget', format: 'currency' },
            { field: 'td_pacing_rate', label: 'Pacing rate', format: 'percent' },
            { field: 'td_imps', label: 'Impressions', format: 'number' },
          ],
        },
        {
          id: 'table',
          kind: 'dashboard.table',
          dataSourceRef: 'delivery_summary',
          columns: [
            { field: 'td_spend', label: 'Spend' },
            { field: 'td_budget', label: 'Budget' },
          ],
        },
      ],
    });

    expect(normalized.metrics.block_0.td_spend).toBe(6887.2879);
    expect(normalized.metrics.block_0.td_budget).toBe(6911.5626);
    expect(normalized.blocks[0].metrics).toEqual([
      expect.objectContaining({ id: 'td_spend', label: 'Spend', selector: 'block_0.td_spend' }),
      expect.objectContaining({ id: 'td_budget', label: 'Budget', selector: 'block_0.td_budget' }),
      expect.objectContaining({ id: 'td_pacing_rate', label: 'Pacing rate', selector: 'block_0.td_pacing_rate' }),
      expect.objectContaining({ id: 'td_imps', label: 'Impressions', selector: 'block_0.td_imps' }),
    ]);
  });

  it('preserves explicit formats on dashboard summary items', () => {
    const normalized = normalizeDashboardPayload({
      type: 'forge_dashboard',
      title: 'Delivery posture',
      blocks: [
        {
          id: 'summary',
          kind: 'dashboard.summary',
          items: [
            { label: 'Pacing rate', value: 0.9965, format: 'percentFraction' },
            { label: 'Impression win rate', value: 0.4526, format: 'percentFraction' },
          ],
        },
      ],
    });

    expect(normalized.metrics.block_0['Pacing rate']).toBe(0.9965);
    expect(normalized.blocks[0].metrics).toEqual([
      expect.objectContaining({ id: 'Pacing rate', format: 'percentFraction', selector: 'block_0.Pacing rate' }),
      expect.objectContaining({ id: 'Impression win rate', format: 'percentFraction', selector: 'block_0.Impression win rate' }),
    ]);
  });

  it('normalizes dashboard summary items that read values from valueField on the datasource row', () => {
    const normalized = normalizeDashboardPayload({
      type: 'forge_dashboard',
      title: 'Inventory constraints',
      dataSources: [
        {
          id: 'order2661308_summary',
          collection: [
            {
              ad_order_name: 'ThomasJHenryLaw_CTVPilot_2026_CTV_Video_Dallas_General',
              baseline_spend: 24082.989,
              daily_budget: 25000,
              pacing_rate: 0.9633,
              bid_to_imp_win_rate: 0.1748,
              primary_blocker: 'Single-deal supply constraint',
            },
          ],
        },
      ],
      blocks: [
        {
          kind: 'dashboard.summary',
          title: 'Delivery posture and blocker ranking',
          dataSourceRef: 'order2661308_summary',
          items: [
            { label: 'Ad order', valueField: 'ad_order_name' },
            { label: 'Baseline spend', valueField: 'baseline_spend', format: 'currency' },
            { label: 'Daily budget', valueField: 'daily_budget', format: 'currency' },
            { label: 'Pacing rate', valueField: 'pacing_rate', format: 'percentFraction' },
            { label: 'Bid→imp win rate', valueField: 'bid_to_imp_win_rate', format: 'percentFraction' },
            { label: 'Primary blocker', valueField: 'primary_blocker' },
          ],
        },
      ],
    });

    expect(normalized.metrics.block_0['Ad order']).toBe('ThomasJHenryLaw_CTVPilot_2026_CTV_Video_Dallas_General');
    expect(normalized.metrics.block_0['Baseline spend']).toBe(24082.989);
    expect(normalized.metrics.block_0['Daily budget']).toBe(25000);
    expect(normalized.metrics.block_0['Pacing rate']).toBe(0.9633);
    expect(normalized.metrics.block_0['Bid→imp win rate']).toBe(0.1748);
    expect(normalized.metrics.block_0['Primary blocker']).toBe('Single-deal supply constraint');
  });

  it('normalizes dashboard dimensions blocks that use field-based dimension and metrics', () => {
    const normalized = normalizeDashboardPayload({
      type: 'forge_dashboard',
      title: 'Restrictive signals',
      dataSources: [
        {
          id: 'restrictive_signals',
          collection: [
            { feature: 'external.pmp.deal', reject_share: 0.1755, reject_count: 163633 },
            { feature: 'ad.pmp.deal.id', reject_share: 0.135, reject_count: 125831 },
          ],
        },
      ],
      blocks: [
        {
          id: 'restrictive',
          kind: 'dashboard.dimensions',
          dataSourceRef: 'restrictive_signals',
          dimension: { field: 'feature', label: 'Feature' },
          metrics: [
            { field: 'reject_share', label: 'Reject share', format: 'percent' },
            { field: 'reject_count', label: 'Reject count', format: 'number' },
          ],
        },
      ],
    });

    expect(normalized.blocks[0].dimension).toEqual(
      expect.objectContaining({ key: 'feature', field: 'feature', label: 'Feature' }),
    );
    expect(normalized.blocks[0].metric).toEqual(
      expect.objectContaining({ key: 'reject_share', label: 'Reject share', format: 'percent' }),
    );
    expect(normalized.blocks[0].metrics).toEqual([
      expect.objectContaining({ key: 'reject_share', label: 'Reject share', format: 'percent' }),
      expect.objectContaining({ key: 'reject_count', label: 'Reject count', format: 'number' }),
    ]);
  });

  it('renders dashboard dimensions rows with varied palette colors', () => {
    const content = [
      '```forge-data',
      JSON.stringify({
        version: 1,
        id: 'restrictive_signals',
        format: 'json',
        mode: 'replace',
        data: [
          { feature: 'external.pmp.deal', reject_share: 0.176 },
          { feature: 'ad.pmp.deal.id', reject_share: 0.135 },
          { feature: 'location', reject_share: 0.101 },
        ],
      }, null, 2),
      '```',
      '',
      '```forge-ui',
      JSON.stringify({
        version: 1,
        title: 'Restricting factors',
        blocks: [
          {
            id: 'restrictive',
            kind: 'dashboard.dimensions',
            dataSourceRef: 'restrictive_signals',
            dimension: { field: 'feature', label: 'Feature' },
            metric: { key: 'reject_share', label: 'Reject share', format: 'percentFraction' },
          },
        ],
      }, null, 2),
      '```',
    ].join('\n');

    const html = renderToStaticMarkup(
      React.createElement(RichContent, { content, messageId: 'msg-dimensions-palette' })
    );

    expect(html).toContain('#2f6de1');
    expect(html).toContain('#7a46d8');
    expect(html).toContain('#db2f7d');
  });

  it('renders compact streamed forge planner fences without extra normalization', () => {
    const content = [
      '```forge-data{"version":1,"id":"recommended_sites","format":"json","mode":"replace","data":[{"site_id":102788,"site_name":"Site List 102788","reason":"Best supplemental expansion test.","selected":true},{"site_id":22547,"site_name":"Site List 22547","reason":"Best backup option.","selected":true}]}',
      '```',
      '',
      '```forge-ui{"version":1,"title":"Review recommended site lists","subtitle":"Suggested target-list expansions for audience 7180287.","blocks":[{"id":"site-review","kind":"planner.table","title":"Recommended sites","dataSourceRef":"recommended_sites","selection":{"mode":"checkbox","field":"selected"},"columns":[{"key":"site_id","label":"Site ID"},{"key":"site_name","label":"Site name"},{"key":"reason","label":"Why recommended"}],"actions":[{"id":"submit-sites","kind":"submit","label":"Submit changes","callback":{"type":"custom_callback","eventName":"planner_table_submit","target":"foreground"}}]}]}',
      '```',
    ].join('\n');

    const html = renderToStaticMarkup(
      React.createElement(RichContent, { content })
    );

    expect(html).toContain('Review recommended site lists');
    expect(html).toContain('Site List 102788');
    expect(html).toContain('Site List 22547');
    expect(html).toContain('Submit changes');
  });

  it('renders a loading placeholder for an unterminated trailing forge-ui fence', () => {
    const content = [
      '```forge-data',
      '{"version":1,"id":"sales_data","format":"json","mode":"replace","data":[]}',
      '```',
      '',
      '```forge-ui',
      '{"version":1,"title":"Sales Dashboard","blocks":['
    ].join('\n');

    const html = renderToStaticMarkup(
      React.createElement(RichContent, { content })
    );

    expect(html).toContain('Building UI');
    expect(html).not.toContain('```forge-ui');
    expect(html).not.toContain('Invalid forge-ui block');
  });

  it('renders a loading placeholder for an unterminated trailing forge-data fence', () => {
    const content = [
      '```forge-data',
      '{"version":1,"id":"sales_data","format":"json","mode":"replace","data":['
    ].join('\n');

    const html = renderToStaticMarkup(
      React.createElement(RichContent, { content })
    );

    expect(html).toContain('Setting datasources');
    expect(html).not.toContain('```forge-data');
  });

  it('normalizes a single-series dashboard timeline into long-form chart rows', () => {
    const normalized = normalizeDashboardPayload({
      type: 'forge_dashboard',
      title: 'Chart test',
      dataSources: [
        {
          name: 'daily_delivery',
          csv: [
            'date,spend',
            '2026-04-08,134.30',
            '2026-04-09,256.42',
          ].join('\n'),
        },
      ],
      blocks: [
        {
          kind: 'dashboard.timeline',
          title: 'Daily spend',
          dataSource: 'daily_delivery',
          dateField: 'date',
          series: ['spend'],
          chartType: 'bar',
        },
      ],
    });

    const timeline = normalized.blocks[0];
    expect(timeline.__collection).toEqual([
      { date: expect.any(Date), series: 'Spend', value: 134.3 },
      { date: expect.any(Date), series: 'Spend', value: 256.42 },
    ]);
    expect(timeline.chart.series).toMatchObject({
      nameKey: 'series',
      valueKey: 'value',
      values: [{ label: 'Spend', name: 'Spend', value: 'spend' }],
    });
  });

  it('treats dashboard.timeline metrics as a backward-compatible alias for series', () => {
    const normalized = normalizeDashboardPayload({
      type: 'forge_dashboard',
      title: 'Chart test',
      dataSources: [
        {
          id: 'weekly_daily_trend',
          collection: [
            { event_date: '2026-05-13', total_spend: 4.4502, clicks: 5, conversions: 2 },
            { event_date: '2026-05-14', total_spend: 5.8362, clicks: 18, conversions: 6 },
          ],
        },
      ],
      blocks: [
        {
          kind: 'dashboard.timeline',
          title: 'Daily delivery trend',
          dataSourceRef: 'weekly_daily_trend',
          dateField: 'event_date',
          metrics: [
            { key: 'total_spend', label: 'Spend', format: 'currency' },
            { key: 'clicks', label: 'Clicks', format: 'number' },
            { key: 'conversions', label: 'Conversions', format: 'number' },
          ],
        },
      ],
    });

    const timeline = normalized.blocks[0];
    expect(timeline.__collection).toEqual([
      { event_date: '2026-05-13', series: 'Total Spend', value: 4.4502 },
      { event_date: '2026-05-13', series: 'Clicks', value: 5 },
      { event_date: '2026-05-13', series: 'Conversions', value: 2 },
      { event_date: '2026-05-14', series: 'Total Spend', value: 5.8362 },
      { event_date: '2026-05-14', series: 'Clicks', value: 18 },
      { event_date: '2026-05-14', series: 'Conversions', value: 6 },
    ]);
    expect(timeline.chart.series).toMatchObject({
      nameKey: 'series',
      valueKey: 'value',
      values: [
        { label: 'Total Spend', name: 'Total Spend', value: 'total_spend' },
        { label: 'Clicks', name: 'Clicks', value: 'clicks' },
        { label: 'Conversions', name: 'Conversions', value: 'conversions' },
      ],
    });
  });

  it('normalizes a dashboard timeline when the payload uses timeKey alias', () => {
    const normalized = normalizeDashboardPayload({
      type: 'forge_dashboard',
      title: 'Chart test',
      dataSources: [
        {
          id: 'delivery_trend',
          collection: [
            { event_date: '2026-05-05', total_spend: 1550.659, bids: 1935643, impressions: 53471 },
            { event_date: '2026-05-06', total_spend: 1674.373, bids: 1443330, impressions: 57737 },
          ],
        },
      ],
      blocks: [
        {
          kind: 'dashboard.timeline',
          title: 'Recent delivery trend',
          dataSourceRef: 'delivery_trend',
          timeKey: 'event_date',
          series: [
            { key: 'total_spend', label: 'Spend' },
            { key: 'bids', label: 'Bids' },
            { key: 'impressions', label: 'Impressions' },
          ],
        },
      ],
    });

    const timeline = normalized.blocks[0];
    expect(timeline.chart.xAxis.dataKey).toBe('event_date');
    expect(timeline.__collection).toEqual([
      { event_date: '2026-05-05', series: 'Total Spend', value: 1550.659 },
      { event_date: '2026-05-05', series: 'Bids', value: 1935643 },
      { event_date: '2026-05-05', series: 'Impressions', value: 53471 },
      { event_date: '2026-05-06', series: 'Total Spend', value: 1674.373 },
      { event_date: '2026-05-06', series: 'Bids', value: 1443330 },
      { event_date: '2026-05-06', series: 'Impressions', value: 57737 },
    ]);
  });

  it('normalizes a dashboard kpiTable from datasource rows when columns are omitted', () => {
    const normalized = normalizeDashboardPayload({
      type: 'forge_dashboard',
      title: 'Recommendation review',
      dataSources: [
        {
          id: 'summary_metrics',
          collection: [
            { label: 'Primary blocker', value: 'Supply / competitiveness' },
            { label: 'Setup state', value: 'Live and setup-ready' },
            { label: 'Observed symptom', value: 'Behind pace' },
          ],
        },
      ],
      blocks: [
        {
          kind: 'dashboard.kpiTable',
          title: 'Key metrics',
          dataSourceRef: 'summary_metrics',
        },
      ],
    });

    const table = normalized.blocks[0];
    expect(table.columns).toEqual(['label', 'value']);
    expect(table.rows).toEqual([
      ['Primary blocker', 'Supply / competitiveness'],
      ['Setup state', 'Live and setup-ready'],
      ['Observed symptom', 'Behind pace'],
    ]);
  });

  it('normalizes a long-form split timeline into chart rows', () => {
    const normalized = normalizeDashboardPayload({
      type: 'forge_dashboard',
      title: 'Forecast',
      dataSources: [
        {
          id: 'forecast_daily',
          collection: [
            { date: '2026-04-29', split: 'overall', avails: 100 },
            { date: '2026-04-29', split: 'inventory', avails: 70 },
            { date: '2026-04-29', split: 'eligibility', avails: 500 },
            { date: '2026-04-30', split: 'overall', avails: 90 },
            { date: '2026-04-30', split: 'inventory', avails: 60 },
            { date: '2026-04-30', split: 'eligibility', avails: 450 },
          ],
        },
      ],
      blocks: [
        {
          kind: 'dashboard.timeline',
          title: '3-day avails by split',
          dataSourceRef: 'forecast_daily',
          dateField: 'date',
          series: ['overall', 'inventory', 'eligibility'],
          chartType: 'line',
        },
      ],
    });

    const timeline = normalized.blocks[0];
    expect(timeline.__collection).toEqual([
      { date: '2026-04-29', series: 'Overall', value: 100 },
      { date: '2026-04-29', series: 'Inventory', value: 70 },
      { date: '2026-04-29', series: 'Eligibility', value: 500 },
      { date: '2026-04-30', series: 'Overall', value: 90 },
      { date: '2026-04-30', series: 'Inventory', value: 60 },
      { date: '2026-04-30', series: 'Eligibility', value: 450 },
    ]);
    expect(timeline.chart.series).toMatchObject({
      nameKey: 'series',
      valueKey: 'value',
    });
  });

  it('normalizes a dashboard timeline with chart.xField plus chart.series field entries', () => {
    const normalized = normalizeDashboardPayload({
      type: 'forge_dashboard',
      title: 'Risk',
      dataSources: [
        {
          id: 'delivery_timeline',
          csv: [
            'event_date,total_spend,daily_spend_shortfall,flight_spend_shortfall',
            '2026-05-01,59.7723,5085.7378,2965.7805',
            '2026-05-02,62.5744,4525.8595,5662.2959',
          ].join('\n'),
        },
      ],
      blocks: [
        {
          kind: 'dashboard.timeline',
          title: 'Daily delivery vs shortfall',
          dataSourceRef: 'delivery_timeline',
          chart: {
            xField: 'event_date',
            series: [
              { field: 'total_spend', label: 'Spend', format: 'currency' },
              { field: 'daily_spend_shortfall', label: 'Daily spend shortfall', format: 'currency' },
              { field: 'flight_spend_shortfall', label: 'Cumulative flight shortfall', format: 'currency' },
            ],
          },
        },
      ],
    });

    const timeline = normalized.blocks[0];
    expect(timeline.__collection).toEqual([
      { event_date: expect.any(Date), series: 'Spend', value: 59.7723 },
      { event_date: expect.any(Date), series: 'Daily spend shortfall', value: 5085.7378 },
      { event_date: expect.any(Date), series: 'Cumulative flight shortfall', value: 2965.7805 },
      { event_date: expect.any(Date), series: 'Spend', value: 62.5744 },
      { event_date: expect.any(Date), series: 'Daily spend shortfall', value: 4525.8595 },
      { event_date: expect.any(Date), series: 'Cumulative flight shortfall', value: 5662.2959 },
    ]);
    expect(timeline.chart).toMatchObject({
      xAxis: { dataKey: 'event_date' },
      series: {
        nameKey: 'series',
        valueKey: 'value',
      },
    });
  });

  it('normalizes a categorical dashboard timeline with chart.xField/yField/seriesField', () => {
    const normalized = normalizeDashboardPayload({
      type: 'forge_dashboard',
      title: 'Restriction pressure',
      dataSources: [
        {
          id: 'restrictive_signals',
          collection: [
            { feature: 'external.pmp.deal', restricted_pct: 0.3159 },
            { feature: 'ad.pmp.deal.id', restricted_pct: 0.2398 },
            { feature: 'channelV2', restricted_pct: 0.1395 },
          ],
        },
      ],
      blocks: [
        {
          kind: 'dashboard.timeline',
          title: 'Restriction pressure by blocker family',
          dataSourceRef: 'restrictive_signals',
          chart: {
            xField: 'feature',
            yField: 'restricted_pct',
            seriesField: 'feature',
            kind: 'bar',
          },
        },
      ],
    });

    const timeline = normalized.blocks[0];
    expect(timeline.__collection).toEqual([
      { feature: 'external.pmp.deal', series: 'External.Pmp.Deal', value: 0.3159 },
      { feature: 'ad.pmp.deal.id', series: 'Ad.Pmp.Deal.Id', value: 0.2398 },
      { feature: 'channelV2', series: 'Channel V2', value: 0.1395 },
    ]);
    expect(timeline.chart).toMatchObject({
      type: 'bar',
      xAxis: { dataKey: 'feature' },
      yAxis: { label: 'Restricted Pct' },
      series: {
        nameKey: 'series',
        valueKey: 'value',
      },
    });
  });

  it('normalizes compare items with selector paths into metric values', () => {
    const normalized = normalizeDashboardPayload({
      type: 'forge_dashboard',
      title: 'Forecast',
      dataSources: [
        {
          id: 'forecast_summary',
          collection: [
            {
              inventory_avails: 252125,
              eligibility_avails: 8181067117,
            },
          ],
        },
      ],
      blocks: [
        {
          kind: 'dashboard.compare',
          title: 'Split comparison',
          dataSourceRef: 'forecast_summary',
          items: [
            { id: 'inventoryAvails', label: 'Inventory Avails', current: '0.inventory_avails', format: 'compactNumber' },
            { id: 'eligibilityAvails', label: 'Eligibility Avails', current: '0.eligibility_avails', format: 'compactNumber' },
          ],
        },
      ],
    });

    expect(normalized.metrics.block_0).toEqual({
      inventoryAvails: { current: 252125, previous: null },
      eligibilityAvails: { current: 8181067117, previous: null },
    });
    expect(normalized.blocks[0].items).toEqual([
      expect.objectContaining({ current: 'block_0.inventoryAvails.current', previous: 'block_0.inventoryAvails.previous' }),
      expect.objectContaining({ current: 'block_0.eligibilityAvails.current', previous: 'block_0.eligibilityAvails.previous' }),
    ]);
  });

  it('normalizes summary metric objects with selector paths into metric values', () => {
    const normalized = normalizeDashboardPayload({
      type: 'forge_dashboard',
      title: 'Forecast',
      dataSources: [
        {
          id: 'forecast_summary',
          collection: [
            {
              overall_avails: 800,
              overall_uniques: 36400,
              overall_ipu: 0.02,
            },
          ],
        },
      ],
      blocks: [
        {
          kind: 'dashboard.summary',
          title: 'Forecast posture',
          dataSourceRef: 'forecast_summary',
          metrics: [
            { id: 'overallAvails', label: 'Latest-day avails', selector: '0.overall_avails', format: 'compactNumber' },
            { id: 'overallUniques', label: 'Latest-day device uniques', selector: '0.overall_uniques', format: 'compactNumber' },
            { id: 'overallIpu', label: 'Avails / unique', selector: '0.overall_ipu', format: 'number' },
          ],
        },
      ],
    });

    expect(normalized.metrics.block_0).toEqual({
      overallAvails: 800,
      overallUniques: 36400,
      overallIpu: 0.02,
    });
    expect(normalized.blocks[0].metrics).toEqual([
      expect.objectContaining({ id: 'overallAvails', selector: 'block_0.overallAvails' }),
      expect.objectContaining({ id: 'overallUniques', selector: 'block_0.overallUniques' }),
      expect.objectContaining({ id: 'overallIpu', selector: 'block_0.overallIpu' }),
    ]);
  });
});
