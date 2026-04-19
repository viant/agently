import { describe, expect, it } from 'vitest';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';

import RichContent, { normalizeDashboardPayload, parseFences } from './RichContent';
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
});
