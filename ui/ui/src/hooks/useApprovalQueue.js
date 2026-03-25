import { useEffect, useMemo, useState } from 'react';
import { isConnectivityError } from '../services/networkError';
import { client } from '../services/agentlyClient';

const POLL_MS = 2000;

export function useApprovalQueue() {
  const [items, setItems] = useState([]);
  const [open, setOpen] = useState(false);
  const [selected, setSelected] = useState(null);
  const [error, setError] = useState('');
  const [backendUnavailable, setBackendUnavailable] = useState(false);

  useEffect(() => {
    let timer = null;
    let canceled = false;

    const tick = async () => {
      try {
        const next = await client.listPendingToolApprovals({ status: 'pending' });
        if (!canceled) {
          setItems(next);
          setError('');
          setBackendUnavailable(false);
        }
      } catch (err) {
        if (!canceled) {
          if (err?.status === 401) {
            setItems([]);
            setError('');
            setBackendUnavailable(false);
          } else if (isConnectivityError(err)) {
            setBackendUnavailable(true);
            setError('');
          } else {
            setBackendUnavailable(false);
            setError(String(err?.message || err));
          }
        }
      }
    };

    tick();
    timer = setInterval(tick, POLL_MS);
    return () => {
      canceled = true;
      if (timer) clearInterval(timer);
    };
  }, []);

  const pendingCount = useMemo(() => items.length, [items]);

  const decide = async (item, action) => {
    if (!item?.id) return;
    await client.decideToolApproval(item.id, { action });
    setItems((current) => current.filter((entry) => entry.id !== item.id));
    if (selected?.id === item.id) setSelected(null);
  };

  return {
    items,
    pendingCount,
    open,
    selected,
    error,
    backendUnavailable,
    setOpen,
    setSelected,
    decide
  };
}
