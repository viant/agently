import {progressStatusPresentation} from '../services/progressPresentation.js';
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
  attention: '●',
  terminated: '■',
  offline: '●'
};

const ELAPSED_PHASES = new Set(['waiting', 'thinking', 'executing', 'streaming']);

function formatElapsed(ms = 0) {
  const sec = Math.max(0, Number(ms || 0)) / 1000;
  if (sec < 10) return `${sec.toFixed(1)}s`;
  return `${Math.round(sec)}s`;
}

export default function StatusBar({ backendUnavailable = false, approvals = null, developerMode = false, hidden = false }) {
  const stage = useStage();
  const [now, setNow] = React.useState(Date.now());
  const {phase, text} = progressStatusPresentation(stage, developerMode, backendUnavailable);
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

  const browserLifecycle = globalThis.performance?.getEntriesByName('agently.browserMCP.lifecycle.settled')?.at(-1)?.detail;
  const browserAliases = globalThis.performance?.getEntriesByName('agently.browserMCP.lifecycle.aliases')?.at(-1)?.detail;
  const browserContinuation = globalThis.performance?.getEntriesByName('agently.browserMCP.lifecycle.continuation')?.at(-1)?.detail;
  const browserCloseCause = globalThis.performance?.getEntriesByName('agently.browserMCP.lifecycle.close')?.at(-1)?.detail?.cause;
  const browserLifecycleLabel = browserLifecycle
    ? `phase:${browserLifecycle.phase} native:${browserLifecycle.nativeIdentity ? 1 : 0} tools:${browserLifecycle.browserTools ? 1 : 0} retained:${browserLifecycle.retained ? 1 : 0}`
    : 'unsettled';

  return (
    <footer className={`app-statusbar phase-${phase}`} style={hidden ? { display: 'none' } : undefined} role="status" aria-label={import.meta.env.DEV ? `Verification ${globalThis.location?.pathname?.match(/\/(?:v1\/|ui\/)?conversation\/([a-f0-9-]{36})(?:\/)?$/)?.[1] || "no-conversation"} | aliases p:${browserAliases?.pendingCount ?? 0} exact:${browserAliases?.exactCount ?? 0} canon:${browserAliases?.canonicalCount ?? 0} slash:${browserAliases?.slashCount ?? 0} dash:${browserAliases?.dashCount ?? 0} | failure:${browserContinuation?.failure || "none"} stage:${browserContinuation?.stage || "none"} pending:${browserContinuation?.pendingCount ?? 0} interrupts:${browserContinuation?.interruptCount ?? 0} | ${browserLifecycleLabel} close:${browserCloseCause || "none"}` : undefined} aria-live="polite">
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
