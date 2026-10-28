import {removeWindow} from 'forge/core';
import {downloadWindowArchive} from 'forge/host-window/download';
import {managementWindowDestination} from 'forge/host-window/navigation';
import {openWindow} from '../components/menuBarHelpers.js';

const dirtyWindows = new Set();
const managementURI = /^window:\/\/datastudio\/[a-z][a-z0-9-]*$/;

export function isPublishedManagementWindowDirty(windowId) {return dirtyWindows.has(String(windowId || ''));}
export function confirmClosePublishedManagementWindow(windowId) {
  if (!isPublishedManagementWindowDirty(windowId)) return true;
  if (typeof window === 'undefined' || !window.confirm?.('Discard unsaved changes in this window?')) return false;
  dirtyWindows.delete(String(windowId));
  return true;
}
export function clearPublishedManagementWindowDirty(windowId) {dirtyWindows.delete(String(windowId || ''));}

export const downloadPublishedManagementFile = downloadWindowArchive;

export function createPublishedManagementHostWindow(windowState) {
  const uri = String(windowState?.resourceRef?.uri || '').trim();
  const providerIdentity = String(windowState?.providerIdentity || '').trim();
  if (!providerIdentity || !managementURI.test(uri)) return null;
  const windowId = String(windowState?.windowId || '');
  return {
    parameters: windowState?.parameters || {},
    navigate(section,parameters={}) {
      const target = managementWindowDestination(section);
      if (!target) throw new Error('Unsupported management window destination.');
      const destination=`window://datastudio/${target.name}`;
      const {discardConfirmed,...contentParameters}=parameters && typeof parameters==='object' && !Array.isArray(parameters) ? parameters : {};
      openWindow(destination,target.title,[],{
        providerIdentity,resourceRef:{uri:destination},
        parameters:{...(windowState?.parameters || {}),...contentParameters},
        conversationId:windowState?.conversationId,parentKey:windowState?.parentKey,
        presentation:windowState?.presentation,region:windowState?.region,
      });
      return true;
    },
    download:downloadPublishedManagementFile,
    setDirty(value) {if(value) dirtyWindows.add(windowId);else dirtyWindows.delete(windowId);},
    close() {if(!confirmClosePublishedManagementWindow(windowId)) return false;removeWindow(windowId);return true;},
  };
}
