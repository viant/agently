import {activeWindows, removeWindow} from 'forge/core';
import {downloadWindowArchive} from 'forge/host-window/download';
import {managementWindowDestination} from 'forge/host-window/navigation';
import {openWindow} from '../components/menuBarHelpers.js';

const dirtyWindows = new Set();

export function isPublishedManagementWindowDirty(windowId) {return dirtyWindows.has(String(windowId || ''));}
export function confirmClosePublishedManagementWindow(windowId) {
  if (!isPublishedManagementWindowDirty(windowId)) return true;
  if (typeof window === 'undefined' || !window.confirm?.('Discard unsaved changes in this window?')) return false;
  dirtyWindows.delete(String(windowId));
  return true;
}
export function clearPublishedManagementWindowDirty(windowId) {dirtyWindows.delete(String(windowId || ''));}

export const downloadPublishedManagementFile = downloadWindowArchive;

export function createPublishedManagementHostWindow(windowState, navigation) {
  const source = navigation?.source;
  const routes = navigation?.routes;
  if (!source || !routes || source.uri !== windowState?.resourceRef?.uri ||
      !source.providerIdentity || source.providerIdentity !== windowState?.providerIdentity) return null;
  const windowId = String(windowState?.windowId || '');
  return {
    parameters: windowState?.parameters || {},
    navigate(section,parameters={}) {
      const target = managementWindowDestination(section, routes, source);
      if (!target) throw new Error('Unsupported management window destination.');
      const destination=target.resourceUri;
      const {discardConfirmed,...contentParameters}=parameters && typeof parameters==='object' && !Array.isArray(parameters) ? parameters : {};
      // A section renders its root and descendants. Drill into a child of the
      // displayed source. A declared link back to its parent reuses that window.
      const parent = activeWindows.peek().find(entry => entry.windowId === windowState.parentKey);
      const returning = parent?.resourceRef?.uri === destination && parent.providerIdentity === source.providerIdentity &&
        parent.region === windowState.region && parent.conversationId === windowState.conversationId;
      const origin = returning ? parent : windowState;
      const parentKey = !returning && origin.region === 'app.section' ? origin.windowId : origin.parentKey;
      openWindow(destination,target.title,[],{
        providerIdentity:source.providerIdentity,resourceRef:{uri:destination},
        parameters:{...(origin.parameters || {}),...contentParameters},
        conversationId:origin.conversationId,parentKey,
        presentation:origin.presentation,region:origin.region,
      });
      return true;
    },
    download:downloadPublishedManagementFile,
    setDirty(value) {if(value) dirtyWindows.add(windowId);else dirtyWindows.delete(windowId);},
    close() {if(!confirmClosePublishedManagementWindow(windowId)) return false;removeWindow(windowId);return true;},
  };
}
