import React from 'react';
import {createRoot} from 'react-dom/client';
import {signal} from '@preact/signals-react';
import {ForgeThemeBoundary, Typography} from 'forge/components';
import WidgetRenderer from 'forge/runtime/WidgetRenderer.jsx';
import TypographySpecimen from 'forge/demos/theme/TypographySpecimen.jsx';
import FoundationSpecimen from 'forge/demos/theme/FoundationSpecimen.jsx';
import Toolbar from 'forge/components/table/basic/Toolbar.jsx';
import {DashboardEditableTable} from 'forge/components/dashboard/DashboardBlocks.jsx';
import WorkspaceStyleProvider from './components/WorkspaceStyleProvider.jsx';
import WorkspaceAppearanceSettings from './components/WorkspaceAppearanceSettings.jsx';
import fixture from '/__typography/fixture.js';
import '@blueprintjs/core/lib/css/blueprint.css';
import 'forge/packs/blueprint/index.jsx';
import './styles/typography.css';
import './styles/ui-settings.css';

const values = signal(fixture.pilot?.values || {});
const context = {
  identity: {windowId: 'typography-pilot', dataSourceRef: 'identity'},
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

createRoot(document.getElementById('root')).render(<WorkspaceStyleProvider metadata={fixture.metadata} accountKey="proof" workspaceKey="proof" onRefresh={() => {window.__foundationRefresh = (window.__foundationRefresh || 0) + 1;}}>
  <main className="app-ui-settings">
    <WorkspaceAppearanceSettings/>
    <ForgeThemeBoundary windowKey="typography-proof">
      <FoundationSpecimen iconExamples={fixture.pilot?.iconExamples}/>
      <TypographySpecimen/>
      {fixture.pilot?.container && <section data-foundation-pilot data-tool-feed-id={fixture.pilot.feedId} aria-label={fixture.pilot.label}>
        <div data-forge-container-id={fixture.pilot.container.id}><div data-forge-part="fields">
          {fixture.pilot.container.items.map(item => <WidgetRenderer key={item.id} item={item} context={context}/>)}
        </div>
        <Toolbar context={context} toolbarItems={fixture.pilot.container.toolbar?.items || []}/></div>
      </section>}
      {fixture.pilot?.tables?.map(table => <TablePilot key={table.container.id} table={table}/>)}
      <Typography as="p" typeRole="body">Local component verification with sample data. Actions only record a local test event.</Typography>
    </ForgeThemeBoundary>
  </main>
</WorkspaceStyleProvider>);
