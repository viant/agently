import { describe, expect, it, vi } from 'vitest';
import {
  buildDefaultMCPUICSP,
  buildMCPUICSPFromPolicy,
  buildSandboxedSrcDoc,
  createMCPUINonce,
  injectNonceIntoInlineTags,
  isMCPUIHTMLResponse,
  MCP_UI_CSP_NONCE_TOKEN,
  MCP_UI_DEFAULT_SANDBOX,
  MCP_UI_HTML_MIME,
  readMCPUIResource,
  normalizeMCPUICSPPolicy,
  resolveMCPUICSP,
  resolveMCPUIFrameConfig,
} from './resourceLoader.js';

describe('mcpApps/resourceLoader', () => {
  it('recognizes MCP UI HTML MIME', () => {
    expect(isMCPUIHTMLResponse({ mimeType: MCP_UI_HTML_MIME })).toBe(true);
    expect(isMCPUIHTMLResponse({ mimeType: 'text/html' })).toBe(false);
  });

  it('builds srcDoc wrapper around HTML payload', () => {
    const html = '<main><h1>Hello</h1></main>';
    const srcDoc = buildSandboxedSrcDoc({ html, title: 'Preview' });
    expect(srcDoc).toContain('<title>Preview</title>');
    expect(srcDoc).toContain(html);
    expect(srcDoc).toContain('<!DOCTYPE html>');
    expect(srcDoc).toContain('http-equiv="Content-Security-Policy"');
  });

  it('resolves explicit renderer and sandbox from resource meta', () => {
    const cfg = resolveMCPUIFrameConfig({
      _meta: {
        ui: {
          rendererUrl: '/mcp-ui-forge-window.html?windowKey=order',
          sandbox: 'allow-scripts allow-same-origin',
        },
      },
    });
    expect(cfg).toEqual({
      rendererUrl: '/mcp-ui-forge-window.html?windowKey=order',
      sandbox: 'allow-scripts allow-same-origin',
      csp: '',
      cspPolicy: null,
      useSrc: true,
    });
  });

  it('falls back to default sandbox when no renderer metadata is present', () => {
    const cfg = resolveMCPUIFrameConfig({});
    expect(cfg).toEqual({
      rendererUrl: '',
      sandbox: MCP_UI_DEFAULT_SANDBOX,
      csp: '',
      cspPolicy: null,
      useSrc: false,
    });
  });

  it('surfaces explicit legacy csp override from resource meta without changing normal frame config behavior', () => {
    const cfg = resolveMCPUIFrameConfig({
      _meta: {
        ui: {
          csp: `default-src 'none'; script-src 'nonce-${MCP_UI_CSP_NONCE_TOKEN}'`,
        },
      },
    });
    expect(cfg).toEqual({
      rendererUrl: '',
      sandbox: MCP_UI_DEFAULT_SANDBOX,
      csp: `default-src 'none'; script-src 'nonce-${MCP_UI_CSP_NONCE_TOKEN}'`,
      cspPolicy: null,
      useSrc: false,
    });
  });

  it('surfaces structured csp policy from resource meta', () => {
    const cfg = resolveMCPUIFrameConfig({
      _meta: {
        ui: {
          cspPolicy: {
            scriptSrc: ['https://cdn.example'],
            connectSrc: ['https://api.example'],
            scriptStrictDynamic: true,
          },
        },
      },
    });
    expect(cfg).toEqual({
      rendererUrl: '',
      sandbox: MCP_UI_DEFAULT_SANDBOX,
      csp: '',
      cspPolicy: {
        scriptSrc: ['https://cdn.example'],
        styleSrc: [],
        connectSrc: ['https://api.example'],
        imgSrc: [],
        fontSrc: [],
        scriptStrictDynamic: true,
      },
      useSrc: false,
    });
  });

  it('injects a deterministic nonce into inline script and style tags', () => {
    const html = '<style>.x{color:red}</style><script>window.x=1</script>';
    expect(injectNonceIntoInlineTags(html, 'abc123')).toBe('<style nonce="abc123">.x{color:red}</style><script nonce="abc123">window.x=1</script>');
  });

  it('builds default strict srcdoc csp and applies the generated nonce before active content', () => {
    vi.spyOn(globalThis, 'crypto', 'get').mockReturnValue({
      getRandomValues(buffer) {
        buffer.set(Uint8Array.from({ length: buffer.length }, (_, i) => i + 1));
        return buffer;
      },
    });
    const srcDoc = buildSandboxedSrcDoc({
      html: '<style>.x{color:red}</style><script>window.x=1</script>',
      title: 'Nonce Test',
    });
    const expectedNonce = '0102030405060708090a0b0c0d0e0f10';
    const cspTagIndex = srcDoc.indexOf('http-equiv="Content-Security-Policy"');
    const scriptIndex = srcDoc.indexOf('<script nonce=');
    expect(cspTagIndex).toBeGreaterThan(-1);
    expect(scriptIndex).toBeGreaterThan(cspTagIndex);
    expect(srcDoc).toContain(buildDefaultMCPUICSP(expectedNonce));
    expect(srcDoc).toContain('<style nonce="0102030405060708090a0b0c0d0e0f10">html,body{margin:0;padding:0;background:#fff;}</style>');
    expect(srcDoc).toContain('<style nonce="0102030405060708090a0b0c0d0e0f10">');
    expect(srcDoc).toContain('<script nonce="0102030405060708090a0b0c0d0e0f10">');
    expect(srcDoc).not.toContain('<body style=');
    vi.restoreAllMocks();
  });

  it('honors explicit csp override only when present by replacing the nonce token', () => {
    expect(resolveMCPUICSP({
      csp: `default-src 'none'; script-src 'nonce-${MCP_UI_CSP_NONCE_TOKEN}' https://cdn.example`,
      nonce: 'abc123',
    })).toBe("default-src 'none'; script-src 'nonce-abc123' https://cdn.example");
  });

  it('builds a structured relaxation policy with explicit allowlists and strict-dynamic', () => {
    expect(buildMCPUICSPFromPolicy({
      nonce: 'abc123',
      policy: {
        scriptSrc: ['https://cdn.example'],
        styleSrc: ['https://fonts.example'],
        connectSrc: ['https://api.example'],
        imgSrc: ['https://img.example'],
        fontSrc: ['https://fonts.example'],
        scriptStrictDynamic: true,
      },
    })).toBe("default-src 'none'; script-src 'nonce-abc123' https://cdn.example 'strict-dynamic'; style-src 'nonce-abc123' https://fonts.example; img-src data: https://img.example; font-src data: https://fonts.example; connect-src https://api.example; base-uri 'none'; form-action 'none'");
  });

  it('prefers structured csp policy over legacy raw csp when both are present', () => {
    expect(resolveMCPUICSP({
      csp: `default-src 'none'; script-src 'nonce-${MCP_UI_CSP_NONCE_TOKEN}' https://legacy.example`,
      cspPolicy: {
        scriptSrc: ['https://cdn.example'],
      },
      nonce: 'abc123',
    })).toBe("default-src 'none'; script-src 'nonce-abc123' https://cdn.example; style-src 'nonce-abc123'; img-src data:; font-src data:; connect-src 'none'; base-uri 'none'; form-action 'none'");
  });

  it('normalizes malformed or empty structured csp policy inputs to null', () => {
    expect(normalizeMCPUICSPPolicy(null)).toBeNull();
    expect(normalizeMCPUICSPPolicy({})).toBeNull();
    expect(normalizeMCPUICSPPolicy({ scriptSrc: ['', 'https://cdn.example'] })).toEqual({
      scriptSrc: ['https://cdn.example'],
      styleSrc: [],
      connectSrc: [],
      imgSrc: [],
      fontSrc: [],
      scriptStrictDynamic: false,
    });
  });

  it('reads and validates an mcp-ui resource payload', async () => {
    const payload = {
      uri: 'ui://agently.wk_demo/demo/show_widget',
      mimeType: MCP_UI_HTML_MIME,
      text: '<main>ok</main>',
      _meta: { ui: { resourceUri: 'ui://agently.wk_demo/demo/show_widget' } },
    };
    const result = await readMCPUIResource(payload.uri, async () => ({
      ok: true,
      json: async () => payload,
    }));
    expect(result).toEqual(payload);
  });

  it('rejects non-mcp-ui mime payloads', async () => {
    await expect(readMCPUIResource('ui://bad', async () => ({
      ok: true,
      json: async () => ({ uri: 'ui://bad', mimeType: 'text/plain', text: 'oops' }),
    }))).rejects.toThrow(MCP_UI_HTML_MIME);
  });
});
