import React from 'react';
import {Button, Icon, Typography} from 'forge/components';
import './requestLifecycle.css';

export function RunNotice({title, message, tone = 'neutral', children, ...props}) {
  return <section className="app-run-notice" data-tone={tone} role={tone === 'danger' ? 'alert' : 'status'} {...props}>
    <Icon name={tone === 'danger' ? 'lucide:circle-alert' : 'lucide:circle-pause'} aria-hidden="true"/>
    <div className="app-run-notice-content">
      <Typography as="p" typeRole="label">{title}</Typography>
      {message ? <Typography as="p" typeRole="body">{message}</Typography> : null}
      {children ? <div className="app-run-notice-actions">{children}</div> : null}
    </div>
  </section>;
}

export default function RunProgressView({activity = 'Working', narration, startedAt, waiting = false, stopping = false, onStop, error}) {
  const [now, setNow] = React.useState(Date.now);
  const start = Date.parse(startedAt || '');
  React.useEffect(() => {
    if (!Number.isFinite(start)) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [start]);
  const seconds = Number.isFinite(start) ? Math.max(0, Math.floor((now - start) / 1000)) : null;
  return <section className="app-run-progress" data-testid="turn-progress-status" aria-label="Request progress">
    <div className="app-run-progress-heading" role="status" aria-live="polite">
      <Icon name={waiting ? 'lucide:circle-help' : 'lucide:loader-circle'} aria-hidden="true"/>
      <Typography typeRole="label">{stopping ? 'Stopping request' : waiting ? 'Needs your input' : activity}</Typography>
    </div>
    {narration ? <Typography as="p" typeRole="body" data-testid="turn-progress-narration">{narration}</Typography> : null}
    <div className="app-run-progress-meta">
      {seconds !== null ? <Typography typeRole="metadata" aria-live="off">{Math.floor(seconds / 60)}:{String(seconds % 60).padStart(2, '0')} elapsed</Typography> : null}
      {!waiting ? <Typography typeRole="metadata">You can leave this chat while work continues.</Typography> : null}
      {onStop ? <Button variant="ghost" size="compact" icon="lucide:square" disabled={stopping} onClick={onStop}>Stop request</Button> : null}
    </div>
    {error ? <Typography as="p" typeRole="body" role="alert">{error}</Typography> : null}
  </section>;
}
