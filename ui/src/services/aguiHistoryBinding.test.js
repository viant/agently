import {describe, expect, it} from 'vitest';
import {AgentlyClient} from 'agently-core-ui-sdk/client';
import {AgUiConversationTransport} from 'agently-core-ui-sdk/aguiConversationTransport';

describe('history navigation binding', () => {
  it('reads only the native header before compact history and preserves opaque identity and messages', async () => {
    const requests=[];
    const conversationId='native-history', wireId='  opaque-雪\t';
    const turns=[{turnId:'saved-turn',status:'completed',message:[{id:'saved-message',role:'assistant',content:'Saved report highlights'}]}];
    const fetchImpl=async (url,init) => {
      expect(init.credentials).toBe('include');
      requests.push({url:String(url),method:init.method});
      if(init.method==='GET') {
        // The native header route defaults to a full transcript, including
        // model payloads. Binding resolution must explicitly opt out.
        const headerOnly=String(url).endsWith('?includeTranscript=false');
        return new Response(JSON.stringify({id:conversationId,aguiThreadId:wireId,...(headerOnly?{}:{transcript:[{modelPayload:'x'.repeat(1024*1024)}]})}));
      }
      const input=JSON.parse(init.body);
      expect(input.threadId).toBe(wireId);
      expect(input.forwardedProps.agently.operation).toBe('conversation.bootstrap');
      expect(input.forwardedProps.agently.payload).toEqual({mode:'transcript',includeModelPayloads:false,selectors:{ExecutionGroup:{limit:5,offset:2}}});
      const events=[{type:'RUN_STARTED',threadId:wireId,runId:input.runId},{type:'RUN_FINISHED',threadId:wireId,runId:input.runId,result:{version:'1',threadId:wireId,transcript:{schemaVersion:'1',aguiThreadId:wireId,conversation:{conversationId,turns}},messages:[],state:{},runs:[],hostActivities:[],unavailableHostActivityIds:[],projection:{lossless:true,unavailableMessageIds:[]}}}];
      return new Response(events.map(event=>'data: '+JSON.stringify(event)+'\n\n').join(''),{headers:{'Content-Type':'text/event-stream'}});
    };
    const host=new AgentlyClient({baseURL:'/v1',useCookies:true,fetchImpl});
    const transport=new AgUiConversationTransport(host,()=>({subscriber:{}}));
    try {
      const input={mode:'transcript',includeModelPayloads:false,selectors:{ExecutionGroup:{limit:5,offset:2}}};
      const history=await transport.readSnapshot(conversationId,input);
      expect(requests).toEqual([{url:'/v1/conversations/native-history?includeTranscript=false',method:'GET'},{url:'/v1/ag-ui/run',method:'POST'}]);
      expect(history.turns).toEqual([{...turns[0],id:'saved-turn',conversationId}]);
      await transport.readSnapshot(conversationId,input);
      expect(requests.filter(request=>request.method==='GET')).toHaveLength(1);
    } finally {transport.reset();host.resetAgUiInteractions();}
  });
});
