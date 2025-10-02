// ToolFeedBubble.jsx – wrapper bubble that renders ToolFeed for a turn

import React from 'react';
import { Icon } from '@blueprintjs/core';
import ToolFeed from './ToolFeed.jsx';
import CollapsibleCard from './CollapsibleCard.jsx';
import { format as formatDate } from 'date-fns';
import { useExecVisibility } from '../../utils/execFeedBus.js';

export default function ToolFeedBubble({ message: msg, context }) {
  const { toolFeed: showToolFeed } = useExecVisibility();
  const avatarColour = 'var(--green3)';
  const createdISO = (function() { try { const d = new Date(msg.createdAt); return isNaN(d) ? '' : formatDate(d, 'HH:mm'); } catch(_) { return ''; } })();
  const executions = Array.isArray(msg?.toolExecutions) ? msg.toolExecutions : [];
  if (!showToolFeed) return null;
  return (
    <div className={`chat-row tool`}> {/* alignment flex row */}
      <div style={{ display: 'flex', alignItems: 'center' }}>
        <div className="avatar" style={{ background: avatarColour, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
          <span role="img" aria-label="puzzle">🧩</span>
        </div>
        <div className="chat-bubble chat-tool" data-ts={createdISO}>
          <div style={{ marginTop: 8 }}>
            <div style={{ width: '80vw' }}>
            <CollapsibleCard
              title={`Tool Feed (${executions.length})`}
              icon="applications"
              defaultOpen={!!msg.isLastTurn}
              width="100%"
              intent="primary"
            >
              <div style={{ width: '100%', overflowX: 'auto' }}>
                <ToolFeed executions={executions} turnId={msg.turnId} context={context} />
              </div>
            </CollapsibleCard>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
