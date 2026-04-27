import React, { useEffect, useMemo, useState } from 'react';
import { Card, H2, H5, Tag } from '@blueprintjs/core';
import NamedLookupInput from './components/lookups/NamedLookupInput.jsx';
import { flattenStored, parseTokens } from './components/lookups/tokens.js';

const originalFetch = globalThis.fetch?.bind(globalThis);

function mockResponse(body, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    statusText: status === 200 ? 'OK' : 'Error',
    async json() {
      return body;
    },
  };
}

function installLookupMocks() {
  if (!originalFetch) return () => {};
  globalThis.fetch = async (input, init = {}) => {
    const url = typeof input === 'string' ? input : String(input?.url || '');
    if (url.includes('/v1/api/lookups/registry')) {
      return mockResponse({
        entries: [
          {
            name: 'order',
            dataSource: 'orders',
            display: '${name}',
            required: true,
            token: {
              store: '${id}',
              display: '${name}',
              modelForm: '${id}',
              queryInput: 'q',
              resolveInput: 'id',
            },
          },
        ],
      });
    }

    if (url.includes('/v1/api/datasources/orders/fetch')) {
      const body = (() => {
        try {
          return JSON.parse(String(init?.body || '{}'));
        } catch (_) {
          return {};
        }
      })();
      const inputs = body?.inputs || {};
      if (inputs.id === '42') {
        return mockResponse({ rows: [{ id: 42, name: 'Order 42', advertiser: 'Northwind' }] });
      }
      if (inputs.q != null) {
        const q = String(inputs.q || '').trim().toLowerCase();
        const rows = [
          { id: 42, name: 'Order 42', advertiser: 'Northwind' },
          { id: 84, name: 'Order 84', advertiser: 'Contoso' },
        ].filter((row) => row.name.toLowerCase().includes(q) || String(row.id).includes(q));
        return mockResponse({ rows });
      }
      return mockResponse({ rows: [] });
    }

    return originalFetch(input, init);
  };

  return () => {
    globalThis.fetch = originalFetch;
  };
}

export default function LookupChipPreview() {
  const [value, setValue] = useState('Troubleshoot @{order:7 "Order 7"} for pacing issues.');
  const registry = useMemo(() => ([
    {
      name: 'order',
      dataSource: 'orders',
      display: '${name}',
      required: true,
      token: {
        store: '${id}',
        display: '${name}',
        modelForm: '${id}',
        queryInput: 'q',
        resolveInput: 'id',
      },
    },
  ]), []);
  const parsed = useMemo(() => parseTokens(value), [value]);
  const llmValue = useMemo(() => {
    try {
      return flattenStored(value, registry);
    } catch (err) {
      return String(err?.message || err || '');
    }
  }, [registry, value]);
  useEffect(() => installLookupMocks(), []);

  return (
    <div
      style={{
        minHeight: '100vh',
        background: 'linear-gradient(180deg, #f6f8fc 0%, #eef2f8 100%)',
        padding: '48px 24px',
        boxSizing: 'border-box',
        fontFamily: 'Inter, system-ui, sans-serif',
      }}
    >
      <div style={{ maxWidth: 960, margin: '0 auto' }}>
        <div style={{ marginBottom: 20 }}>
          <H2 style={{ margin: 0 }}>Lookup Chip Preview</H2>
          <p style={{ margin: '8px 0 0', color: '#5f6b7c', fontSize: 15 }}>
            Click the chip to switch into direct id entry. Blur resolves by exact id. The search control still opens the lookup path.
          </p>
          <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginTop: 12 }}>
            <Tag minimal style={{ borderRadius: 999, padding: '6px 10px', background: '#eef4fb', color: '#34506b' }}>
              {'Click chip -> input'}
            </Tag>
            <Tag minimal style={{ borderRadius: 999, padding: '6px 10px', background: '#eef8f0', color: '#0f6b43' }}>
              {'Blur id 42 -> Order 42'}
            </Tag>
            <Tag minimal style={{ borderRadius: 999, padding: '6px 10px', background: '#fff7e8', color: '#8a5d00' }}>
              {'Search icon -> dialog path'}
            </Tag>
          </div>
        </div>
        <Card
          elevation={2}
          style={{
            borderRadius: 18,
            padding: 24,
            background: 'rgba(255,255,255,0.96)',
            boxShadow: '0 18px 48px rgba(15, 23, 42, 0.10)',
            border: '1px solid rgba(213, 221, 234, 0.85)',
          }}
        >
          <H5 style={{ marginTop: 0, marginBottom: 14 }}>Composer-style Interaction</H5>
          <NamedLookupInput
            value={value}
            onChange={setValue}
            context={{ handlers: { window: {} } }}
            contextKind="chat-composer"
            contextID="default"
            multiline
            data-testid="lookup-preview-input"
            style={{
              minHeight: 68,
              borderRadius: 16,
              border: '1px solid #d7deea',
              boxShadow: 'inset 0 1px 0 rgba(255,255,255,0.85)',
              background: '#fff',
            }}
          />
          <div
            style={{
              display: 'grid',
              gridTemplateColumns: 'minmax(0, 1fr) minmax(0, 1fr)',
              gap: 12,
              marginTop: 18,
            }}
          >
            <div
              style={{
                borderRadius: 14,
                border: '1px solid #d9e3ee',
                background: '#f8fbff',
                padding: 14,
              }}
            >
              <div style={{ fontSize: 11, fontWeight: 700, letterSpacing: '0.04em', textTransform: 'uppercase', color: '#5f6b7c', marginBottom: 8 }}>
                Stored Value
              </div>
              <code style={{ display: 'block', fontSize: 13, lineHeight: 1.5, whiteSpace: 'pre-wrap', color: '#1f2937' }}>
                {value}
              </code>
            </div>
            <div
              style={{
                borderRadius: 14,
                border: '1px solid #d9e3ee',
                background: '#f8fbff',
                padding: 14,
              }}
            >
              <div style={{ fontSize: 11, fontWeight: 700, letterSpacing: '0.04em', textTransform: 'uppercase', color: '#5f6b7c', marginBottom: 8 }}>
                Sent To Model
              </div>
              <code style={{ display: 'block', fontSize: 13, lineHeight: 1.5, whiteSpace: 'pre-wrap', color: '#1f2937' }}>
                {llmValue}
              </code>
            </div>
          </div>
          <div
            style={{
              marginTop: 12,
              display: 'flex',
              gap: 8,
              flexWrap: 'wrap',
              alignItems: 'center',
            }}
          >
            <span style={{ fontSize: 12, color: '#5f6b7c' }}>Current chip label:</span>
            <Tag minimal style={{ borderRadius: 999, padding: '6px 10px', background: '#eef8f0', color: '#0a6640' }}>
              {parsed[0]?.label || 'n/a'}
            </Tag>
          </div>
        </Card>
      </div>
    </div>
  );
}
