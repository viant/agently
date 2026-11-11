// @vitest-environment jsdom
import React from 'react';
import {afterEach,expect,it,vi} from 'vitest';
import {cleanup,render,waitFor} from '@testing-library/react';
vi.mock('forge/components',()=>({ForgeThemeProvider:({children})=>children}));
import WorkspaceStyleProvider from './WorkspaceStyleProvider.jsx';
import {getWorkspaceStyleManager} from '../services/workspaceStyles';
afterEach(()=>{cleanup();getWorkspaceStyleManager().dispose();});
it('withdraws a managed global stylesheet when metadata includes its missing-publication diagnostic',async()=>{
 const globalStyle={resourceUri:'style://global/workspace',resourceConnection:'public',providerIdentity:'publisher',styleRevision:'one'};
 const readResource=vi.fn(async({uri})=>({contents:[{uri,text:JSON.stringify({schemaVersion:1,css:'.global-test{color:red}'})}]}));
 const fetchAsset=vi.fn(async()=>{throw new Error('local assets must not be read');});
 const props={accountKey:'reader',workspaceKey:'synthetic',readResource,fetchAsset};
 const view=render(<WorkspaceStyleProvider {...props} metadata={{uiStyleSource:'managed',globalStyle}}><span>Ready</span></WorkspaceStyleProvider>);
 await waitFor(()=>expect([...document.head.querySelectorAll('style')].some(node=>node.textContent.includes('.global-test'))).toBe(true));
 view.rerender(<WorkspaceStyleProvider {...props} metadata={{uiStyleSource:'managed',uiStyleDiagnostics:['No public global stylesheet is published.']}}><span>Ready</span></WorkspaceStyleProvider>);
 await waitFor(()=>expect([...document.head.querySelectorAll('style')].some(node=>node.textContent.includes('.global-test'))).toBe(false));
 expect(fetchAsset).not.toHaveBeenCalled();expect(readResource).toHaveBeenCalledTimes(1);
});
