import { useEffect, useMemo, useRef, useState } from 'react';
import { isConnectivityError } from '../services/networkError';
import { client } from '../services/agentlyClient';
import { dispatchMCPUIApprovalOutcome, normalizeMCPUIApprovalOutcome } from '../services/mcpApps/approvalEvents.js';

const POLL_MS = 2000;
const PAGE_SIZE = 8;

export function shouldPollApprovalQueue(enabled = true, visibilityState = 'visible', hasWindowFocus = true, isOpen = false, hasActiveSelection = false) {
  return Boolean(enabled) && visibilityState === 'visible' && Boolean(hasWindowFocus) && (Boolean(isOpen) || Boolean(hasActiveSelection));
}

export function resolveApprovalDecisionOutcome(output = null) {
  return normalizeMCPUIApprovalOutcome(output?.outcome || null);
}

// dispatchApprovalDecisionOutcomes forwards every canonical
// DecideToolApprovalOutcome carried by a pending-approvals page to
// the MCP UI approval event bus. The dispatcher is invoked once per
// outcome per poll. The backend may re-emit the same canonical
// outcome on consecutive polls whose carried OutcomeSince cursor
// predates the row's terminal transition — that durability is what
// lets a client which polled just after the transition moment still
// observe the outcome.
export function dispatchApprovalDecisionOutcomes(page = null, dispatcher = dispatchMCPUIApprovalOutcome) {
  const outcomes = Array.isArray(page?.outcomes) ? page.outcomes : [];
  outcomes.forEach((outcome) => {
    const normalized = resolveApprovalDecisionOutcome({ outcome });
    if (normalized) {
      dispatcher(normalized);
    }
  });
}

// pickNextOutcomeCursor selects the cursor the client should send on
// its next poll. Cursor advancement is purely a transport concern:
// the UI never invents or interprets the cursor shape, it just
// stores whatever non-empty cursor the backend returned and echoes
// it back. An empty/missing cursor in the response leaves the prior
// cursor in place so the next poll can still re-emit any outcome
// the client has not yet seen.
export function pickNextOutcomeCursor(page = null, current = '') {
  const next = page && typeof page.outcomeCursor === 'string' ? page.outcomeCursor.trim() : '';
  return next !== '' ? next : (typeof current === 'string' ? current : '');
}

export function useApprovalQueue(enabled = true) {
  const [items, setItems] = useState([]);
  const [open, setOpen] = useState(false);
  const [selected, setSelected] = useState(null);
  const [error, setError] = useState('');
  const [backendUnavailable, setBackendUnavailable] = useState(false);
  const [page, setPage] = useState(0);
  const [total, setTotal] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [outcomeCursor, setOutcomeCursor] = useState('');
  // outcomeCursorRef carries the latest cursor across polling ticks
  // without re-triggering the effect every time the cursor advances.
  // The state above only exists so consumers/tests can read the
  // current cursor; the live polling loop reads the ref to avoid an
  // effect restart per cursor advance.
  const outcomeCursorRef = useRef('');
  const [visibilityState, setVisibilityState] = useState(() => {
    if (typeof document === 'undefined') return 'visible';
    return document.visibilityState === 'hidden' ? 'hidden' : 'visible';
  });
  const [hasWindowFocus, setHasWindowFocus] = useState(() => {
    if (typeof document === 'undefined' || typeof document.hasFocus !== 'function') return true;
    return document.hasFocus();
  });

  useEffect(() => {
    if (typeof document === 'undefined' || typeof window === 'undefined') return () => {};

    const syncVisibility = () => {
      setVisibilityState(document.visibilityState === 'hidden' ? 'hidden' : 'visible');
    };
    const onFocus = () => setHasWindowFocus(true);
    const onBlur = () => setHasWindowFocus(false);

    syncVisibility();
    if (typeof document.hasFocus === 'function') {
      setHasWindowFocus(document.hasFocus());
    }

    document.addEventListener('visibilitychange', syncVisibility);
    window.addEventListener('focus', onFocus);
    window.addEventListener('blur', onBlur);

    return () => {
      document.removeEventListener('visibilitychange', syncVisibility);
      window.removeEventListener('focus', onFocus);
      window.removeEventListener('blur', onBlur);
    };
  }, []);

  useEffect(() => {
    if (!enabled) {
      setItems([]);
      setError('');
      setBackendUnavailable(false);
      setPage(0);
      setTotal(0);
      setHasMore(false);
      setOutcomeCursor('');
      outcomeCursorRef.current = '';
      return () => {};
    }
    const hasActiveSelection = !!selected?.id;
    const pollEnabled = shouldPollApprovalQueue(enabled, visibilityState, hasWindowFocus, open, hasActiveSelection);
    if (!pollEnabled && visibilityState !== 'visible') {
      return () => {};
    }

    let timer = null;
    let canceled = false;

    const tick = async () => {
      try {
        const next = await client.listPendingToolApprovalsPage({
          status: 'pending',
          limit: PAGE_SIZE,
          offset: page * PAGE_SIZE,
          outcomeSince: outcomeCursorRef.current || undefined,
        });
        if (!canceled) {
          dispatchApprovalDecisionOutcomes(next);
          const advanced = pickNextOutcomeCursor(next, outcomeCursorRef.current);
          if (advanced !== outcomeCursorRef.current) {
            outcomeCursorRef.current = advanced;
            setOutcomeCursor(advanced);
          }
          const nextRows = Array.isArray(next?.rows) ? next.rows : [];
          const nextTotal = Number(next?.total || 0) || 0;
          if (selected?.id) {
            const resolved = Array.isArray(next?.outcomes) ? next.outcomes.find((entry) => String(entry?.approvalId || '').trim() === String(selected.id || '').trim()) : null;
            if (resolved || !nextRows.find((entry) => entry?.id === selected.id)) {
              setSelected(null);
            }
          }
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
      if (!canceled && pollEnabled) {
        timer = window.setTimeout(tick, POLL_MS);
      }
    };

    tick();
    return () => {
      canceled = true;
      if (timer) window.clearTimeout(timer);
    };
  }, [enabled, open, selected?.id, page, visibilityState, hasWindowFocus]);

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
    const output = await client.decideToolApproval(item.id, payload);
    const outcome = resolveApprovalDecisionOutcome(output);
    if (outcome) {
      dispatchMCPUIApprovalOutcome(outcome);
    }
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
    outcomeCursor,
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
