// ToolFeedBubble.jsx – wrapper bubble that renders ToolFeed for a turn

import React from 'react';
import { Icon } from '@blueprintjs/core';
import ToolFeed from './ToolFeed.jsx';
import { format as formatDate } from 'date-fns';
import { useExecVisibility } from '../../utils/execFeedBus.js';

export default function ToolFeedBubble({ message: msg, context }) {
  const { toolFeed: showToolFeed } = useExecVisibility();
  const avatarColour = 'var(--orange3)';
  const createdISO = (function() { try { const d = new Date(msg.createdAt); return isNaN(d) ? '' : formatDate(d, 'HH:mm'); } catch(_) { return ''; } })();
  const executions = Array.isArray(msg?.toolExecutions) ? msg.toolExecutions : [];
  if (!showToolFeed) return null;
  return (
    <div className={`chat-row tool`}> {/* alignment flex row */}
      <div style={{ display: 'flex', alignItems: 'flex-start' }}>
        <div className="avatar" style={{ background: avatarColour }}>
          <Icon icon={'wrench'} color="var(--black)" size={12} />
        </div>
        <div className="chat-bubble chat-tool" data-ts={createdISO}>
          <ToolFeed executions={executions} turnId={msg.turnId} context={context} />
        </div>
      </div>
    </div>
  );
}
