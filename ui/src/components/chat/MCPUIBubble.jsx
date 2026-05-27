import React from 'react';
import { AvatarIcon } from 'forge/components';
import AppRenderer from '../mcpApps/AppRenderer.jsx';

export default function MCPUIBubble({ row }) {
  const title = String(row?.toolName || 'Interactive app').trim() || 'Interactive app';
  const uri = String(row?.uri || '').trim();
  if (!uri) return null;
  return (
    <div
      className="app-bubble-row app-bubble-row-assistant app-mcpui-bubble-row"
      data-render-key={row?.renderKey || ''}
      data-tool-call-id={row?.toolCallId || ''}
      data-uri={uri}
      data-testid="mcpui-bubble-row"
    >
      <div className="app-bubble-avatar app-bubble-avatar-assistant" data-testid="mcpui-bubble-avatar">
        <AvatarIcon name="SealCheck" size={14} weight="regular" color="#58657a" />
      </div>
      <div className="app-bubble app-bubble-assistant app-mcpui-bubble">
        <div className="app-bubble-content app-mcpui-bubble-content">
          <div className="app-mcpui-bubble-label">{title}</div>
          <AppRenderer uri={uri} title={title} toolInput={row?.toolInput ?? null} />
        </div>
      </div>
    </div>
  );
}
