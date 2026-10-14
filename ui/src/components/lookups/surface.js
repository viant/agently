import {findMetadataSignal} from 'forge/core';

export function lookupSurfaceInvocation(entry, context, parameters = []) {
  const signal = context?.identity?.windowId ? findMetadataSignal(context.identity.windowId) : null;
  const metadata = signal ? signal.peek?.() : context?.metadata;
  const namespace = uri => /^window:\/\/([^/]+)\//.exec(uri || "")?.[1];
  const sameOwner = entry.providerIdentity && metadata?.resource?.providerIdentity === entry.providerIdentity
    && namespace(metadata?.resource?.uri) === namespace(entry.pickerResource?.uri);
  const bundled = metadata?.dialogs?.some(dialog => dialog?.id === entry.dialogId);
  if (entry.dialogId && (!entry.pickerResource || (sameOwner && bundled))) {
    return {handler: 'openDialog', args: [entry.dialogId, {awaitResult: true, parameters}]};
  }
  const key = entry.pickerResource?.uri || entry.windowId;
  if (!key) throw new Error('Lookup picker resource is unavailable');
  return {handler: 'openWindow', args: [key, '', '', false, {
    awaitResult: true, parameters, modal: true,
    resourceRef: entry.pickerResource,
    providerIdentity: entry.providerIdentity,
    pickerDialogId: entry.dialogId || undefined,
  }]};
}
