// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest';
import React,{act} from 'react';
import {createRoot} from 'react-dom/client';
import { renderToStaticMarkup } from 'react-dom/server';

vi.mock('@blueprintjs/core', () => {const components={
  Button: ({ text = '', icon = '', minimal: _minimal, small: _small, ...props }) => React.createElement('button', { ...props, 'data-icon': icon }, text),
  Icon: ({ icon = '', size = 16 }) => React.createElement('span', { 'data-icon': icon, 'data-size': size }),
  Tooltip: ({ content = '', children }) => React.createElement('span', { 'data-tooltip': content }, children),
};return {...components,default:components};});
vi.mock('forge/components', async () => ({
  RetainedWindowContent: (await import('forge/components/WindowContentMount.jsx')).RetainedWindowContent,
  WindowContent: ({ window }) => {
    React.useEffect(() => { window?.onRendererReady?.(); }, [window?.windowId]);
    return React.createElement('div', { 'data-window-id': window?.windowId || '' });
  },
}));
vi.mock('./ToolFeedDetail.jsx',()=>({default:({hostedFeedId})=>React.createElement('div',{'data-feed-id':hostedFeedId})}));
vi.mock('./mcpApps/AppRenderer.jsx', () => ({
  default: ({ uri, hosted }) => React.createElement('div', { 'data-mcp-uri': uri, 'data-hosted': hosted ? 'true' : 'false' }),
}));

import ConversationWorkspaceSurface, { resolveChatWindowRenderKey, resolveWorkspaceBreadcrumbs, resolveWorkspaceNavigation, shouldShowWorkspaceTabs } from './ConversationWorkspaceSurface';

expect(resolveChatWindowRenderKey({windowId: 'chat/new'})).toBe('chat/new:0');
expect(resolveChatWindowRenderKey({windowId: 'chat/new', conversationInstanceVersion: 2})).toBe('chat/new:2');

