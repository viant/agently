import React from 'react';
import { useStage } from '../services/stageBus';

const PHASE_ICON = {
  ready: '●',
  waiting: '⏳',
  thinking: '🤔',
  executing: '⚙',
  streaming: '✍',
  done: '✓',
  error: '⚠',
  terminated: '■',
  offline: '●'
};

const ELAPSED_PHASES = new Set(['waiting', 'thinking', 'executing', 'streaming']);

function formatElapsed(ms = 0) {
  const sec = Math.max(0, Number(ms || 0)) / 1000;
  if (sec < 10) return `${sec.toFixed(1)}s`;
  return `${Math.round(sec)}s`;
}

export default function StatusBar({ backendUnavailable = false, approvals = null }) {
  const stage = useStage();
  const [now, setNow] = React.useState(Date.now());
  const phase = backendUnavailable ? 'offline' : String(stage?.phase || 'ready');
  const text = backendUnavailable
    ? 'Service temporarily unavailable. Reconnecting...'
    : String(stage?.text || 'Ready');
  const pendingApprovals = Number(approvals?.pendingCount || 0);
  const isElapsedActive = !backendUnavailable && ELAPSED_PHASES.has(phase);
  const startedAt = Number(stage?.startedAt || 0);
  const completedAt = Number(stage?.completedAt || 0);
  const elapsedMs = isElapsedActive
    ? Math.max(0, (startedAt || now) ? now - (startedAt || now) : 0)
    : (startedAt && completedAt && completedAt >= startedAt ? (completedAt - startedAt) : 0);

  React.useEffect(() => {
    if (!isElapsedActive) return undefined;
    const timer = window.setInterval(() => setNow(Date.now()), 200);
    return () => window.clearInterval(timer);
  }, [isElapsedActive, phase, stage?.updatedAt]);

  return (
    <footer className={`app-statusbar phase-${phase}`}>
      <div className="app-statusbar-main">
        <span className="app-statusbar-icon">{PHASE_ICON[phase] || '●'}</span>
        <span className="app-statusbar-text">
          {text}
          {isElapsedActive ? ` ${formatElapsed(elapsedMs)}` : ''}
        </span>
      </div>
      <div className="app-statusbar-right">
        {pendingApprovals > 0 ? <span className="app-status-chip">Approvals {pendingApprovals}</span> : null}
      </div>
    </footer>
  );
}
