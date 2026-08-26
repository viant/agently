import { describe, expect, it, vi } from 'vitest';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';

vi.mock('@blueprintjs/core', () => ({
  Button: ({ text = '', icon = '', minimal: _minimal, small: _small, ...props }) => React.createElement('button', { ...props, 'data-icon': icon }, text),
}));
vi.mock('forge/components', () => ({
  WindowContent: ({ window }) => React.createElement('div', { 'data-window-id': window?.windowId || '' }),
}));
vi.mock('./mcpApps/AppRenderer.jsx', () => ({
  default: ({ uri, hosted }) => React.createElement('div', { 'data-mcp-uri': uri, 'data-hosted': hosted ? 'true' : 'false' }),
}));

import ConversationWorkspaceSurface, { resolveWorkspaceNavigation } from './ConversationWorkspaceSurface';

describe('ConversationWorkspaceSurface', () => {
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

  it('shows a workspace link from Conversation while keeping workspace mounted and hidden', () => {
    const html = renderToStaticMarkup(<ConversationWorkspaceSurface
      activeSurface="conversation"
      chatWindow={{ windowId: 'chat' }}
      workspaceWindow={{ windowId: 'report', windowKey: 'reportBuilder', navigation: { label: 'Reports', icon: 'chart' } }}
    />);
    expect(html).toContain('Reports');
    expect(html).toContain('data-window-id="chat"');
    expect(html).toContain('data-window-id="report"');
    expect(html).toContain('is-surface-hidden');
  });

  it('shows Workspace with a back link while retaining the composer host', () => {
    const html = renderToStaticMarkup(<ConversationWorkspaceSurface
      activeSurface="workspace"
      chatWindow={{ windowId: 'chat' }}
      workspaceWindow={{ windowId: 'report', windowKey: 'reportBuilder', navigation: { label: 'Reports', icon: 'chart' } }}
    />);
    expect(html).toContain('Conversation');
    expect(html).toContain('data-window-id="report"');
    expect(html).toContain('data-window-id="chat"');
    expect(html).toContain('is-composer-only');
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
    expect(html).toContain('data-window-id="report"');
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
    expect(html).toContain('Back to Conversation');
  });
});
