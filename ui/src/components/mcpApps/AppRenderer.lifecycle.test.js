import { describe, expect, it } from 'vitest';
import { buildInitialHostNotifications, buildTeardownNotification } from './AppRenderer.jsx';

describe('AppRenderer lifecycle notifications', () => {
  it('builds host-ready and tool-input notifications with exact envelope shape', () => {
    expect(buildInitialHostNotifications({
      windowId: 'mcpui-preview:ui://mcpuiverify/demo/verify_widget',
      resourceUri: 'ui://mcpuiverify/demo/verify_widget',
      allowedTools: ['system/os:getEnv'],
      allowedToolBundles: ['mcpuiverify_queue'],
      protocolVersion: '1.0.0',
      toolInput: { title: 'hello' },
    })).toEqual([
      {
        version: '1.0.0',
        method: 'mcpui:host-ready',
        params: {
          windowId: 'mcpui-preview:ui://mcpuiverify/demo/verify_widget',
          resourceUri: 'ui://mcpuiverify/demo/verify_widget',
          allowedTools: ['system/os:getEnv'],
          allowedToolBundles: ['mcpuiverify_queue'],
          protocolVersion: '1.0.0',
        },
      },
      {
        version: '1.0.0',
        method: 'mcpui:tool-input',
        params: {
          windowId: 'mcpui-preview:ui://mcpuiverify/demo/verify_widget',
          resourceUri: 'ui://mcpuiverify/demo/verify_widget',
          input: { title: 'hello' },
        },
      },
    ]);
  });

  it('builds optional tool-input-partial and teardown notifications', () => {
    expect(buildInitialHostNotifications({
      windowId: 'w1',
      resourceUri: 'ui://demo/widget',
      toolInputPartial: { title: 'hel' },
    })).toEqual([
      {
        version: '1.0.0',
        method: 'mcpui:host-ready',
        params: {
          windowId: 'w1',
          resourceUri: 'ui://demo/widget',
          allowedTools: [],
          allowedToolBundles: [],
          protocolVersion: '1.0.0',
        },
      },
      {
        version: '1.0.0',
        method: 'mcpui:tool-input-partial',
        params: {
          windowId: 'w1',
          resourceUri: 'ui://demo/widget',
          input: { title: 'hel' },
        },
      },
    ]);
    expect(buildTeardownNotification({ windowId: 'w1', resourceUri: 'ui://demo/widget' })).toEqual({
      version: '1.0.0',
      method: 'mcpui:teardown',
      params: {
        windowId: 'w1',
        resourceUri: 'ui://demo/widget',
      },
    });
  });
});
