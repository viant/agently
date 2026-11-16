// @vitest-environment jsdom
import {test,expect,afterEach} from 'vitest';
import {activeWindows,selectedWindowId,selectedTabId} from 'forge/core';
import {openWorkspaceSettingsWindow} from './workspaceSettingsWindow';
import {isWorkspaceSettingsWindow} from './workspaceSettingsIdentity';
afterEach(()=>{activeWindows.value=[];selectedWindowId.value=null;selectedTabId.value=null;});
test('opens a reusable native management window while retaining the conversation and original report',()=>{
 const chat={windowId:'chat/new',windowKey:'chat/new',parameters:{conversations:{form:{id:'conversation-1'}}}};
 const report={windowId:'original-report',windowKey:'report://example/overview',resourceRef:{uri:'report://example/overview',revision:'7'},conversationId:'conversation-1'};
 activeWindows.value=[chat,report];
 const first=openWorkspaceSettingsWindow('conversation-1');
 expect(isWorkspaceSettingsWindow(first)).toBe(true);
 expect(first.conversationId).toBe('conversation-1');
 expect(first.workspaceObject.content.renderer).toBe('workspaceSettings');
 expect(first.inlineMetadata.dataSource).toEqual({});
 expect(first.resourceRef).toBeUndefined();
 expect(activeWindows.value.find(row=>row.windowId==='original-report')).toEqual(report);
 expect(activeWindows.value.find(row=>row.windowId==='chat/new')).toEqual(chat);
 const repeated=openWorkspaceSettingsWindow('conversation-1');
 expect(repeated.windowId).toBe(first.windowId);
 expect(activeWindows.value).toHaveLength(3);
 expect(selectedWindowId.value).toBe(first.windowId);
});

test('keeps management selected when transcript restoration reconciles an older report',async()=>{
 const {renderHook,act,cleanup}=await import('@testing-library/react');
 const {useWorkspaceAdapters}=await import('./useWorkspaceAdapters');
 const {setScopedWorkspaceSelection,setScopedActiveSurface,getScopedWorkspaceSelection}=await import('./conversationWindow');
 const conversationId='settings-restoration-test';
 const object={version:1,objectId:'historical-report',revision:1,conversationId,kind:'report',content:{renderer:'inlineReport',windowId:'historical-report',windowKey:'report://example/summary'},lifecycle:{state:'ready'}};
 const projectedRows=[{kind:'assistant',attachments:[{kind:'workspaceObject',objectId:object.objectId,workspaceObject:object}]}];
 activeWindows.value=[{windowId:'historical-report',windowKey:'report://example/summary',conversationId,workspaceObject:object}];
 setScopedWorkspaceSelection(conversationId,'historical-report');setScopedActiveSurface(conversationId,'workspace');
 const options={mainConversationId:conversationId,workspaceWindows:[],projectedRows,documentPresentation:true,active:true,setActiveSurface:()=>{},setWorkspacePresentationMode:()=>{}};
 const hook=renderHook(props=>useWorkspaceAdapters(props),{initialProps:options});
 let settings;
 act(()=>{settings=openWorkspaceSettingsWindow(conversationId);});
 hook.rerender({...options,projectedRows:[...projectedRows]});
 expect(selectedWindowId.peek()).toBe(settings.windowId);
 expect(selectedTabId.peek()).toBe(settings.windowId);
 expect(getScopedWorkspaceSelection(conversationId)).toBe(settings.windowId);
 cleanup();
});

test('selecting an existing chat after opening landing settings focuses that chat without adopting settings',async()=>{
 const {openConversationInMainWindow,getScopedWorkspaceSelection,setScopedActiveSurface,MAIN_CHAT_WINDOW_ID}=await import('./conversationWindow');
 activeWindows.value=[{windowId:MAIN_CHAT_WINDOW_ID,windowKey:MAIN_CHAT_WINDOW_ID,parameters:{}}];
 const settings=openWorkspaceSettingsWindow('');
 expect(selectedWindowId.peek()).toBe(settings.windowId);
 setScopedActiveSurface('existing-chat','conversation');
 openConversationInMainWindow('existing-chat');
 expect(selectedWindowId.peek()).toBe(MAIN_CHAT_WINDOW_ID);
 expect(selectedTabId.peek()).toBe(MAIN_CHAT_WINDOW_ID);
 const retained=activeWindows.peek().find(entry=>entry.windowId===settings.windowId);
 expect(retained?.conversationId).toBeUndefined();
 expect(getScopedWorkspaceSelection('existing-chat')).not.toBe(settings.windowId);
});

test('restoring saved management retains its native renderer and window identity',async()=>{
 const {setScopedWorkspaceState,setScopedActiveSurface,ensureWorkspaceWindowForConversation,reopenWorkspaceForConversation,MAIN_CHAT_WINDOW_ID}=await import('./conversationWindow');
 const conversation='restore-settings-renderer';
 activeWindows.value=[{windowId:MAIN_CHAT_WINDOW_ID,windowKey:MAIN_CHAT_WINDOW_ID,parameters:{}}];
 const settings=openWorkspaceSettingsWindow(conversation);
 setScopedWorkspaceState(conversation,settings);
 setScopedActiveSurface(conversation,'workspace');
 activeWindows.value=activeWindows.peek().filter(entry=>entry.windowId!==settings.windowId);
 selectedWindowId.value=MAIN_CHAT_WINDOW_ID;selectedTabId.value=MAIN_CHAT_WINDOW_ID;
 const restored=ensureWorkspaceWindowForConversation(conversation);
 expect(restored.windowId).toBe(settings.windowId);
 expect(restored.conversationId).toBe(conversation);
 expect(isWorkspaceSettingsWindow(restored)).toBe(true);
 expect(restored.workspaceObject.content.renderer).toBe('workspaceSettings');
 expect(selectedWindowId.peek()).toBe(MAIN_CHAT_WINDOW_ID);
 const reopened=reopenWorkspaceForConversation(conversation);
 expect(reopened.windowId).toBe(settings.windowId);
 expect(selectedWindowId.peek()).toBe(settings.windowId);
});
