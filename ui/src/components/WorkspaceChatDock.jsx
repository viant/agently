import React, {useId, useState} from 'react';
import {Button} from 'forge/components';
import './workspaceChatDock.css';

// Keep the existing conversation mounted while application windows remain open.
export default function WorkspaceChatDock({active, children}) {
  const [expanded, setExpanded] = useState(false);
  const id = useId();
  return <>
    {active && <Button className="app-workspace-chat-toggle" variant="secondary" icon="lucide:message-square"
      aria-controls={id} aria-expanded={expanded} onClick={() => setExpanded(value => !value)}>
      {expanded ? 'Hide chat' : 'Chat'}
    </Button>}
    <div id={id} className={`app-workspace-chat-dock${active ? ' is-overlay' : ''}`}
      role={active ? 'region' : undefined} aria-label={active ? 'Workspace chat' : undefined}
      style={active && !expanded ? {display:'none'} : undefined}>
      {children}
    </div>
  </>;
}
