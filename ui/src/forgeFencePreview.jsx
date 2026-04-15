import React from 'react';
import { createRoot } from 'react-dom/client';
import RichContent from './components/chat/RichContent.jsx';
import { connectForgeUIActionsToChat, subscribeForgeUIAction } from './services/forgeUIActions.js';

const content = [
  '```forge-data',
  JSON.stringify({
    version: 1,
    id: 'summary_metrics',
    format: 'csv',
    mode: 'replace',
    data: [
      'pipeline_spend,total_pipeline_gap,lead_to_sale_rate,lead_conversion_rate,cost_per_lead,sales_closed,days_below_goal',
      '18420.55,7420.10,0.23,0.08,61.20,94,5',
    ].join('\n'),
  }, null, 2),
  '```',
  '',
  '```forge-data',
  JSON.stringify({
    version: 1,
    id: 'daily_delivery',
    format: 'csv',
    mode: 'replace',
    data: [
      'date,spend,lead_gap,qualified_leads,sales_closed,lead_conversion_rate',
      '2026-04-08,2410.25,910.15,42,11,0.09',
      '2026-04-09,2875.40,620.00,47,13,0.10',
      '2026-04-10,2511.15,1040.65,39,12,0.08',
      '2026-04-11,2698.32,980.10,44,15,0.09',
      '2026-04-12,2220.18,1435.45,35,10,0.07',
      '2026-04-13,2384.70,1320.25,37,14,0.08',
      '2026-04-14,3320.55,1113.50,51,19,0.10',
    ].join('\n'),
  }, null, 2),
  '```',
  '',
  '```forge-data',
  JSON.stringify({
    version: 1,
    id: 'entity_table',
    format: 'csv',
    mode: 'replace',
    data: [
      'date_range,segment,spend,qualified_leads,sales_closed,lead_to_sale_rate,lead_conversion_rate,cost_per_lead,goal_attainment_pct,days_below_goal,reported_days',
      '2026-04-08 to 2026-04-14,Mid-market inbound,18420.55,295,94,0.23,0.08,61.20,0.71,5,7',
    ].join('\n'),
  }, null, 2),
  '```',
  '',
  '```forge-ui',
  JSON.stringify({
    version: 1,
    title: 'Revenue pipeline performance',
    subtitle: 'Mid-market inbound segment · 2026-04-08 to 2026-04-14',
    blocks: [
      {
        id: 'summary',
        kind: 'dashboard.summary',
        title: 'Performance summary',
        items: [
          { label: 'Pipeline spend', value: 18420.55, metricKey: 'pipeline_spend' },
          { label: 'Total pipeline gap', value: 7420.10, metricKey: 'total_pipeline_gap' },
          { label: 'Lead-to-sale rate', value: 0.23, metricKey: 'lead_to_sale_rate' },
          { label: 'Lead conversion rate', value: 0.08, metricKey: 'lead_conversion_rate' },
          { label: 'Cost per lead', value: 61.20, metricKey: 'cost_per_lead' },
          { label: 'Sales closed', value: 94, metricKey: 'sales_closed' },
        ],
      },
      {
        id: 'totals',
        kind: 'dashboard.kpiTable',
        title: 'Segment totals',
        dataSourceRef: 'entity_table',
        columns: [
          { key: 'date_range', label: 'Date range' },
          { key: 'segment', label: 'Segment' },
          { key: 'spend', label: 'Spend' },
          { key: 'qualified_leads', label: 'Qualified leads' },
          { key: 'sales_closed', label: 'Sales closed' },
          { key: 'lead_to_sale_rate', label: 'Lead-to-sale rate' },
          { key: 'lead_conversion_rate', label: 'Lead conversion rate' },
          { key: 'cost_per_lead', label: 'Cost per lead' },
          { key: 'goal_attainment_pct', label: 'Goal attainment' },
          { key: 'days_below_goal', label: 'Days below goal' },
          { key: 'reported_days', label: 'Reported days' },
        ],
      },
      {
        id: 'daily-spend',
        kind: 'dashboard.timeline',
        title: 'Daily spend',
        dataSourceRef: 'daily_delivery',
        dateField: 'date',
        series: ['spend'],
        chartType: 'bar',
      },
      {
        id: 'daily-spend-report',
        kind: 'dashboard.report',
        title: 'Daily spend highlight',
        sections: [
          {
            id: 'spend-interpretation',
            title: 'Interpretation',
            body: [
              'Spend peaked on 2026-04-14, but the segment still remained below target on most days in the seven-day window.',
              'Lead gap stayed elevated across the week, so the chart should be read as steady under-attainment rather than a one-day anomaly.',
            ],
          },
        ],
      },
      {
        id: 'messages',
        kind: 'dashboard.messages',
        items: [
          {
            severity: 'high',
            title: 'Pipeline under-attainment',
            body: 'The segment missed target on five of seven days and accumulated a meaningful lead gap despite steady spend.',
          },
        ],
      },
    ],
  }, null, 2),
  '```',
].join('\n');

function App() {
  React.useEffect(() => subscribeForgeUIAction(() => {}), []);
  React.useEffect(() => connectForgeUIActionsToChat(
    async () => {},
    () => ({ preview: true }),
  ), []);

  return (
    <div style={{ padding: 28 }}>
      <div style={{ width: 'min(1100px, 100%)', margin: '0 auto', display: 'grid', gap: 16 }}>
        <div style={{ display: 'grid', gap: 8 }}>
          <div style={{ fontSize: 12, letterSpacing: '0.12em', textTransform: 'uppercase', color: '#2d67c7', fontWeight: 700 }}>
            Agently UI Preview
          </div>
          <div style={{ fontSize: 30, lineHeight: 1.1, fontWeight: 800, color: '#182230' }}>
            `forge-ui` + multiple `forge-data` blocks for analytics dashboard
          </div>
          <div style={{ fontSize: 15, lineHeight: 1.55, color: '#6b7688', maxWidth: 840 }}>
            This preview uses the actual Agently UI <code>RichContent</code> component path and exercises one dashboard UI block backed by several named datasource blocks.
          </div>
        </div>
        <RichContent content={content} />
      </div>
    </div>
  );
}

createRoot(document.getElementById('root')).render(<App />);
