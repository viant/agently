import React, {lazy, Suspense} from 'react';
import {Typography} from 'forge/components';

const AuditWindow = lazy(() => import('./ConversationAuditWindow.jsx'));

export const isConversationAuditWindow = (entry = {}) => entry.windowKey === 'conversation-audit';

export default function ConversationAuditDestination() {
  return <Suspense fallback={<Typography as="p" typeRole="body">Loading conversation audit…</Typography>}><AuditWindow/></Suspense>;
}
