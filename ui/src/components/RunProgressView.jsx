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

export default function RunProgressView({activity = 'Working on your request', narration, startedAt, progressKey, summary, waiting = false, stopping = false, onStop, error}) {
  const [now, setNow] = React.useState(Date.now);
  const [lastUpdate, setLastUpdate] = React.useState(Date.now);
  const start = Date.parse(startedAt || '');
  React.useEffect(() => {
    setLastUpdate(Date.now());
  }, [progressKey, activity, narration, startedAt, waiting, stopping]);
  React.useEffect(() => {
    if (waiting) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [waiting]);
  const seconds = Number.isFinite(start) ? Math.max(0, Math.floor((now - start) / 1000)) : null;
  const quietSeconds = Math.max(0, Math.floor((now - lastUpdate) / 1000));
  const quiet = !waiting && !stopping && quietSeconds >= 30;
  const clock = value => `${Math.floor(value / 60)}:${String(value % 60).padStart(2, '0')}`;
  return <section className="app-run-progress" data-testid="turn-progress-status" aria-label="Request progress">
    <div className="app-run-progress-heading">
      <span className="app-run-progress-activity" role="status" aria-live="polite">
        <Icon name={waiting ? 'lucide:circle-help' : 'lucide:loader-circle'} className={waiting ? '' : 'app-run-progress-spinner'} aria-hidden="true"/>
        <Typography typeRole="label">{stopping ? 'Stopping request' : waiting ? 'Needs your input' : quiet ? 'Waiting for an update' : activity}</Typography>
      </span>
      {seconds !== null ? <Typography typeRole="metadata" aria-live="off">{clock(seconds)} elapsed</Typography> : null}
      {onStop ? <Button variant="ghost" size="compact" icon="lucide:square" disabled={stopping} onClick={onStop}>Stop request</Button> : null}
    </div>
    {narration ? <Typography as="p" typeRole="body" data-testid="turn-progress-narration">{narration}</Typography> : null}
    {summary ? <Typography as="p" typeRole="metadata" className="app-run-progress-meta" role="status" data-testid="turn-progress-summary">{summary}</Typography> : null}
    {quiet ? <Typography as="p" typeRole="metadata" className="app-run-progress-meta" aria-live="off">No new progress for {clock(quietSeconds)}. {onStop ? 'You can wait or stop this request.' : 'Waiting for the service to respond.'}</Typography>
      : stopping ? <Typography as="p" typeRole="metadata" className="app-run-progress-meta">Waiting for confirmation that the request has stopped.</Typography>
      : !waiting && seconds >= 15 ? <Typography as="p" typeRole="metadata" className="app-run-progress-meta">You can leave this chat and return later.</Typography> : null}
    {error ? <Typography as="p" typeRole="body" role="alert">{error}</Typography> : null}
  </section>;
}
