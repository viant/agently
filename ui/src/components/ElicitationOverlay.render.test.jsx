// @vitest-environment jsdom
import React from 'react';
import {act, cleanup, fireEvent, render, screen, waitFor} from '@testing-library/react';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import ChatFeed from './chat/ChatFeedFromChatStore.jsx';
import {ConversationViewContext} from '../context/ConversationViewContext.js';
import {DetailContext} from '../context/DetailContext.js';
import ElicitationOverlay, {ConversationQuestions} from './ElicitationOverlay.jsx';
import {clearPendingElicitation, getPendingElicitations, setPendingElicitation} from '../services/elicitationBus.js';

const mocks = vi.hoisted(() => ({resolve:vi.fn(), refresh:vi.fn()}));
vi.mock('../services/agentlyClient', () => ({client:{resolveElicitation:mocks.resolve}}));
vi.mock('./chat/LazyRichContent', async()=>({default:(await import('./chat/RichContent.jsx')).default}));
vi.mock('../services/chatRuntime', () => ({refreshAfterElicitationResolution:mocks.refresh}));

const requestedSchema = {type:'object', required:['frequency'], properties:{frequency:{type:'string', title:'Update frequency', enum:['Daily','Weekly','I am not sure'], 'x-ui-widget':'radio'}}};
const question = {conversationId:'example', elicitationId:'question', message:'How often would you like an update?', requestedSchema,
  callbackURL:'/v1/api/conversations/example-child/elicitation/question'};
function mount() {
  return render(<><ConversationQuestions conversationId="example"/><ElicitationOverlay appearance="document" inlineConversationId="example"/></>);
}
beforeEach(() => {clearPendingElicitation();mocks.resolve.mockReset().mockResolvedValue({});mocks.refresh.mockReset().mockResolvedValue({});});
afterEach(() => {cleanup();clearPendingElicitation();});

