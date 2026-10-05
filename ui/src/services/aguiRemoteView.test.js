import { describe, expect, it, vi } from 'vitest';
vi.mock('./agentlyClient',()=>({client:{}}));
import { createRemoteViewService } from './aguiRemoteView';
import { remoteRows } from './aguiRemoteRows';
function harness(events) {
  let count=0;const posts=[];
  const host={listAgUiBackends:vi.fn(async()=>['one','two'].map(id=>({id,label:id,profile:'standard',durableReplay:false,ephemeral:true}))),createAgUiBackendThread:vi.fn(async connectionId=>({connectionId,threadId:`thread-${++count}`,ephemeral:true,durableReplay:false})),agUiTransport:vi.fn(()=>({url:'/bff/run',fetch:async(_url,init)=>{const input=JSON.parse(init.body);posts.push(input);return new Response((events?.(input,posts.length)||[{type:'RUN_STARTED',threadId:input.threadId,runId:input.runId},{type:'MESSAGES_SNAPSHOT',messages:[...input.messages,{id:`assistant-${posts.length}`,role:'assistant',content:'answer'}]},{type:'RUN_FINISHED',threadId:input.threadId,runId:input.runId}]).map(event=>`data: ${JSON.stringify(event)}\n\n`).join(''),{headers:{'Content-Type':'text/event-stream'}});}}))};
  return {service:createRemoteViewService(host),host,posts};
}
describe('remote production feed view',()=>{
  it('reconciles snapshot revisions/deletions by reading the official graph',async()=>{
    const {service}=harness(input=>[{type:'RUN_STARTED',threadId:input.threadId,runId:input.runId},{type:'MESSAGES_SNAPSHOT',messages:[...input.messages,{id:'old',role:'assistant',content:'first'},{id:'delete',role:'assistant',content:'gone'}]},{type:'MESSAGES_SNAPSHOT',messages:[...input.messages,{id:'old',role:'assistant',content:'replacement'}]},{type:'RUN_FINISHED',threadId:input.threadId,runId:input.runId}]);
    await service.ensure('one');await service.send('one','hello');
    expect(service.getSnapshot('one').rows.map(row=>row.content)).toEqual(['hello','replacement']);
    expect(service.getSnapshot('one').rows.every(row=>row.hostEffectsAllowed===false)).toBe(true);
  });
  it('retains one thread/history on remount and isolates other backend threads',async()=>{
    const {service,host,posts}=harness();await service.ensure('one');
    const stop=service.subscribe('one',()=>{});await service.send('one','hello');stop();
    const before=service.getSnapshot('one');await service.ensure('one');expect(service.getSnapshot('one')).toBe(before);
    await service.send('one','follow-up');await service.ensure('two');await service.send('two','separate');
    expect(host.createAgUiBackendThread).toHaveBeenCalledTimes(2);expect(posts[1].messages.length).toBeGreaterThan(1);expect(posts[2].messages).toHaveLength(1);
    expect(service.getSnapshot('one').rows[0].renderKey).not.toBe(service.getSnapshot('two').rows[0].renderKey);
    expect(posts.every(input=>JSON.stringify(input.forwardedProps)==='{}'&&input.tools.length===0&&input.context.length===0)).toBe(true);
    service.reset();expect(service.getSnapshot('one').rows).toEqual([]);expect(service.getSnapshot('one').thread).toBeNull();
  });
  it('updates baseline history and removes prior messages on a follow-up snapshot',async()=>{
    const {service}=harness((input,count)=>[{type:'RUN_STARTED',threadId:input.threadId,runId:input.runId},{type:'MESSAGES_SNAPSHOT',messages:count===1?[...input.messages,{id:'baseline-answer',role:'assistant',content:'old answer'}]:[{...input.messages[0],content:'edited old question'},input.messages.at(-1),{id:'replacement-answer',role:'assistant',content:'new answer'}]},{type:'RUN_FINISHED',threadId:input.threadId,runId:input.runId}]);
    await service.ensure('one');await service.send('one','first');const before=service.getSnapshot('one').rows[0].renderKey;
    await service.send('one','second');
    expect(service.getSnapshot('one').rows.map(row=>row.content)).toEqual(['edited old question','second','new answer']);
    expect(service.getSnapshot('one').rows[0].renderKey).toBe(before);
  });
  it('hides actual Dojo App Context system instructions and extensions without changing official history',()=>{
    const snapshot={phase:'completed',messages:[{id:'dojo-system',role:'system',content:'App Context: {"thread_id":"dojo-remote-thread"}'},{id:'developer',role:'developer',content:'Internal developer instructions'},{id:'u',role:'user',content:'Hello'},{id:'extension',role:'activity',activityType:'unknown.extension',content:{privateImplementation:'opaque'}},{id:'a',role:'assistant',content:'AGUI_WEB_INTEROP_OK'}]};
    const rows=remoteRows('dojo','thread',snapshot);
    expect(rows.map(row=>row.content)).toEqual(['Hello','AGUI_WEB_INTEROP_OK']);
    expect(snapshot.messages).toHaveLength(5);expect(snapshot.messages[0].content).toContain('App Context');
  });
  it('preserves multipart/tool text passively and hides unknown activity bodies',()=>{
    const rows=remoteRows('one','thread',{phase:'completed',messages:[{id:'u',role:'user',content:[{type:'text',text:'question'},{type:'image',url:'https://private'}]},{id:'tool',role:'tool',toolCallId:'call',content:'{"_meta":{"ui":{"resourceUri":"ui://private"}}}'},{id:'activity',role:'activity',activityType:'mcp-apps',content:{resourceUri:'ui://private'}}]});
    expect(rows[0].content).toContain('https://private');expect(rows[1].content).toContain('_meta');expect(rows).toHaveLength(2);expect(rows.every(row=>row.hostEffectsAllowed===false&&row.connectionProfile==='standard')).toBe(true);
  });
});
