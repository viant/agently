import {activeWindows,getActiveUIBridgeTransportContext,refreshWorkspace} from 'forge/core';

const abortError=()=>new DOMException('The operation was aborted.','AbortError');

// Only the current mounted window's refresh registration can issue the host
// metadata GET that rebinds its negotiated UI instance. Datasource inputs
// never populate or renew a server binding.
const locator=window=>JSON.stringify([window?.windowKey,window?.providerIdentity,window?.resourceRef?.uri,
  window?.resourceRef?.revision,window?.parameters]);

export function createPublishedWindowBindingRecovery({refresh,currentTransport,currentWindow}) {
  const pending=new Map();
  return async ({windowId,signal,sessionId}={})=>{
    if(signal?.aborted)throw abortError();
    const session=currentTransport()?.sessionId;
    if(!windowId || !session || sessionId!==session)throw new Error('Workspace transport changed during readmission');
    const mounted=currentWindow(windowId);
    if(!mounted)throw new Error('Workspace window is no longer mounted');
    const expected=locator(mounted);
    let admission=pending.get(windowId);
    if(admission && admission.session!==session)throw new Error('Workspace transport changed during readmission');
    if(!admission){
      const task=Promise.resolve().then(()=>refresh({windowId,reloadMetadata:true}));
      admission={session,task};
      pending.set(windowId,admission);
      void task.finally(()=>{if(pending.get(windowId)===admission)pending.delete(windowId);}).catch(()=>{});
    }
    await admission.task;
    if(signal?.aborted)throw abortError();
    if(currentTransport()?.sessionId!==session)throw new Error('Workspace transport changed during readmission');
    if(!currentWindow(windowId) || locator(currentWindow(windowId))!==expected)throw new Error('Workspace window changed during readmission');
  };
}

export const recoverPublishedWindowBinding=createPublishedWindowBindingRecovery({
  refresh:refreshWorkspace,currentTransport:getActiveUIBridgeTransportContext,
  currentWindow:id=>activeWindows.peek().find(window=>window.windowId===id),
});
