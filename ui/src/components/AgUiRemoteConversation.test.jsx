import { describe, expect, it, vi } from 'vitest';
import React, { useContext } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { ConversationViewContext } from '../context/ConversationViewContext';
const mocks=vi.hoisted(()=>({ensure:vi.fn(),send:vi.fn(),fresh:vi.fn(),view:{phase:'idle',rows:[],thread:{threadId:'foreign'},sending:false,uncertain:false,error:null}}));
vi.mock('../services/aguiRemoteView',()=>({aguiRemoteView:{getSnapshot:()=>mocks.view,subscribe:()=>()=>{},ensure:mocks.ensure,send:mocks.send,newConversation:mocks.fresh}}));
vi.mock('./chat/ChatFeedFromChatStore',()=>({default:props=>{const context=useContext(ConversationViewContext);return <div data-testid="production-feed" data-conversation={props.conversationId} data-workspace={String(context.workspaceVisible)}>{props.rowsOverride.length} rows</div>;}}));
import AgUiRemoteConversation from './AgUiRemoteConversation';
describe('remote conversation production surface',()=>{
  it('uses isolated production feed and a service-free text composer with unavailable capabilities',()=>{
    const html=renderToStaticMarkup(<AgUiRemoteConversation connection={{id:'dojo',label:'Dojo'}}/>);
    expect(html).toContain('production-feed');expect(html).toContain('data-conversation=""');expect(html).toContain('data-workspace="false"');expect(html).toContain('<textarea');expect(html).toContain('Session-only conversation');expect(html).toContain('Text only');expect(html).toContain('chat-composer-shell');expect(html).toContain('data-connection-profile="standard"');
    expect(mocks.send).not.toHaveBeenCalled();expect(mocks.ensure).not.toHaveBeenCalled();
  });
  it('shows backend state and extension diagnostics only after explicit developer opt-in',()=>{
    mocks.view={...mocks.view,snapshot:{state:{privateState:'state detail'},messages:[{role:'system',content:'App Context: {thread_id: remote}'},{role:'activity',activityType:'unknown.extension',content:{debug:'extension detail'}}]}};
    const normal=renderToStaticMarkup(<AgUiRemoteConversation connection={{id:'dojo',label:'Dojo'}}/>);
    expect(normal).not.toContain('Backend state');expect(normal).not.toContain('App Context');expect(normal).not.toContain('extension detail');
    const diagnostic=renderToStaticMarkup(<AgUiRemoteConversation connection={{id:'dojo',label:'Dojo'}} developerMode/>);
    expect(diagnostic).toContain('Backend state');expect(diagnostic).toContain('Protocol diagnostics');expect(diagnostic).toContain('App Context');expect(diagnostic).toContain('extension detail');
  });
  it('keeps inactive surface hidden and submission controls disabled',()=>{
    const html=renderToStaticMarkup(<AgUiRemoteConversation connection={{id:'dojo',label:'Dojo'}} active={false}/>);
    expect(html).toContain('display:none');expect(html).toContain('disabled=""');expect(mocks.fresh).not.toHaveBeenCalled();
  });
  it('reports uncertain outcomes rather than implying failure/cancellation of effects',()=>{
    mocks.view={...mocks.view,uncertain:true,error:'lost'};
    const html=renderToStaticMarkup(<AgUiRemoteConversation connection={{id:'dojo',label:'Dojo'}}/>);
    expect(html).toContain('execution outcome is uncertain');expect(html).toContain('Create a new conversation');
  });
});
