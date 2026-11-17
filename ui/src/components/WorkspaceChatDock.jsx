import React, {useId, useRef, useState, useEffect, useLayoutEffect, useCallback} from 'react';
import Draggable from 'react-draggable';
import {Button, DropdownMenu} from 'forge/components';
import {Menu, MenuDivider, MenuItem} from '@blueprintjs/core';
import {useWorkspaceStyle} from './WorkspaceStyleProvider';
import './workspaceChatDock.css';

// Presentation changes never remount the existing conversation or its draft.
export default function WorkspaceChatDock({active, children, onOpen}) {
  const [expanded,setExpanded]=useState(false),[mode,setMode]=useState('overlay');
  const isDesignSystem2 = useWorkspaceStyle()?.state.themeId === 'design-system-2';
  const [side, setSide] = useState('left');
  const [position, setPosition] = useState({x: 0, y: 0});
  const [bounds, setBounds] = useState(false);
  const positionRef = useRef(position);
  const movable = isDesignSystem2 && active && expanded && mode === 'overlay';
  const id=useId(),content=useRef(null);
  const moveTo = useCallback(next => {
    positionRef.current = next;
    setPosition(next);
  }, []);
  const measureBounds = useCallback(() => {
    const node = content.current;
    const parent = node?.parentElement;
    if (!node || !parent) return;
    const rect = node.getBoundingClientRect(), area = parent.getBoundingClientRect();
    if (!rect.width || !rect.height || !area.width || !area.height) return;
    const current = positionRef.current;
    const next = {
      left: area.left + 16 - rect.left + current.x,
      right: area.right - 16 - rect.right + current.x,
      top: area.top + 16 - rect.top + current.y,
      bottom: area.bottom - 16 - rect.bottom + current.y,
    };
    setBounds(next);
    moveTo({
      x: Math.max(next.left, Math.min(next.right, current.x)),
      y: Math.max(next.top, Math.min(next.bottom, current.y)),
    });
  }, [moveTo]);
  useLayoutEffect(() => {
    if (!movable) return;
    measureBounds();
    const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(measureBounds);
    observer?.observe(content.current);
    observer?.observe(content.current.parentElement);
    return () => observer?.disconnect();
  }, [movable, measureBounds]);
  const moveWithKeyboard = event => {
    // Header movement is available without a pointer; ordinary chat keys retain
    // their existing behavior, including keys on the header's child controls.
    if (!movable || event.target !== event.currentTarget) return;
    if (event.key === 'Home') {
      event.preventDefault(); moveTo({x: 0, y: 0}); return;
    }
    const delta = {ArrowLeft: [-16, 0], ArrowRight: [16, 0], ArrowUp: [0, -16], ArrowDown: [0, 16]}[event.key];
    if (!delta) return;
    event.preventDefault();
    moveTo({
      x: Math.max(bounds.left ?? -Infinity, Math.min(bounds.right ?? Infinity, position.x + delta[0])),
      y: Math.max(bounds.top ?? -Infinity, Math.min(bounds.bottom ?? Infinity, position.y + delta[1])),
    });
  };
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
    <Draggable nodeRef={content} disabled={!movable} handle=".app-workspace-chat-controls"
      cancel="button, a, input, select, textarea, [role=button]" bounds={bounds}
      position={movable ? position : {x: 0, y: 0}}
      positionOffset={active && mode === 'overlay' ? {x: '-50%', y: 0} : undefined}
      onStart={measureBounds} onDrag={(_, next) => moveTo({x: next.x, y: next.y})}>
    <div ref={content} id={id} className={`app-workspace-chat-dock${active ? ` is-${mode}` : ''}`} data-forge-component="chat-dock"
      data-chat-side={isDesignSystem2 ? side : undefined}
      data-expanded={active?expanded:undefined} role={active?'region':undefined} aria-label={active?'Workspace chat':undefined}
      style={active&&!expanded?{display:'none'}:undefined}>
      {active&&expanded&&<div className="app-workspace-chat-controls" data-movable={movable || undefined}
        data-chat-options={isDesignSystem2 || undefined}
        tabIndex={movable ? 0 : undefined} role={movable ? 'group' : undefined}
        aria-label={movable ? 'Move chat. Use arrow keys to move; Home to reset position.' : undefined}
        onKeyDown={moveWithKeyboard}>
        {isDesignSystem2 ? <>
          <span className="app-workspace-chat-title">Chat</span>
          <DropdownMenu placement="bottom-end" content={<Menu aria-label="Chat layout">
            <MenuItem text="Split on left" aria-current={mode === 'split' && side === 'left' ? 'true' : undefined}
              onClick={() => {setSide('left');setMode('split');}}/>
            <MenuItem text="Split on right" aria-current={mode === 'split' && side === 'right' ? 'true' : undefined}
              onClick={() => {setSide('right');setMode('split');}}/>
            <MenuItem text="Overlay" aria-current={mode === 'overlay' ? 'true' : undefined} onClick={() => setMode('overlay')}/>
            {mode === 'overlay' && <><MenuDivider/><MenuItem text="Reset position" disabled={!position.x && !position.y}
              onClick={() => moveTo({x: 0, y: 0})}/></>}
          </Menu>}>
            <Button variant="ghost" size="compact" hideLabel icon="lucide:ellipsis" aria-label="Chat options" title="Chat options"/>
          </DropdownMenu>
        </> : <Button variant="ghost" icon={mode==='overlay'?'lucide:panel-right':'lucide:panel-right-close'} onClick={()=>setMode(value=>value==='overlay'?'split':'overlay')}>
          {mode==='overlay'?'Split view':'Overlay'}
        </Button>}
        <Button variant="ghost" size={isDesignSystem2 ? 'compact' : 'default'} hideLabel={isDesignSystem2 || undefined}
          icon="lucide:x" aria-label="Hide chat" onClick={()=>setExpanded(false)}/>
      </div>}
      {children}
    </div>
    </Draggable>
  </>;
}