describe('ordinary questions in the conversation', () => {
  it('renders once without a default, validates through the real form, and resumes the authoritative conversation once', async () => {
    let finish;
    mocks.resolve.mockImplementation(() => new Promise(resolve => {finish=resolve;}));
    setPendingElicitation(question);mount();
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(screen.getAllByRole('region', {name:'Needs your input'})).toHaveLength(1);
    expect(screen.getAllByRole('radio')).toHaveLength(3);
    expect(screen.getAllByRole('radio').every(input=>!input.checked)).toBe(true);
    expect(screen.getByRole('radiogroup').getAttribute('aria-invalid')).not.toBe('true');
    expect(screen.getByRole('radiogroup').className).not.toContain('forge-required-missing');
    fireEvent.click(screen.getByRole('button',{name:'Continue',exact:true}));
    expect(mocks.resolve).not.toHaveBeenCalled();
    expect(screen.getByRole('radiogroup').getAttribute('aria-invalid')).toBe('true');
    expect(screen.getAllByRole('alert').map(el=>el.textContent).join(' ')).toContain('Please fix');
    fireEvent.click(screen.getByRole('radio',{name:'Weekly'}));
    fireEvent.click(screen.getByRole('button',{name:'Continue',exact:true}));
    expect(mocks.resolve).toHaveBeenCalledOnce();
    expect(mocks.resolve).toHaveBeenCalledWith('example-child','question',{action:'submit',payload:{frequency:'Weekly'}});
    expect(screen.getByRole('button',{name:'Continue',exact:true}).disabled).toBe(true);
    expect(screen.getAllByRole('radio').every(input=>input.disabled)).toBe(true);
    await act(async()=>finish({}));
    await waitFor(()=>expect(screen.queryByRole('region',{name:'Needs your input'})).toBeNull());
    expect(getPendingElicitations()).toEqual([]);
    expect(mocks.refresh).toHaveBeenCalledOnce();
  });
  for (const [label,action] of [['Skip','decline'],['Cancel','cancel']]) {
    it(`${label} returns no inferred answer`, async () => {
      setPendingElicitation(question);mount();
      fireEvent.click(screen.getByRole('radio',{name:'Daily'}));
      fireEvent.click(screen.getByRole('button',{name:label,exact:true}));
      await waitFor(()=>expect(mocks.resolve).toHaveBeenCalledOnce());
      expect(mocks.resolve).toHaveBeenCalledWith('example-child','question',{action,payload:{}});
    });
  }
  it('keeps the answer and shows the permission error when resolution is denied', async () => {
    mocks.resolve.mockRejectedValue(Object.assign(new Error('Denied'),{status:403}));
    setPendingElicitation(question);mount();
    fireEvent.click(screen.getByRole('radio',{name:'I am not sure'}));
    fireEvent.click(screen.getByRole('button',{name:'Continue',exact:true}));
    await waitFor(()=>expect(screen.getByRole('alert').textContent).toContain('no longer have access'));
    expect(screen.getByRole('radio',{name:'I am not sure'}).checked).toBe(true);
    expect(getPendingElicitations()).toHaveLength(1);
    expect(mocks.refresh).not.toHaveBeenCalled();
  });
  it('keeps the finding visible during an active question without a duplicate review prompt', async () => {
    setPendingElicitation(question);
    const row = {kind:'iteration',renderKey:'example-turn',conversationId:'example',turnId:'turn',lifecycle:'running',elicitation:question,elicitations:[question],isLatestIteration:true,
      rounds:[{pageId:'example-page',status:'running',phase:'main',toolCalls:[{toolCallId:'question-call',toolName:'message/askUser',status:'running'}]}]};
    render(<ConversationViewContext.Provider value={{presentation:'document',inlineQuestionConversationId:'example',reportsInWorkspace:true,developerMode:false}}>
      <DetailContext.Provider value={{showDetail:()=>{}}}>
        <ChatFeed conversationId="example" rowsOverride={[row,{kind:'assistant',renderKey:'finding',messageId:'finding',turnId:'turn',content:'The saved settings need one more check.'}]}/>
        <ElicitationOverlay appearance="document" inlineConversationId="example"/>
      </DetailContext.Provider>
    </ConversationViewContext.Provider>);
    await waitFor(()=>expect(screen.getByText('The saved settings need one more check.')).toBeDefined());
    expect(screen.getAllByRole('region',{name:'Needs your input'})).toHaveLength(1);
    expect(screen.queryByRole('button',{name:'Review request',exact:true})).toBeNull();
    expect(screen.queryByRole('dialog')).toBeNull();
  });
  it('does not repeat model narration as a second question while the form is waiting', async () => {
    setPendingElicitation(question);
    const row = {kind:'iteration',renderKey:'question-turn',conversationId:'example',turnId:'turn',lifecycle:'running',elicitation:question,elicitations:[question],isLatestIteration:true,
      rounds:[{pageId:'question-page',status:'running',phase:'main',content:question.message,toolCalls:[{toolCallId:'ask-call',toolName:'message/askUser',status:'running'}]}]};
    render(<ConversationViewContext.Provider value={{presentation:'document',inlineQuestionConversationId:'example',reportsInWorkspace:true,developerMode:false}}>
      <DetailContext.Provider value={{showDetail:()=>{}}}><ChatFeed conversationId="example" rowsOverride={[row]}/></DetailContext.Provider>
    </ConversationViewContext.Provider>);
    expect(screen.getAllByText(question.message)).toHaveLength(1);
    expect(screen.getAllByRole('radiogroup')).toHaveLength(1);
  });
  it('keeps approvals, authentication and another conversation in their dialogs', () => {
    setPendingElicitation(question);
    setPendingElicitation({...question,elicitationId:'approval',requestedSchema:{type:'object',properties:{_type:{const:'tool_approval'},_title:{const:'Review permission'}}}});
    setPendingElicitation({...question,elicitationId:'connection',mode:'url',url:'https://example.org/authorize',requestedSchema:{type:'object',properties:{}}});
    setPendingElicitation({...question,conversationId:'another',elicitationId:'other'});
    mount();
    expect(screen.getAllByRole('dialog')).toHaveLength(3);
    expect(screen.getAllByText('How often would you like an update?')).toHaveLength(4);
    expect(screen.getAllByRole('button',{name:'Continue',exact:true})).toHaveLength(1);
    expect(screen.getByRole('button',{name:'Open',exact:true})).toBeDefined();
    expect(mocks.resolve).not.toHaveBeenCalled();
  });
});
