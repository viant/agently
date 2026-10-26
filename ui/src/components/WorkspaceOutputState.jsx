import React from 'react';
import {Button, Typography} from 'forge/components';

export default function WorkspaceOutputState({loading = false, error, onRetry, onReturn}) {
  const status = Number(error?.status || 0);
  const renderFailure = error?.code === 'render';
  const title = loading ? 'Loading output…' : status === 403 ? 'You don’t have access to this output' : status === 404 ? 'This output is unavailable' : renderFailure ? 'This output couldn’t be displayed' : 'This output couldn’t load';
  const detail = status === 403 ? 'Ask the owner for access, then try again.' : status === 404 ? 'It may have been removed, or access may have changed. You can check again or return to the conversation.' : renderFailure ? 'Try again, or return to the conversation. If this continues, ask the owner to check the output.' : 'Check your connection and try again. Your conversation and draft are still here.';
  return <div className="app-workspace-output-state" aria-busy={loading || undefined}>
    <div role={loading ? 'status' : 'alert'}>
      <Typography as="h3" typeRole="section">{title}</Typography>
      {!loading ? <Typography as="p" typeRole="body">{detail}</Typography> : null}
    </div>
    {!loading ? <div className="app-workspace-output-state-actions">
      {onRetry ? <Button variant="secondary" onClick={onRetry}>Try again</Button> : null}
      {onReturn ? <Button variant="ghost" onClick={onReturn}>Return to conversation</Button> : null}
    </div> : null}
  </div>;
}
