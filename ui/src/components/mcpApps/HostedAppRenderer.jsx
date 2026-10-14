import React, { useEffect, useRef, useState } from 'react';
import { AppBridge, PostMessageTransport } from '@modelcontextprotocol/ext-apps/app-bridge';
import AppFrame from './AppFrame.jsx';
import { readMCPUIResource, resolveMCPUIFrameConfig } from '../../services/mcpApps/resourceLoader.js';

// The opened-window context goes only to the server's ordinary read DTO.
// Parent identity, credentials and resource proofs never enter the guest frame.
export default function HostedAppRenderer({ uri, title = 'Hosted app', resourceContext }) {
  const frame = useRef(null);
  const [frameLoaded, setFrameLoaded] = useState(false);
  const [state, setState] = useState({ loading: true, error: '', src: '', sandbox: '' });
  const selectionKey = JSON.stringify(resourceContext);

  useEffect(() => {
    let alive = true;
    setFrameLoaded(false);
    setState({ loading: true, error: '', src: '', sandbox: '' });
    void (async () => {
      try {
        const payload = await readMCPUIResource(uri, globalThis.fetch, resourceContext);
        if (!alive) return;
        const config = resolveMCPUIFrameConfig(payload);
        if (!config.rendererUrl || config.sandbox !== 'allow-scripts allow-same-origin allow-forms allow-popups') {
          throw new Error('An installed immutable hosted app URL is required');
        }
        setFrameLoaded(false);
        setState({ loading: true, error: '', src: config.rendererUrl, sandbox: config.sandbox });
      } catch (cause) {
        if (alive) setState({ loading: false, error: String(cause?.message || cause), src: '', sandbox: '' });
      }
    })();
    return () => { alive = false; };
  }, [uri, selectionKey]);

  useEffect(() => {
    const target = frame.current?.contentWindow;
    if (!frameLoaded || !state.src || !target) return undefined;
    let alive = true;
    const bridge = new AppBridge(null, { name: 'Agently', version: '1' }, {}, {
      hostContext: { displayMode: 'inline', availableDisplayModes: ['inline'] },
    });
    bridge.oninitialized = async () => {
      if (!alive) return;
      try {
        await bridge.sendToolInput({ arguments: {} });
        if (alive) setState(previous => ({ ...previous, loading: false }));
      } catch (cause) {
        if (alive) setState(previous => ({ ...previous, loading: false, error: String(cause?.message || cause) }));
      }
    };
    void bridge.connect(new PostMessageTransport(target, target)).catch(cause => {
      if (alive) setState(previous => ({ ...previous, loading: false, error: String(cause?.message || cause) }));
    });
    return () => { alive = false; void bridge.close(); };
  }, [frameLoaded, state.src]);

  return <div style={{ display: 'flex', flex: 1, minHeight: 0, flexDirection: 'column' }}>
    {state.loading && !state.error ? <div role="status">Connecting app…</div> : null}
    {state.error ? <div role="alert">{state.error}</div> : null}
    <AppFrame key={state.src || 'unloaded'} ref={frame} src={state.src || undefined}
      sandbox={state.sandbox || 'allow-scripts'} title={title}
      onLoad={() => { if (state.src) setFrameLoaded(true); }}
      style={{ flex: 1, minHeight: 480, width: '100%' }} />
  </div>;
}
