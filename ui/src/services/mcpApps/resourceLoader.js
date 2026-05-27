import { sdkBaseURL } from '../../endpoint';

export const MCP_UI_READ_PATH = `${sdkBaseURL}/api/mcp-ui/resources/read`;
export const MCP_UI_HTML_MIME = 'text/html;profile=mcp-app';
export const MCP_UI_DEFAULT_SANDBOX = 'allow-scripts';
export const MCP_UI_CSP_NONCE_TOKEN = '{{nonce}}';

export function isMCPUIHTMLResponse(payload = null) {
  return String(payload?.mimeType || '').trim().toLowerCase() === MCP_UI_HTML_MIME;
}

export function createMCPUINonce() {
  const bytes = new Uint8Array(16);
  if (globalThis.crypto?.getRandomValues) {
    globalThis.crypto.getRandomValues(bytes);
  } else {
    for (let i = 0; i < bytes.length; i += 1) {
      bytes[i] = Math.floor(Math.random() * 256);
    }
  }
  return Array.from(bytes, (value) => value.toString(16).padStart(2, '0')).join('');
}

export function buildDefaultMCPUICSP(nonce = '') {
  const safeNonce = String(nonce || '').trim();
  return [
    "default-src 'none'",
    `script-src 'nonce-${safeNonce}'`,
    `style-src 'nonce-${safeNonce}'`,
    "img-src data:",
    "font-src data:",
    "connect-src 'none'",
    "base-uri 'none'",
    "form-action 'none'",
  ].join('; ');
}

function normalizeStringList(value) {
  if (!Array.isArray(value)) return [];
  return value
    .map((item) => String(item || '').trim())
    .filter(Boolean);
}

export function normalizeMCPUICSPPolicy(value = null) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return null;
  }
  const policy = {
    scriptSrc: normalizeStringList(value.scriptSrc),
    styleSrc: normalizeStringList(value.styleSrc),
    connectSrc: normalizeStringList(value.connectSrc),
    imgSrc: normalizeStringList(value.imgSrc),
    fontSrc: normalizeStringList(value.fontSrc),
    scriptStrictDynamic: value.scriptStrictDynamic === true,
  };
  const hasEntries = policy.scriptSrc.length > 0
    || policy.styleSrc.length > 0
    || policy.connectSrc.length > 0
    || policy.imgSrc.length > 0
    || policy.fontSrc.length > 0
    || policy.scriptStrictDynamic;
  return hasEntries ? policy : null;
}

export function buildMCPUICSPFromPolicy({ nonce = '', policy = null } = {}) {
  const safeNonce = String(nonce || '').trim();
  const normalized = normalizeMCPUICSPPolicy(policy);
  if (!normalized) {
    return buildDefaultMCPUICSP(safeNonce);
  }
  const scriptSrc = [`'nonce-${safeNonce}'`, ...normalized.scriptSrc];
  if (normalized.scriptStrictDynamic) {
    scriptSrc.push("'strict-dynamic'");
  }
  const styleSrc = [`'nonce-${safeNonce}'`, ...normalized.styleSrc];
  const connectSrc = normalized.connectSrc.length > 0 ? normalized.connectSrc : ["'none'"];
  const imgSrc = normalized.imgSrc.length > 0 ? ['data:', ...normalized.imgSrc] : ['data:'];
  const fontSrc = normalized.fontSrc.length > 0 ? ['data:', ...normalized.fontSrc] : ['data:'];
  return [
    "default-src 'none'",
    `script-src ${scriptSrc.join(' ')}`,
    `style-src ${styleSrc.join(' ')}`,
    `img-src ${imgSrc.join(' ')}`,
    `font-src ${fontSrc.join(' ')}`,
    `connect-src ${connectSrc.join(' ')}`,
    "base-uri 'none'",
    "form-action 'none'",
  ].join('; ');
}

export function injectNonceIntoInlineTags(html = '', nonce = '') {
  const safeNonce = String(nonce || '').trim();
  const source = String(html || '');
  if (!safeNonce || !source) return source;
  return source
    .replace(/<script\b(?![^>]*\bnonce=)([^>]*)>/gi, `<script nonce="${safeNonce}"$1>`)
    .replace(/<style\b(?![^>]*\bnonce=)([^>]*)>/gi, `<style nonce="${safeNonce}"$1>`);
}

export function resolveMCPUICSP({ csp = '', cspPolicy = null, nonce = '' } = {}) {
  const normalizedPolicy = normalizeMCPUICSPPolicy(cspPolicy);
  if (normalizedPolicy) {
    return buildMCPUICSPFromPolicy({ nonce, policy: normalizedPolicy });
  }
  const override = String(csp || '').trim();
  if (!override) {
    return buildDefaultMCPUICSP(nonce);
  }
  return override.split(MCP_UI_CSP_NONCE_TOKEN).join(String(nonce || '').trim());
}

export function buildSandboxedSrcDoc({ html = '', title = 'MCP UI Resource', csp = '', cspPolicy = null } = {}) {
  const docTitle = String(title || 'MCP UI Resource').trim() || 'MCP UI Resource';
  const nonce = createMCPUINonce();
  const body = injectNonceIntoInlineTags(String(html || ''), nonce);
  const cspValue = resolveMCPUICSP({ csp, cspPolicy, nonce });
  return `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8" />
  <meta http-equiv="Content-Security-Policy" content="${cspValue}" />
  <title>${docTitle}</title>
  <style nonce="${nonce}">html,body{margin:0;padding:0;background:#fff;}</style>
</head>
<body>
${body}
</body>
</html>`;
}

export function resolveMCPUIFrameConfig(payload = null) {
  const uiMeta = payload?._meta?.ui || {};
  const rendererUrl = String(uiMeta?.rendererUrl || '').trim();
  const sandbox = String(uiMeta?.sandbox || '').trim() || MCP_UI_DEFAULT_SANDBOX;
  const csp = String(uiMeta?.csp || '').trim();
  const cspPolicy = normalizeMCPUICSPPolicy(uiMeta?.cspPolicy);
  return {
    rendererUrl,
    sandbox,
    csp,
    cspPolicy,
    useSrc: rendererUrl !== '',
  };
}

export async function readMCPUIResource(uri, fetchImpl = globalThis.fetch) {
  const normalized = String(uri || '').trim();
  if (!normalized) {
    throw new Error('uri is required');
  }
  if (typeof fetchImpl !== 'function') {
    throw new Error('fetch implementation is required');
  }
  const response = await fetchImpl(`${MCP_UI_READ_PATH}?uri=${encodeURIComponent(normalized)}`, {
    method: 'GET',
    credentials: 'include',
    headers: {
      Accept: 'application/json',
    },
  });
  if (!response.ok) {
    const text = await response.text().catch(() => '');
    throw new Error(text || `mcp-ui resource read failed (${response.status})`);
  }
  const payload = await response.json();
  if (!isMCPUIHTMLResponse(payload)) {
    throw new Error(`expected ${MCP_UI_HTML_MIME} resource, got ${payload?.mimeType || 'unknown'}`);
  }
  if (!String(payload?.text || '').trim()) {
    throw new Error('mcp-ui resource text is required');
  }
  return payload;
}
