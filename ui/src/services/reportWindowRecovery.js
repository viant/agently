import {addWindow, findViewSignal, getViewSignal} from 'forge/core';

// Explicit recovery creates a new native instance; it never rebinds the old
// instance to different published bytes or changes an explicitly pinned ref.
export function reopenReportWindow(entry) {
  const resourceRef = entry.resourceRef?.uri ? {...entry.resourceRef} : {uri:entry.windowKey};
  const observed = findViewSignal(entry.windowId)?.peek()?.report;
  const retained = observed?.resourceUri === resourceRef.uri ? observed : null;
  const object = value => value && typeof value === 'object' && !Array.isArray(value);
  const applied = object(retained?.parameters) ? retained.parameters : entry.parameters || {};
  const draft = object(retained?.draftParameters) ? retained.draftParameters : applied;
  const next = addWindow(entry.windowTitle, entry.windowId, entry.windowKey, null,
    entry.inTab !== false, {...structuredClone(applied),executeOnOpen:false}, {
      newInstance:true,resourceRef,linkedRevision:entry.linkedRevision,
      providerIdentity:entry.providerIdentity,conversationId:entry.conversationId,
      presentation:entry.presentation,region:entry.region,windowScopeId:entry.windowScopeId,
      hostOpenState:'historical_replay',
    });
  // Reuse the existing replay seed for applied/draft inputs only. Completed
  // output belongs to the old revision and must not cross this boundary.
  getViewSignal(next.windowId).value={report:{resourceUri:resourceRef.uri,
    parameters:structuredClone(applied),draftParameters:structuredClone(draft)}};
  return next;
}
