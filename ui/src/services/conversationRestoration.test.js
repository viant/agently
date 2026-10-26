// @vitest-environment jsdom
import {describe,expect,it,vi} from 'vitest';
import {MAIN_CHAT_WINDOW_ID, resolveConversationSelection} from './conversationWindow';
import {trackConversationRestoration} from './conversationRestoration';

describe('asynchronous conversation restoration ownership', () => {
  it('cannot restore a previous conversation when auth finishes after New Chat', async () => {
    const ownership=trackConversationRestoration();
    let finish;
    const status=new Promise(resolve=>{finish=resolve;});
    const open=vi.fn();
    const pending=status.then(value=>{if(ownership.isCurrent())open(value.conversationId);});
    window.dispatchEvent(new CustomEvent('agently:conversation-new',{detail:{windowId:MAIN_CHAT_WINDOW_ID}}));
    finish({conversationId:'previous-chat'});
    await pending;
    expect(open).not.toHaveBeenCalled();
    ownership.dispose();
  });
  it.each(['agently:conversation-select','agently:conversation-path','popstate'])('rejects restoration after %s navigation', eventType => {
    const ownership=trackConversationRestoration();
    window.dispatchEvent(new CustomEvent(eventType,{detail:{id:'next-chat',windowId:MAIN_CHAT_WINDOW_ID}}));
    expect(ownership.isCurrent()).toBe(false);
    ownership.dispose();
  });
  it('keeps normal restoration current across creation notifications and linked-window selections', () => {
    const ownership=trackConversationRestoration();
    window.dispatchEvent(new CustomEvent('agently:conversation-new',{detail:{id:'created-chat',windowId:MAIN_CHAT_WINDOW_ID}}));
    window.dispatchEvent(new CustomEvent('agently:conversation-select',{detail:{id:'linked-chat',windowId:'linked-window'}}));
    const currentId=resolveConversationSelection(MAIN_CHAT_WINDOW_ID);
    window.dispatchEvent(new CustomEvent('agently:conversation-select',{detail:{id:currentId,windowId:MAIN_CHAT_WINDOW_ID}}));
    window.dispatchEvent(new CustomEvent('agently:conversation-path',{detail:{conversationId:currentId}}));
    expect(ownership.isCurrent()).toBe(true);
    ownership.dispose();
    expect(ownership.isCurrent()).toBe(false);
  });
});
