import React from 'react';
import {Dialog, Button as LegacyButton} from '@blueprintjs/core';
import {Button, Typography, ForgeThemeBoundary} from 'forge/components';
import './requestLifecycle.css';

export function reviewDecisionError(error) {
  return Number(error?.status) === 403
    ? 'You no longer have access to decide this request. Check your permissions.'
    : 'The decision couldn’t be confirmed. Your edits are kept. Check its status before retrying.';
}

export function ReviewAction({appearance, intent, minimal, small, icon, children, ...props}) {
  if (appearance !== 'document') return <LegacyButton intent={intent} minimal={minimal} small={small} icon={icon} {...props}>{children}</LegacyButton>;
  return <Button variant={intent === 'primary' ? 'primary' : intent === 'danger' ? 'danger' : minimal ? 'ghost' : 'secondary'}
    size={small ? 'compact' : 'default'} {...props}>{children}</Button>;
}

export default function RequestReviewDialog({appearance, busy = false, title, children, ...props}) {
  if (appearance !== 'document') return <Dialog title={title} {...props}>{children}</Dialog>;
  return <Dialog {...props} className="app-request-review-dialog" style={undefined}
    canEscapeKeyClose={!busy} canOutsideClickClose={!busy} isCloseButtonShown={false}
    title={<div className="app-request-review-title"><Typography typeRole="section">{title}</Typography>
      <Button variant="ghost" size="compact" icon="lucide:x" hideLabel aria-label="Close review" disabled={busy} onClick={props.onClose}/></div>}>
    <ForgeThemeBoundary presentation="document" windowKey="request-review">{children}</ForgeThemeBoundary>
  </Dialog>;
}
