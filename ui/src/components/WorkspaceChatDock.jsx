import React, {useId, useRef, useState, useEffect} from 'react';
import {Button} from 'forge/components';
import './workspaceChatDock.css';

// Presentation changes never remount the existing conversation or its draft.
export default function WorkspaceChatDock({active, children, onOpen}) {
  const [expanded,setExpanded]=useState(false),[mode,setMode]=useState('overlay');
  const id=useId(),content=useRef(null);
  useEffect(()=>{if(active&&expanded){const feed=content.current?.querySelector('.chat-feed-stage');if(feed)feed.scrollTop=feed.scrollHeight;}},[active,expanded]);
  return <>
    {active&&!expanded&&<Button className="app-workspace-chat-toggle" variant="secondary" icon="lucide:message-square"
      aria-controls={id} aria-expanded={false} onClick={()=>{onOpen?.();setExpanded(true);}}>Chat</Button>}
    <div ref={content} id={id} className={`app-workspace-chat-dock${active ? ` is-${mode}` : ''}`}
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
