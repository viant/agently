import { useCallback, useEffect, useRef, useState } from 'react';
import { client } from '../services/agentlyClient';

const native = Object.freeze({ id: 'agently', label: 'Agently', profile: 'agently' });
export function normalizeBackendConnections(entries) {
  const connections = [native];
  const seen = new Set(['agently']);
  for (const entry of Array.isArray(entries) ? entries : []) {
    if (entry?.profile !== 'standard' || typeof entry.id !== 'string' || !entry.id || seen.has(entry.id)) continue;
    seen.add(entry.id);
    connections.push({ ...entry, label: String(entry.label || entry.id) });
  }
  return connections;
}
export function useAgUiBackendSelection(authReady) {
  const [connections, setConnections] = useState([native]);
  const [selectedId, setSelectedId] = useState('agently');
  const [visitedIds, setVisitedIds] = useState([]);
  const generation = useRef(0);
  useEffect(() => {
    let mounted = true;
    const reset = () => {
      generation.current++;
      setConnections([native]); setSelectedId('agently'); setVisitedIds([]);
    };
    const refresh = async () => {
      if (!authReady) return;
      const current = generation.current;
      try {
        const entries = await client.listAgUiBackends();
        if (mounted && current === generation.current) {
          const next = normalizeBackendConnections(entries);
          const allowed = new Set(next.map(entry => entry.id));
          setConnections(next);
          setSelectedId(id => allowed.has(id) ? id : 'agently');
          setVisitedIds(ids => ids.filter(id => allowed.has(id)));
        }
      } catch { /* Existing BFF authentication/error hooks own request failures. */ }
    };
    if (authReady) void refresh(); else reset();
    window.addEventListener('agently:session-reset', reset);
    window.addEventListener('agently:authorized', refresh);
    return () => {
      mounted = false;
      generation.current++;
      window.removeEventListener('agently:session-reset', reset);
      window.removeEventListener('agently:authorized', refresh);
    };
  }, [authReady]);
  const select = useCallback(id => {
    if (!connections.some(entry => entry.id === id)) return;
    setSelectedId(id);
    if (id !== 'agently') setVisitedIds(ids => ids.includes(id) ? ids : [...ids, id]);
  }, [connections]);
  return { connections, selectedId, select, visited: connections.filter(entry => visitedIds.includes(entry.id)), isRemote: selectedId !== 'agently' };
}
