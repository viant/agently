import React, { useEffect, useState } from 'react';

export function elicitationDeadline(source = {}) {
  const sources = [source.elicitation, source].filter(Boolean);
  for (const item of sources) {
    for (const key of ['deadline', 'expiresAt', 'timeoutAt']) {
      const raw = item[key];
      if (raw == null || raw === '') continue;
      const value = typeof raw === 'number' ? raw : Date.parse(raw);
      if (Number.isFinite(value)) return value;
    }
    const created = Date.parse(item.createdAt || '');
    const duration = Number(item.timeoutMs);
    if (Number.isFinite(created) && duration > 0) return created + duration;
  }
  return null;
}

export function elicitationTimingText(deadline, now = Date.now()) {
  if (deadline == null) return '';
  const seconds = Math.ceil((deadline - now) / 1000);
  if (seconds <= 0) return 'Taking longer? You can still answer or skip.';
  const minutes = Math.floor(seconds / 60);
  return `Agent wait time: ${minutes}:${String(seconds % 60).padStart(2, '0')}. You can still answer afterward.`;
}

// Keep the clock inside this component so ticks never reset the form values.
export default function ElicitationTiming({ source }) {
  const deadline = elicitationDeadline(source);
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    setNow(Date.now());
    if (deadline == null || deadline <= Date.now()) return;
    const timer = setInterval(() => {
      const current = Date.now();
      setNow(current);
      if (current >= deadline) clearInterval(timer);
    }, 1000);
    return () => clearInterval(timer);
  }, [deadline]);
  const text = elicitationTimingText(deadline, now);
  return text ? <p role="status" aria-live="off" style={{ marginTop: 12, marginBottom: 0 }}>{text}</p> : null;
}