describe('ConversationWorkspaceSurface', () => {
  it('renders a landing workspace without trying to mount a null chat window',()=>{
    const html=renderToStaticMarkup(<ConversationWorkspaceSurface activeSurface="workspace"
      workspaceWindow={{windowId:'menu-advertisers',windowKey:'advertiserList',windowTitle:'Advertisers'}} chatWindow={null} />);
    expect(html).toContain('data-window-id="menu-advertisers"');
    expect(html).not.toContain('data-window-id=""');
  });
  it('uses no tabs for one object and one tab per object for multiple objects by default', () => {
    expect(shouldShowWorkspaceTabs(0)).toBe(false);
    expect(shouldShowWorkspaceTabs(1)).toBe(false);
    expect(shouldShowWorkspaceTabs(2)).toBe(true);
    expect(shouldShowWorkspaceTabs(1, 'always')).toBe(true);
    expect(shouldShowWorkspaceTabs(2, 'never')).toBe(false);
  });

  it('uses a readable workspace header for URI-backed windows', () => {
    expect(resolveWorkspaceNavigation({windowKey:'window://workspace/creative',windowTitle:'Window: Workspace Creative',navigation:{chipName:'Creative'}})).toMatchObject({label:'Creative',chipName:'Creative'});
    expect(resolveWorkspaceNavigation({windowKey:'window://workspace/siteList',windowTitle:'window://workspace/siteList'})).toMatchObject({label:'Site List'});
  });

  it('resolves explicit navigation with deterministic fallbacks', () => {
    expect(resolveWorkspaceNavigation({
      windowKey: 'reportBuilder',
      windowTitle: 'Reports',
      navigation: { label: 'Performance Reports', icon: 'chart' },
    })).toMatchObject({ label: 'Performance Reports', icon: 'chart' });
    expect(resolveWorkspaceNavigation({ windowKey: 'order', windowTitle: 'order' })).toMatchObject({
      label: 'Order',
      icon: 'application',
    });
  });

  it('builds compact Chat-to-window breadcrumb ancestry', () => {
    const breadcrumbs = resolveWorkspaceBreadcrumbs({
      windowId: 'advertiser-101705',
      windowKey: 'advertiser',
      windowTitle: 'A_TEST',
      navigation: { chipName: 'Advertiser', chipIcon: 'application' },
      navigationTrail: [{
        windowId: 'advertiser-list',
        windowKey: 'advertiserList',
        windowTitle: 'Advertisers',
        navigation: { chipName: 'List', chipIcon: 'grid-view' },
      }],
    });
    expect(breadcrumbs).toEqual([
      { index: 0, windowId: 'advertiser-list', label: 'List', icon: 'grid-view', current: false },
      { index: 1, windowId: 'advertiser-101705', label: 'Advertiser', icon: 'application', current: true },
    ]);
  });

  it('renders ancestor chips as actions and the current workspace as the terminal chip', () => {
    const html = renderToStaticMarkup(<ConversationWorkspaceSurface
      activeSurface="workspace"
      chatWindow={{ windowId: 'chat' }}
      workspaceWindow={{
        windowId: 'advertiser-101705',
        windowKey: 'advertiser',
        windowTitle: 'A_TEST',
        navigation: { chipName: 'Advertiser', chipIcon: 'application' },
        navigationTrail: [{
          windowId: 'advertiser-list',
          windowKey: 'advertiserList',
          windowTitle: 'Advertisers',
          navigation: { chipName: 'List', chipIcon: 'grid-view' },
        }],
      }}
    />);
    expect(html).toContain('aria-label="Workspace navigation"');
    expect(html).toContain('title="List"');
    expect(html).toContain('aria-label="Advertiser"');
    expect(html).toContain('data-icon="grid-view"');
    expect(html).toContain('data-icon="application"');
    expect(html).toContain('aria-current="page"');
    expect(html).not.toContain('>Advertiser</span>');
  });

  it('shows a workspace link from Conversation without loading an unvisited workspace', () => {
    const html = renderToStaticMarkup(<ConversationWorkspaceSurface
      activeSurface="conversation"
      chatWindow={{ windowId: 'chat' }}
      workspaceWindow={{ windowId: 'report', windowKey: 'reportBuilder', navigation: { label: 'Reports', icon: 'chart' } }}
    />);
    expect(html).toContain('Reports');
    expect(html).toContain('data-window-id="chat"');
    expect(html).not.toContain('data-window-id="report"');
    expect(html).toContain('is-surface-hidden');
    expect(html).toContain('inert=""');
  });

  it('shows an explicit chat-cloud return action and workspace identity while retaining the composer host', () => {
    const html = renderToStaticMarkup(<ConversationWorkspaceSurface
      activeSurface="workspace"
      chatWindow={{ windowId: 'chat' }}
      workspaceWindow={{ windowId: 'report', windowKey: 'reportBuilder', navigation: { label: 'Reports', icon: 'chart' } }}
    />);
    expect(html).toContain('aria-label="Return to chat"');
    expect(html).toContain('app-summary-workspace-chat-action');
    expect(html).toContain('app-summary-workspace-identity');
    expect(html.indexOf('app-summary-workspace-identity')).toBeGreaterThan(html.indexOf('app-workspace-breadcrumbs'));
    expect(html.indexOf('app-summary-workspace-identity')).toBeLessThan(html.indexOf('app-summary-workspace-context'));
    expect(html).toContain('title="Chat"');
    expect(html).not.toContain('>Chat</button>');
    expect(html).not.toContain('app-summary-workspace-eyebrow');
    expect(html).not.toContain('>Workspace<');
    expect(html).toContain('data-window-id="report"');
    expect(html).toContain('data-window-id="chat"');
    expect(html).toContain('is-composer-only');
    expect(html).toContain('app-workspace-composer-toggle');
    expect(html).toContain('aria-label="Show composer"');
    expect(html).toContain('aria-label="Close Reports"');
    expect(html).toContain('app-workspace-window-control is-close');
    expect(html).toContain('app-workspace-window-control is-layout');
    expect(html).toContain('title="Restore split view"');
    expect(html).not.toContain('>Close</button>');
    expect(html).not.toContain('>Focus</button>');
    expect(html).not.toContain('>Split</button>');
    expect(html).not.toContain('app-window-dot-close');
  });

  it('suppresses duplicate top navigation when the transcript owns the workspace link', () => {
    const html = renderToStaticMarkup(<ConversationWorkspaceSurface
      activeSurface="conversation"
      chatWindow={{ windowId: 'chat' }}
      workspaceWindow={{ windowId: 'report', windowKey: 'reportBuilder', navigation: { label: 'Reports', icon: 'chart' } }}
      suppressConversationWorkspaceLink
    />);
    expect(html).not.toContain('app-summary-surface-navigation');
    expect(html).not.toContain('Return to Reports');
    expect(html).not.toContain('data-window-id="report"');
  });

  it('does not create Workspace navigation without an eligible hosted window', () => {
    const html = renderToStaticMarkup(<ConversationWorkspaceSurface
      activeSurface="conversation"
      chatWindow={{ windowId: 'chat' }}
      workspaceWindow={null}
    />);
    expect(html).toContain('data-window-id="chat"');
    expect(html).not.toContain('Return to');
    expect(html).not.toContain('app-summary-workspace');
  });

  it('renders multiple hosted windows as nested Workspace tabs', () => {
    const html = renderToStaticMarkup(<ConversationWorkspaceSurface
      activeSurface="workspace"
      chatWindow={{ windowId: 'chat' }}
      workspaceWindow={{ windowId: 'report', windowKey: 'reportBuilder', navigation: { label: 'Reports', icon: 'chart' } }}
      workspaceTabs={[
        { windowId: 'report', label: 'Reports', isActive: true },
        { windowId: 'preview', label: 'Preview', isActive: false },
      ]}
    />);
    expect(html).toContain('role="tablist"');
    expect(html).toContain('aria-selected="true"');
    expect(html).toContain('Preview');
  });

  it('hosts policy-approved MCP UI inside Workspace while retaining navigation metadata', () => {
    const html = renderToStaticMarkup(<ConversationWorkspaceSurface
      activeSurface="workspace"
      chatWindow={{ windowId: 'chat' }}
      workspaceWindow={{
        windowId: 'mcp-diagnostics',
        conversationId: 'conv-1',
        navigation: { label: 'Diagnostics', icon: 'application' },
        mcpUI: { uri: 'ui://diagnostics', title: 'Delivery diagnostics' },
      }}
    />);
    expect(html).toContain('Diagnostics');
    expect(html).toContain('data-mcp-uri="ui://diagnostics"');
    expect(html).toContain('data-hosted="true"');
    expect(html).toContain('aria-label="Return to chat"');
  });
});

