import React, { useEffect, useState, useSyncExternalStore } from 'react';
import { Button } from '@blueprintjs/core';
import ChatFeedFromChatStore from './chat/ChatFeedFromChatStore';
import { ConversationViewContext } from '../context/ConversationViewContext';
import { aguiRemoteView } from '../services/aguiRemoteView';
const isolated={developerMode:false,showIntakeDetails:false,setShowIntakeDetails:()=>{},toolFeedDock:'inline',workspaceWindow:null,workspaceVisible:false,onOpenWorkspace:()=>{}};
export default function AgUiRemoteConversation({connection,active=true,developerMode=false}) {
  const id=connection.id;
  const view=useSyncExternalStore(listener=>aguiRemoteView.subscribe(id,listener),()=>aguiRemoteView.getSnapshot(id),()=>aguiRemoteView.getSnapshot(id));
  const [text,setText]=useState('');
  useEffect(()=>{if(active)void aguiRemoteView.ensure(id).catch(()=>{});},[id,active]);
  const context=React.useMemo(()=>({Context:()=>({handlers:{dataSource:{peekFormData:()=>({id:view.namespace||`remote:${id}`})}}})}),[id,view.namespace]);
  const blocked=!active||!view.thread||view.sending||view.uncertain||view.phase==='interrupted'||!!view.snapshot?.pendingToolCallIds?.length;
  const submit=async event=>{event?.preventDefault();if(blocked||!text.trim())return;const submitted=text;setText('');try{await aguiRemoteView.send(id,submitted);}catch(_){} };
  const status=view.uncertain?'Connection lost; execution outcome is uncertain. Create a new conversation to continue.':view.phase==='interrupted'?'This backend interrupted the run. Continuation is unavailable in this text-only connection.':view.snapshot?.pendingToolCallIds?.length?'Frontend tool execution is unavailable in this connection.':view.sending?'Running…':view.phase==='creating'?'Creating conversation…':view.error||'';
  return <ConversationViewContext.Provider value={isolated}>
    <section className="app-agui-remote-conversation" data-testid="agui-remote-conversation" data-backend={id} style={{display:active?'flex':'none',flexDirection:'column',height:'100%',minHeight:0}}>
      <div style={{padding:'8px 16px',display:'flex',alignItems:'center',gap:12}}><span>{connection.label} · Session-only conversation · Text only</span><Button small disabled={!active||view.sending||view.phase==='creating'} onClick={()=>{setText('');void aguiRemoteView.newConversation(id).catch(()=>{});}}>New conversation</Button></div>
      <div data-testid="chat-feed" style={{flex:1,overflow:'auto'}}>
        <ChatFeedFromChatStore conversationId="" rowsOverride={view.rows} context={context}/>
        {developerMode && view.snapshot?.state && Object.keys(view.snapshot.state).length ? <details style={{padding:'8px 16px'}}><summary>Backend state</summary><pre>{JSON.stringify(view.snapshot.state,null,2)}</pre></details>:null}
        {developerMode ? <details style={{padding:'8px 16px'}}><summary>Protocol diagnostics</summary><pre>{JSON.stringify((view.snapshot?.messages || []).filter(message=>['system','developer','activity'].includes(message.role)),null,2)}</pre></details> : null}
      </div>
      {status?<div role="status" style={{padding:'8px 16px'}}>{status}</div>:null}
      <form data-testid="chat-composer" data-connection-profile="standard" onSubmit={submit}>
        <div data-testid="chat-composer-shell" className="composer-bar" style={{display:'flex',alignItems:'end',gap:8,padding:12}}>
          <div className="composer-input-wrap" style={{flex:1}}><textarea data-testid="chat-composer-input" aria-label={`Message ${connection.label}`} placeholder="Message…" rows={2} value={text} disabled={blocked} onChange={event=>setText(event.target.value)} onKeyDown={event=>{if(event.key==='Enter'&&!event.shiftKey){event.preventDefault();void submit();}}} style={{width:'100%',resize:'vertical'}}/></div>
          <div className="composer-bar-right"><Button intent="primary" type="submit" disabled={blocked||!text.trim()}>Send</Button></div>
        </div>
        <div style={{padding:'6px 16px 8px',fontSize:12}}>Enter to send · Shift+Enter for a new line</div>
      </form>
    </section>
  </ConversationViewContext.Provider>;
}
