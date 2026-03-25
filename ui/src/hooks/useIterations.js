import { useMemo, useState } from 'react';
import { groupIntoIterations } from '../services/messageNormalizer';

export function useIterations(items = [], initialVisible = 5, step = 5) {
  const [visibleCount, setVisibleCount] = useState(initialVisible);

  const state = useMemo(() => {
    const all = groupIntoIterations(items);
    const iterations = all.filter((item) => item.type === 'iteration');
    const others = all.filter((item) => item.type !== 'iteration');
    const start = Math.max(iterations.length - visibleCount, 0);
    const visibleIterations = iterations.slice(start);
    const hiddenCount = start;
    return { visibleIterations, others, hiddenCount, total: iterations.length };
  }, [items, visibleCount]);

  const loadMore = () => setVisibleCount((v) => v + step);
  const loadAll = () => setVisibleCount(Number.MAX_SAFE_INTEGER);
  const reset = () => setVisibleCount(initialVisible);

  return {
    visible: [...state.others, ...state.visibleIterations],
    hiddenCount: state.hiddenCount,
    hasHidden: state.hiddenCount > 0,
    loadMore,
    loadAll,
    reset
  };
}