it('names the green layout control for maximize and restore states', () => {
  for (const [mode, label, pressed] of [['split', 'Maximize workspace', 'false'], ['focus', 'Restore split view', 'true']]) {
    const html = renderToStaticMarkup(<ConversationWorkspaceSurface activeSurface="workspace" workspaceMode={mode} chatWindow={{windowId:'chat'}} workspaceWindow={{windowId:'report',windowKey:'reportBuilder'}} />);
    expect(html).toContain(`aria-label="${label}"`);
    expect(html).toContain(`aria-pressed="${pressed}"`);
  }
});

it('renders the shared Forge tab runtime and retains host renderer nodes across native keyboard selection',async()=>{
 globalThis.IS_REACT_ACT_ENVIRONMENT=true;window.matchMedia=()=>({matches:false,addEventListener(){},removeEventListener(){}});const host=document.createElement('div');document.body.append(host);const root=createRoot(host),close=vi.fn();
 const windows=[{windowId:'host-a',windowKey:'a',windowTitle:'A'},{windowId:'host-b',windowKey:'b',windowTitle:'B',workspaceObject:{capabilities:{close:false}}}];
 function App(){const [selected,setSelected]=React.useState('host-a');return <ConversationWorkspaceSurface activeSurface="workspace" workspaceWindow={windows.find(entry=>entry.windowId===selected)} workspaceWindows={windows} workspaceTabs={windows.map(entry=>({windowId:entry.windowId,label:entry.windowTitle,isActive:entry.windowId===selected}))} onSelectWorkspaceTab={setSelected} onCloseWorkspaceTab={close} renderConversation={()=><div>Host conversation</div>}/>;}
 try{await act(async()=>root.render(<App/>));const original=host.querySelector('[data-window-id="host-a"]');expect(original).not.toBeNull();expect(host.querySelector('[data-window-id="host-b"]')).toBeNull();const first=host.querySelector('[role=tab]');first.focus();await act(async()=>first.dispatchEvent(new KeyboardEvent('keydown',{key:'ArrowRight',bubbles:true})));expect(document.activeElement.textContent.trim()).toBe('B');expect(original.closest('[role=tabpanel]').hidden).toBe(true);expect(host.querySelector('[aria-label="Close B"]').disabled).toBe(true);await act(async()=>document.activeElement.dispatchEvent(new KeyboardEvent('keydown',{key:'Home',bubbles:true})));expect(host.querySelector('[data-window-id="host-a"]')).toBe(original);expect(host.querySelector('[data-active-surface]').dataset.activeSurface).toBe('workspace');expect(host.querySelector('[aria-label="Show composer"]')).not.toBeNull();await act(async()=>host.querySelector('[role=tablist] [aria-label="Close A"]').click());expect(close).toHaveBeenCalledWith('host-a',expect.anything());
 }finally{await act(async()=>root.unmount());host.remove();}
});

