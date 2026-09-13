import React, {useState} from 'react';
import {createRoot} from 'react-dom/client';
import AppWorkspaceStyles from './components/AppWorkspaceStyles';
import WorkspaceAppearanceSettings from './components/WorkspaceAppearanceSettings';
import {getWorkspaceStyleManager} from './services/workspaceStyles';
import {ForgeThemeBoundary} from 'forge/components';
import WidgetRenderer from 'forge/runtime/WidgetRenderer.jsx';
import 'forge/packs/blueprint/index.jsx';
import './index.css';

// Deliberately session-only: the iframe must follow postMessage, not storage events.
getWorkspaceStyleManager({storage: null});
function CascadeProof() {
  const state = useState({parentSample: ''});
  const [result, setResult] = useState('Not checked');
  const addLateStyle = () => {
    const late = document.createElement('style');
    late.nonce = 'workspace-proof';
    late.textContent = '.agently-workspace[data-forge-theme="baseline"][data-forge-color-mode="dark"] {--forge-control-radius:0px}';
    document.head.appendChild(late);
    requestAnimationFrame(() => {
      const manager = getWorkspaceStyleManager();
      const input = document.querySelector('input[data-forge-control-id="parentSample"]');
      const expected = manager.state.catalog?.themes.find(theme => theme.id === manager.state.themeId)?.modes?.[manager.state.mode]?.['control.radius'];
      const passed = !!late.sheet && document.head.lastElementChild === manager.style && getComputedStyle(input).borderRadius === `${expected}px`;
      setResult(passed ? 'Workspace style wins over accepted late stylesheet' : 'Cascade check failed');
    });
  };
  const checkContainers = () => {
    const doc = document.querySelector('iframe').contentDocument;
    const ids = ['plain-case', 'card-case', 'section-case', 'collapsible-case', 'combined-case'];
    const valid = ids.every(id => doc.querySelector(`[data-forge-container-id="${id}"]`)?.classList.contains(`ws-${id}`));
    const input = doc.querySelector('input[data-forge-control-id="plainInput"]');
    setResult(valid && input?.closest('.forge-control-wrapper')?.classList.contains('ws-input') && input?.parentElement?.classList.contains('ws-input') && doc.defaultView.getComputedStyle(input).borderRadius === '9px'
      ? 'Container classes and authored inline style preserved' : 'Container contract check failed');
  };
  return <section>
    <ForgeThemeBoundary windowKey="parent-proof"><WidgetRenderer item={{id: 'parentSample', label: 'Parent sample', widget: 'text', scope: 'local'}} state={state}/></ForgeThemeBoundary>
    <button onClick={addLateStyle}>Add late stylesheet</button><button onClick={checkContainers}>Check container contracts</button><p role="status">{result}</p>
  </section>;
}
createRoot(document.getElementById('root')).render(<AppWorkspaceStyles>
  <main style={{padding: 20}}>
    <h1>Workspace theme frame proof</h1>
    <p>Preference storage is disabled in this parent page.</p>
    <WorkspaceAppearanceSettings />
    <CascadeProof />
    <iframe title="Themed Forge window" src="/mcp-ui/forge-window?windowKey=style-proof"
      sandbox="allow-scripts allow-same-origin" style={{width: '100%', height: 360, border: '1px solid #999'}} />
  </main>
</AppWorkspaceStyles>);
