import { describe, expect, it } from 'vitest';
import { buildEnvelope, isEnvelopeCandidate, MCPUI_METHODS, MCPUI_VERSION, validateEnvelope } from './appproto.js';

describe('mcpApps/appproto', () => {
  it('builds versioned envelopes', () => {
    const env = buildEnvelope(MCPUI_METHODS.MESSAGE, { content: 'hi' });
    expect(env.version).toBe(MCPUI_VERSION);
    expect(env.method).toBe(MCPUI_METHODS.MESSAGE);
    expect(env.params).toEqual({ content: 'hi' });
  });

  it('validates supported envelopes', () => {
    const result = validateEnvelope(buildEnvelope(MCPUI_METHODS.OPEN_LINK, { url: 'https://example.com' }));
    expect(result.ok).toBe(true);
    expect(result.envelope.method).toBe(MCPUI_METHODS.OPEN_LINK);
  });

  it('rejects unsupported versions and methods', () => {
    expect(validateEnvelope({ version: '9.9.9', method: MCPUI_METHODS.MESSAGE, params: {} }).ok).toBe(false);
    expect(validateEnvelope({ version: MCPUI_VERSION, method: 'wrong', params: {} }).ok).toBe(false);
    expect(isEnvelopeCandidate(null)).toBe(false);
  });
});
