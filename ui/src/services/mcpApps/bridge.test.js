import { describe, expect, it } from 'vitest';
import { buildEnvelope, MCPUI_METHODS } from './appproto.js';
import {
  classifyOpenLinkURL,
  executeGuestToolCall,
  handleGuestEnvelope,
  isAllowedTool,
  normalizeSafeOpenLink,
} from './bridge.js';

describe('mcpApps/bridge', () => {
  it('checks allowed tools exactly', () => {
    expect(isAllowedTool(['demo:show_widget'], 'demo:show_widget')).toBe(true);
    expect(isAllowedTool(['demo:show_widget'], 'message:add')).toBe(false);
  });

  it('returns message passthrough for mcpui:message', async () => {
    const out = await handleGuestEnvelope(buildEnvelope(MCPUI_METHODS.MESSAGE, { content: 'hello' }), {});
    expect(out).toEqual({ type: 'message', payload: { content: 'hello' } });
  });

  it('returns open-link passthrough for mcpui:open-link', async () => {
    const out = await handleGuestEnvelope(buildEnvelope(MCPUI_METHODS.OPEN_LINK, { url: 'https://example.com' }), {});
    expect(out).toEqual({ type: 'open-link', payload: { url: 'https://example.com/' } });
  });

  it('normalizes https open-link URLs exactly', () => {
    expect(normalizeSafeOpenLink('https://example.com/path')).toBe('https://example.com/path');
  });

  it('rejects javascript open-link URLs', () => {
    expect(() => normalizeSafeOpenLink('javascript:alert(1)')).toThrow('open-link url is not allowed');
  });

  it('rejects data open-link URLs', () => {
    expect(() => normalizeSafeOpenLink('data:text/html,boom')).toThrow('open-link url is not allowed');
  });

  it('rejects relative open-link URLs', () => {
    expect(() => normalizeSafeOpenLink('/local/path')).toThrow('open-link url is invalid');
  });

  it('rejects malformed open-link URLs', () => {
    expect(() => normalizeSafeOpenLink('not a url')).toThrow('open-link url is invalid');
  });

  it('classifyOpenLinkURL returns a structured accept verdict for https urls', () => {
    expect(classifyOpenLinkURL('https://example.com/path')).toEqual({
      ok: true,
      url: 'https://example.com/path',
    });
  });

  it('classifyOpenLinkURL rejects javascript:, data:, relative, malformed and empty urls deterministically', () => {
    expect(classifyOpenLinkURL('javascript:alert(1)')).toEqual({
      ok: false,
      reason: 'open-link url is not allowed: javascript:',
    });
    expect(classifyOpenLinkURL('data:text/html,boom')).toEqual({
      ok: false,
      reason: 'open-link url is not allowed: data:',
    });
    expect(classifyOpenLinkURL('/local/path')).toEqual({
      ok: false,
      reason: 'open-link url is invalid: /local/path',
    });
    expect(classifyOpenLinkURL('not a url')).toEqual({
      ok: false,
      reason: 'open-link url is invalid: not a url',
    });
    expect(classifyOpenLinkURL('')).toEqual({
      ok: false,
      reason: 'open-link url is required',
    });
    // http: must also reject — MVP is https-only.
    expect(classifyOpenLinkURL('http://example.com')).toEqual({
      ok: false,
      reason: 'open-link url is not allowed: http:',
    });
  });

  it('handleGuestEnvelope rejects unsafe open-link schemes through the host-owned path', async () => {
    await expect(handleGuestEnvelope(
      buildEnvelope(MCPUI_METHODS.OPEN_LINK, { url: 'javascript:alert(1)' }),
      {},
    )).rejects.toThrow('open-link url is not allowed');
    await expect(handleGuestEnvelope(
      buildEnvelope(MCPUI_METHODS.OPEN_LINK, { url: 'data:text/html,boom' }),
      {},
    )).rejects.toThrow('open-link url is not allowed');
    await expect(handleGuestEnvelope(
      buildEnvelope(MCPUI_METHODS.OPEN_LINK, { url: '/local/path' }),
      {},
    )).rejects.toThrow('open-link url is invalid');
    await expect(handleGuestEnvelope(
      buildEnvelope(MCPUI_METHODS.OPEN_LINK, { url: 'not a url' }),
      {},
    )).rejects.toThrow('open-link url is invalid');
  });

  it('executes allowed guest tool call and returns tool-result envelope', async () => {
    const out = await handleGuestEnvelope(
      buildEnvelope(MCPUI_METHODS.TOOLS_CALL, {
        windowId: 'w1',
        resourceUri: 'ui://demo',
        name: 'demo:show_widget',
        arguments: { title: 'hello' },
      }),
      {
        allowedTools: ['demo:show_widget'],
        allowedToolBundles: ['mcp_ui_preview_queue'],
        conversationId: 'conv-1',
        protocolVersion: '1.0.0',
        fetchImpl: async () => ({
          ok: true,
          json: async () => ({ result: 'tool-ok' }),
        }),
      },
    );
    expect(out.type).toBe('tool-result');
    expect(out.payload.method).toBe(MCPUI_METHODS.TOOL_RESULT);
    expect(out.payload.params.toolName).toBe('demo:show_widget');
  });

  it('rejects disallowed guest tool calls', async () => {
    await expect(handleGuestEnvelope(
      buildEnvelope(MCPUI_METHODS.TOOLS_CALL, { name: 'demo:show_widget' }),
      { allowedTools: [] },
    )).rejects.toThrow('tool not allowed');
  });

  it('calls the execute endpoint', async () => {
    const payload = await executeGuestToolCall(
      {
        conversationId: 'conv-1',
        name: 'demo:show_widget',
        arguments: { title: 'hello' },
        assistantText: 'Run demo tool from MCP UI.',
        toolBundles: ['mcp_ui_preview_queue'],
      },
      async (url, init) => {
        expect(url).toContain('/v1/api/mcp-ui/tools/call');
        expect(init.method).toBe('POST');
        expect(JSON.parse(init.body)).toEqual({
          conversationId: 'conv-1',
          toolName: 'demo:show_widget',
          arguments: { title: 'hello' },
          assistantText: 'Run demo tool from MCP UI.',
          toolBundles: ['mcp_ui_preview_queue'],
        });
        return {
          ok: true,
          json: async () => ({ result: 'ok' }),
        };
      },
    );
    expect(payload).toEqual({ result: 'ok' });
  });
});
