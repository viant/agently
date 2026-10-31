import React from 'react';
import {Button, Icon, Typography, typographyProps} from 'forge/components';
import {renderMarkdownBlock} from 'agently-core-ui-sdk';
import './requestLifecycle.css';

export function RunActivity({appearance, progress = null, children}) {
  if (appearance !== 'document') return <>{progress}{children}</>;
  return <section className="app-run-activity" aria-label="Request activity" data-has-progress={!!progress}>
    {progress}{children}
  </section>;
}

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

export function workDurationLabel(seconds) {
  if (!Number.isFinite(seconds)) return 'Work completed';
  const total = Math.max(0, Math.floor(seconds));
  const hours = Math.floor(total / 3600), minutes = Math.floor(total % 3600 / 60), rest = total % 60;
  return `Worked for ${[hours ? `${hours}h` : '', minutes ? `${minutes}m` : '', rest || !hours && !minutes ? `${rest}s` : ''].filter(Boolean).join(' ')}`;
}

export default function RunProgressView({activity = 'Working on your request', narration, startedAt, progressKey, summary, waiting = false, stopping = false, completed = false, durationSeconds, onToggleDetails, expanded = false, controls, onStop, error}) {
  const [now, setNow] = React.useState(Date.now);
  const [lastUpdate, setLastUpdate] = React.useState(Date.now);
  const start = Date.parse(startedAt || '');
  React.useEffect(() => {
    setLastUpdate(Date.now());
  }, [progressKey, activity, narration, startedAt, waiting, stopping]);
  React.useEffect(() => {
    if (waiting || completed) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [waiting, completed]);
  const seconds = Number.isFinite(durationSeconds) ? durationSeconds : Number.isFinite(start) ? Math.max(0, Math.floor((now - start) / 1000)) : null;
  const quietSeconds = Math.max(0, Math.floor((now - lastUpdate) / 1000));
  const quiet = !waiting && !stopping && !completed && quietSeconds >= 30;
  const clock = value => `${Math.floor(value / 60)}:${String(value % 60).padStart(2, '0')}`;
  const Status = onToggleDetails ? 'button' : 'div';
  return <section className={`app-run-progress${completed ? ' is-completed' : ''}`} data-testid={completed ? 'turn-completed-status' : 'turn-progress-status'} aria-label={completed ? 'Request complete' : 'Request progress'}>
    <div className="app-run-progress-heading">
      <Status className={`app-run-progress-toggle${onToggleDetails ? ' app-iteration-head' : ''}`} {...typographyProps(completed ? 'metadata' : 'label')}
        {...(onToggleDetails ? {type:'button','aria-label':completed ? workDurationLabel(seconds) : `${stopping ? 'Stopping request' : waiting ? 'Needs your input' : activity}. ${expanded ? 'Hide' : 'Show'} working steps`,'aria-expanded':expanded,'aria-controls':controls,onClick:onToggleDetails} : {})}>
      <span className="app-run-progress-activity" role="status" aria-live="polite">
        {!completed ? <Icon name={waiting ? 'lucide:circle-help' : 'lucide:loader-circle'} className={waiting ? '' : 'app-run-progress-spinner'} aria-hidden="true"/> : null}
        <Typography typeRole={completed ? 'metadata' : 'label'}>{completed ? workDurationLabel(seconds) : stopping ? 'Stopping request' : waiting ? 'Needs your input' : activity}</Typography>
      </span>
      {!completed && seconds !== null ? <Typography typeRole="metadata" aria-label={`${clock(seconds)} elapsed`} aria-live="off">{clock(seconds)}</Typography> : null}
      {onToggleDetails ? <Icon name={expanded ? 'lucide:chevron-down' : 'lucide:chevron-right'} aria-hidden="true"/> : null}
      </Status>
      {onStop && !completed ? <Button variant="ghost" size="compact" icon="lucide:square" disabled={stopping} onClick={onStop}>Stop request</Button> : null}
    </div>
    {!completed && narration && (!onToggleDetails || expanded) ? <Typography as="div" typeRole="body" className="app-rich-prose" data-testid="turn-progress-narration" dangerouslySetInnerHTML={{__html:renderMarkdownBlock(narration)}}/> : null}
    {!completed && summary && (!onToggleDetails || expanded) ? <Typography as="p" typeRole="metadata" className="app-run-progress-meta" role="status" data-testid="turn-progress-summary">{summary}</Typography> : null}
    {quiet ? <Typography as="p" typeRole="metadata" className="app-run-progress-meta" aria-live="off">No new updates for {clock(quietSeconds)}. {onStop ? 'You can wait or stop this request.' : 'Waiting for the service to respond.'}</Typography>
      : stopping ? <Typography as="p" typeRole="metadata" className="app-run-progress-meta">Waiting for confirmation that the request has stopped.</Typography>
      : null}
    {error ? <Typography as="p" typeRole="body" role="alert">{error}</Typography> : null}
  </section>;
}
