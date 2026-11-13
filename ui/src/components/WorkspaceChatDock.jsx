import React, {useId, useRef, useState, useEffect} from 'react';
import {Button} from 'forge/components';
import './workspaceChatDock.css';

// Presentation changes never remount the existing conversation or its draft.
export default function WorkspaceChatDock({active, children, onOpen}) {
  const [expanded,setExpanded]=useState(false),[mode,setMode]=useState('overlay');
  const id=useId(),content=useRef(null);
  useEffect(()=>{if(active&&expanded){const feed=content.current?.querySelector('.chat-feed-stage');if(feed)feed.scrollTop=feed.scrollHeight;}},[active,expanded]);
  const openChat = nextMode => {
    onOpen?.();
    setMode(nextMode);
    setExpanded(true);
  };
  return <>
    {active && !expanded && <div className="app-workspace-chat-toggle" role="group" aria-label="Open workspace chat">
      <Button variant="secondary" icon="lucide:message-square" title="Open overlay chat"
        aria-controls={id} aria-expanded={false} onClick={() => openChat('overlay')}>Chat</Button>
      <Button variant="secondary" icon="lucide:panel-right" className="app-chat-left-toggle" aria-label="Open chat in split view"
        title="Open chat in split view" aria-controls={id} onClick={() => openChat('split')}/>
    </div>}
    <div ref={content} id={id} className={`app-workspace-chat-dock${active ? ` is-${mode}` : ''}`} data-forge-component="chat-dock"
      data-expanded={active?expanded:undefined} role={active?'region':undefined} aria-label={active?'Workspace chat':undefined}
      style={active&&!expanded?{display:'none'}:undefined}>
      {active&&expanded&&<div className="app-workspace-chat-controls">
        <Button variant="ghost" icon={mode==='overlay'?'lucide:panel-right':'lucide:panel-right-close'} onClick={()=>setMode(value=>value==='overlay'?'split':'overlay')}>
          {mode==='overlay'?'Split view':'Overlay'}
        </Button>
        <Button variant="ghost" icon="lucide:x" aria-label="Hide chat" onClick={()=>setExpanded(false)}/>
      </div>}
      {children}
    </div>
  </>;
}