it('mounts a hidden fresh opening renderer for readiness while restored windows await a visible row-link selection',async()=>{
 globalThis.IS_REACT_ACT_ENVIRONMENT=true;
 window.matchMedia=()=>({matches:false,addEventListener(){},removeEventListener(){}});
 const host=document.createElement('div');document.body.append(host);const root=createRoot(host);
 const ready=vi.fn();
 const fresh={windowId:'fresh-pending-readiness',windowKey:'fresh',windowTitle:'Fresh',hostOpenState:'fresh',workspaceObject:{lifecycle:{state:'opening'}},onRendererReady:()=>ready('fresh')};
 const restored={windowId:'restored-inactive',windowKey:'restored',windowTitle:'Restored',hostOpenState:'historical_replay',workspaceObject:{lifecycle:{state:'opening'}},onRendererReady:()=>ready('restored')};
 const show=(activeSurface='conversation',selected=restored)=>act(async()=>root.render(<ConversationWorkspaceSurface
  activeSurface={activeSurface} workspaceWindow={selected} workspaceWindows={[fresh,restored]}
  renderConversation={()=><textarea aria-label="Conversation input"/>}/>));
 try{
  await show();
  const renderer=host.querySelector('[data-window-id="fresh-pending-readiness"]');
  expect(ready.mock.calls).toEqual([['fresh']]);
  expect(renderer).not.toBeNull();
  expect(renderer.closest('[role=tabpanel]').hidden).toBe(true);
  expect(renderer.closest('[role=tabpanel]').hasAttribute('inert')).toBe(true);
  expect(host.querySelector('[data-window-id="restored-inactive"]')).toBeNull();
  expect(host.querySelector('[data-active-surface]').dataset.activeSurface).toBe('conversation');
  const composer=host.querySelector('textarea');composer.focus();
  fresh.workspaceObject.lifecycle.state='ready';
  await show();
  expect(host.querySelector('[data-window-id="fresh-pending-readiness"]')).toBe(renderer);
  expect(document.activeElement).toBe(composer);
  // A normal row-link selects the workspace surface directly; it must still
  // mount its selected renderer without fresh-opening preload admission.
  await show('workspace',restored);
  expect(host.querySelector('[data-window-id="restored-inactive"]').closest('[role=tabpanel]').hidden).toBe(false);
  expect(ready.mock.calls).toEqual([['fresh'],['restored']]);
  expect(host.querySelector('[data-window-id="fresh-pending-readiness"]')).toBe(renderer);
 }finally{await act(async()=>root.unmount());host.remove();}
});
