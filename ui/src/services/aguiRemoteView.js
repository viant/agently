import { AgUiRemoteConversationTransport, AgUiRunError } from 'agently-core-ui-sdk';
import { client } from './agentlyClient';
import { remoteRows, remoteNamespace } from './aguiRemoteRows';
const empty=Object.freeze({phase:'idle',rows:[],thread:null,sending:false,uncertain:false,error:null});
/** Cache belongs to this authenticated browser session, never native chatStore. */
export function createRemoteViewService(host) {
  const transport=new AgUiRemoteConversationTransport(host), entries=new Map();
  let generation=0;
  const entry=id=>{if(!entries.has(id))entries.set(id,{view:empty,listeners:new Set(),loading:null,subscription:null});return entries.get(id);};
  const publish=(item,update)=>{item.view={...item.view,...update};for(const listener of item.listeners)listener();};
  const current=(id,item)=>entries.get(id)===item;
  const read=(id,item,snapshot)=>{if(!current(id,item))return;publish(item,{snapshot,phase:snapshot.phase,rows:remoteRows(id,item.view.thread.threadId,snapshot)});};
  const service={
    getSnapshot:id=>entry(id).view,
    subscribe(id,listener){const item=entry(id);item.listeners.add(listener);return()=>item.listeners.delete(listener);},
    ensure(id){const item=entry(id);if(item.view.thread)return Promise.resolve(item.view.thread);if(item.loading)return item.loading;return service.newConversation(id);},
    async newConversation(id){
      const item=entry(id);if(item.view.sending)throw new Error('Wait for this run to finish before creating a new conversation');
      if(item.loading)return item.loading;
      const owner=generation;publish(item,{phase:'creating',error:null});
      item.loading=(async()=>{
        try{
          const thread=await transport.create(id);if(owner!==generation||!current(id,item))return;
          item.subscription?.close();publish(item,{...empty,thread,namespace:remoteNamespace(id,thread.threadId)});
          item.subscription=transport.subscribe(id,thread.threadId,{onSession:snapshot=>read(id,item,snapshot),onDescriptor:descriptor=>{if(descriptor.kind==='protocol-snapshot')read(id,item,transport.getSnapshot(id,thread.threadId));}});
          return thread;
        }catch(error){if(current(id,item))publish(item,{phase:'failed',error:error.message});throw error;}
        finally{if(owner===generation)item.loading=null;}
      })();return item.loading;
    },
    async send(id,text){
      const item=entry(id);if(!item.view.thread)throw new Error('Remote conversation is not ready');
      if(item.view.sending||item.view.uncertain)throw new Error('Remote execution is running or uncertain');
      const owner=generation;const thread=item.view.thread;let submitted=false;publish(item,{sending:true,error:null});
      try{const submission=transport.send(id,thread.threadId,text);submitted=true;await submission.completion;}
      catch(error){if(owner===generation&&current(id,item))publish(item,{error:error.message,uncertain:submitted && !(error instanceof AgUiRunError)});throw error;}
      finally{if(owner===generation&&current(id,item))publish(item,{sending:false});}
    },
    reset(){generation++;transport.reset();for(const item of entries.values()){item.subscription?.close();item.subscription=null;item.loading=null;publish(item,{...empty});}},
  };return service;
}
export const aguiRemoteView=createRemoteViewService(client);
if(typeof window!=='undefined')window.addEventListener('agently:session-reset',()=>aguiRemoteView.reset());
