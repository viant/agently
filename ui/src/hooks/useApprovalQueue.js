import { useEffect, useMemo, useState } from 'react';
import { isConnectivityError } from '../services/networkError';
import { client } from '../services/agentlyClient';

const POLL_MS = 2000;
const PAGE_SIZE = 8;

export function useApprovalQueue(enabled = true) {
  const [items, setItems] = useState([]);
  const [open, setOpen] = useState(false);
  const [selected, setSelected] = useState(null);
  const [error, setError] = useState('');
  const [backendUnavailable, setBackendUnavailable] = useState(false);
  const [page, setPage] = useState(0);
  const [total, setTotal] = useState(0);
  const [hasMore, setHasMore] = useState(false);

  useEffect(() => {
    if (!enabled) {
      setItems([]);
      setError('');
      setBackendUnavailable(false);
      setPage(0);
      setTotal(0);
      setHasMore(false);
      return () => {};
    }

    let timer = null;
    let canceled = false;

    const tick = async () => {
      try {
        const next = await client.listPendingToolApprovalsPage({
          status: 'pending',
          limit: PAGE_SIZE,
          offset: page * PAGE_SIZE
        });
        if (!canceled) {
          const nextRows = Array.isArray(next?.rows) ? next.rows : [];
          const nextTotal = Number(next?.total || 0) || 0;
          const maxPage = Math.max(0, Math.ceil(nextTotal / PAGE_SIZE) - 1);
          if (page > maxPage) {
            setPage(maxPage);
            return;
          }
          setItems(nextRows);
          setTotal(nextTotal);
          setHasMore(Boolean(next?.hasMore));
          setError('');
          setBackendUnavailable(false);
        }
      } catch (err) {
        if (!canceled) {
          if (err?.status === 401) {
            setItems([]);
            setError('');
            setBackendUnavailable(false);
            setTotal(0);
            setHasMore(false);
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
  }, [enabled, page]);

  const pendingCount = useMemo(() => total, [total]);
  const pageCount = useMemo(() => Math.max(1, Math.ceil((Number(total) || 0) / PAGE_SIZE)), [total]);
  const start = useMemo(() => (items.length === 0 ? 0 : (page * PAGE_SIZE) + 1), [items.length, page]);
  const end = useMemo(() => Math.min((page * PAGE_SIZE) + items.length, total), [items.length, page, total]);

  const decide = async (item, action, editedFields = null) => {
    if (!item?.id) return;
    const payload = { action };
    if (editedFields && typeof editedFields === 'object' && Object.keys(editedFields).length > 0) {
      payload.editedFields = editedFields;
    }
    await client.decideToolApproval(item.id, payload);
    setItems((current) => current.filter((entry) => entry.id !== item.id));
    setTotal((current) => Math.max(0, current - 1));
    if (selected?.id === item.id) setSelected(null);
  };

  return {
    items,
    pendingCount,
    page,
    pageCount,
    start,
    end,
    hasPrevious: page > 0,
    hasNext: hasMore || page < pageCount - 1,
    open,
    selected,
    error,
    backendUnavailable,
    setOpen,
    setPage,
    setSelected,
    decide
  };
}
