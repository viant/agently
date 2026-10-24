import React from 'react';
import {createRoot} from 'react-dom/client';
import {signal} from '@preact/signals-react';
import {ForgeThemeBoundary, Typography, SelectControl, typographyProps} from 'forge/components';
import Container from 'forge/components/Container.jsx';
import TypographySpecimen from 'forge/demos/theme/TypographySpecimen.jsx';
import FoundationSpecimen from 'forge/demos/theme/FoundationSpecimen.jsx';
import ChartSpecimen, {ChartExample} from 'forge/demos/theme/ChartSpecimen.jsx';
import {DashboardEditableTable} from 'forge/components/dashboard/DashboardBlocks.jsx';
import WorkspaceStyleProvider from './components/WorkspaceStyleProvider.jsx';
import WorkspaceAppearanceSettings from './components/WorkspaceAppearanceSettings.jsx';
import ChatSpecimen from './components/ChatSpecimen.jsx';
import NavigationSpecimen from './components/NavigationSpecimen.jsx';
import fixture from '/__typography/fixture.js';
import '@blueprintjs/core/lib/css/blueprint.css';
import 'forge/packs/blueprint/index.jsx';
import './styles/typography.css';
import './styles/ui-settings.css';

const values = signal(fixture.pilot?.values || {});
const context = {
  identity: {windowId: 'typography-pilot', dataSourceRef: 'identity'},
  metadata: {dataSource: {identity: {}}},
    signals: {form: values, collection: signal([]), control: signal({}), selection: signal({}), input: signal({}), windowForm: signal({}), formStatus: signal({}), message: signal([])},
  handlers: {dataSource: {getFormData: () => values.value}},
    lookupHandler: name => () => {window.__foundationActions = [...(window.__foundationActions || []), name];},
  Context() { return this; },
};

function TablePilot({table}) {
  const [collection] = React.useState(() => signal(table.rows));
  const tableContext = {...context, signals: {...context.signals, collection}, handlers: {dataSource: {
    peekFullCollection: () => collection.value,
    replaceCollection: ({rows}) => {collection.value = rows; window.__foundationTableRows = rows;},
  }}};
  tableContext.Context = () => tableContext;
  return <section data-foundation-table-pilot aria-label={table.container.title} style={{maxWidth: table.maxWidth, minWidth: 0}}>
    <Typography as="h2" typeRole="section">{table.container.title}</Typography>
    <DashboardEditableTable embedded container={{...table.container, allowAdd: false}} context={tableContext}/>
  </section>;
}

function ChartPilot({charts}) {
  const [id,setId] = React.useState(charts[0].id);
  const example = charts.find(chart=>chart.id===id);
  return <section className="forge-chart-specimen" id="foundation-report-charts" aria-label="Authored report charts">
    <Typography as="h2" typeRole="section">Report charts</Typography>
    <Typography as="p" typeRole="body">Authored report charts with sample data. Choose a chart to review its presentation.</Typography>
    <label {...typographyProps('label')}>Report chart <SelectControl {...typographyProps('body')} aria-label="Authored report chart" value={id} onChange={event=>setId(event.target.value)}>{charts.map(chart=><option key={chart.id} value={chart.id}>{chart.title}</option>)}</SelectControl></label>
    <section key={id} data-foundation-chart-pilot={id} aria-label={example.title}>
      <Typography as="h3" typeRole="label">{example.title}</Typography>
      <ChartExample example={example} onSelect={example.checkSelectionFields ? selection=>{window.__foundationChartSelection=selection;} : undefined}/>
    </section>
  </section>;
}

createRoot(document.getElementById('root')).render(<WorkspaceStyleProvider metadata={fixture.metadata} accountKey="proof" workspaceKey="proof" onRefresh={() => {window.__foundationRefresh = (window.__foundationRefresh || 0) + 1;}}>
  <main className="app-ui-settings">
    <WorkspaceAppearanceSettings/>
    <ForgeThemeBoundary windowKey="typography-proof">
      <FoundationSpecimen iconExamples={fixture.pilot?.iconExamples}/>
      <TypographySpecimen/>
      <ChartSpecimen/>
      <NavigationSpecimen/>
      <ChatSpecimen/>
      {fixture.pilot?.container && <section data-foundation-pilot data-tool-feed-id={fixture.pilot.feedId} data-forge-appearance={fixture.pilot.appearance} aria-label={fixture.pilot.label}>
        <Container container={fixture.pilot.container} context={context}/>
      </section>}
      {fixture.pilot?.tables?.map(table => <TablePilot key={table.container.id} table={table}/>)}
      {fixture.pilot?.charts?.length > 0 && <ChartPilot charts={fixture.pilot.charts}/>}
      <Typography as="p" typeRole="body">Local component verification with sample data. Actions only record a local test event.</Typography>
    </ForgeThemeBoundary>
  </main>
</WorkspaceStyleProvider>);
