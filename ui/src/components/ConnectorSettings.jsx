import React, { useCallback, useEffect, useState } from 'react';
import { Button } from '@blueprintjs/core';
import { connectProvider, disconnectProvider, finishConnectorReturn, listConnectors } from '../services/connectors';

export default function ConnectorSettings() {
  const [rows, setRows] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [busy, setBusy] = useState('');
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    const reset = () => { setNotice(''); setRows([]); setRevision(value => value + 1); };
    window.addEventListener('agently:session-reset', reset);
    return () => window.removeEventListener('agently:session-reset', reset);
  }, []);
  useEffect(() => {
    let active = true;
    setRows([]);
    setLoading(true);
    setError('');
    (async () => {
      try {
        const connections = await listConnectors();
        // A previous interrupted tool flow must not prevent managing connectors.
        await finishConnectorReturn().catch(() => null);
        if (active) setRows(connections);
      } catch (e) {
        if (active) setError(e.message || 'Connections could not be loaded. Try again.');
      } finally { if (active) setLoading(false); }
    })();
    return () => { active = false; };
  }, [revision]);
  const connect = useCallback(async (server) => {
    setBusy(server);
    setNotice('');
    setError('');
    try {
      const result = await connectProvider(server);
      if (result?.status === 'connected') setRevision(value => value + 1);
      else if (result?.status !== 'connect') setError('Authorization is already in progress. Complete it, then refresh.');
    } catch (_) { setError('Could not start this connection. Try again or ask your workspace administrator to check its configuration.'); }
    finally { setBusy(''); }
  }, []);
  const disconnect = useCallback(async (server) => {
    setBusy(server);
    setNotice('');
    setError('');
    try {
      setRows(await disconnectProvider(server));
      setNotice('Disconnected. The next tool call that needs this provider will ask you to connect again.');
    } catch (e) { setError(e.message || 'Could not disconnect. Try again.'); }
    finally { setBusy(''); }
  }, []);
  return <section aria-labelledby="connectors-heading">
    <div className="app-connectors-heading">
      <div><h2 id="connectors-heading" className="app-ui-settings-title">Connectors</h2>
        <p>Connect your accounts once. Agently uses your saved connection when a tool needs it.</p></div>
      <Button minimal icon="refresh" text="Refresh" disabled={loading || !!busy} onClick={() => setRevision(value => value + 1)} />
    </div>
    {notice && <p role="status">{notice}</p>}
    {error && <p role="alert" className="app-connectors-error">{error}</p>}
    {loading ? <p role="status">Loading connections…</p> : !error && rows.length === 0 ?
      <p>No connectors are configured for this workspace.</p> :
      <ul className="app-connectors-list">{rows.map(row => <li key={`${row.provider}:${row.server || ''}`}>
        <div><h3>{row.provider}</h3>{row.server && row.server !== row.provider && <p>{row.server}</p>}
          <p className="app-connectors-status">{!row.available ? row.reason || 'Unavailable' : row.connected ? 'Connected' : row.pending ? 'Authorization in progress' : 'Not connected'}</p></div>
        {row.available && row.connected && <Button text={busy === row.server ? 'Disconnecting…' : 'Disconnect'} disabled={!!busy} onClick={() => disconnect(row.server)} aria-label={`Disconnect ${row.provider}`} />}
        {row.available && !row.connected && <Button text={busy === row.server ? 'Connecting…' : row.pending ? 'Continue connection' : 'Connect'} disabled={!!busy} onClick={() => connect(row.server)} aria-label={`Connect ${row.provider}`} />}
      </li>)}</ul>}
    <p className="app-connectors-note">Connections belong to your user account. Access follows your permissions in each service. Disconnect removes the saved connection; your next tool call will request authorization again.</p>
  </section>;
}
