// @vitest-environment jsdom
import React from 'react';
import fs from 'node:fs';
import path from 'node:path';
import {act,cleanup,fireEvent,render,screen,waitFor} from '@testing-library/react';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
const reads=vi.hoisted(()=>({metadata:vi.fn(),me:vi.fn(),resource:vi.fn()}));
vi.mock('../services/agentlyClient',()=>({refreshWorkspaceMetadata:(...a)=>reads.metadata(...a),getAuthMeSilently:(...a)=>reads.me(...a),fetchWorkspaceStyleAsset:vi.fn()}));
vi.mock('../services/httpClient',()=>({request:(...a)=>reads.resource(...a)}));
vi.mock('../services/workspaceMetadata',()=>({subscribeWorkspaceMetadata:()=>()=>{}}));
vi.mock('forge/components',()=>({ForgeThemeProvider:({children})=>children,Typography:({as:Tag='span',typeRole,...props})=><Tag {...props}/>,Button:({variant,...props})=><button {...props}/>}));
import AppWorkspaceStyles from './AppWorkspaceStyles.jsx';
import {getWorkspaceStyleManager,workspaceStyleAccountKey} from '../services/workspaceStyles';
import {sdkBaseURL} from '../endpoint';
const catalog=JSON.parse(fs.readFileSync(path.resolve(process.env.AGENTLY_CORE_SOURCE_ROOT || '../../agently-core','protocol/ui/theme/testdata/baseline.json')));
catalog.defaultTheme='design-system-2';catalog.defaultMode='light';catalog.themes[0].id='design-system-2';
const globalStyle={resourceUri:'style://global/workspace',resourceConnection:'public',providerIdentity:'publisher',styleRevision:'one'};
const metadata={workspaceRoot:'workspace',uiStyleSource:'managed',globalStyle};
const principal={subject:'alice'};
const definition={schemaVersion:1,css:'.startup-proof{color:red}',themeCatalog:catalog};
const response=()=>({contents:[{uri:globalStyle.resourceUri,text:JSON.stringify(definition)}]});
const deferred=()=>{let resolve,reject;const promise=new Promise((a,b)=>{resolve=a;reject=b;});return {promise,resolve,reject};};
beforeEach(()=>{
 vi.clearAllMocks();window.history.replaceState(null,'','/');localStorage.clear();
 reads.me.mockResolvedValue(principal);reads.metadata.mockResolvedValue(metadata);reads.resource.mockResolvedValue(response());
});
afterEach(()=>{cleanup();getWorkspaceStyleManager().dispose();localStorage.clear();});
it('keeps app chrome inaccessible until metadata, Global CSS and the saved mode are ready',async()=>{
 const m=deferred(),r=deferred();reads.metadata.mockReturnValue(m.promise);reads.resource.mockReturnValue(r.promise);
 const account=workspaceStyleAccountKey(principal,sdkBaseURL);
 localStorage.setItem(`agently.theme.v1:${JSON.stringify([account,'workspace'])}`,JSON.stringify({themeId:'design-system-2',modePreference:'dark'}));
 render(<AppWorkspaceStyles><button>App action</button></AppWorkspaceStyles>);
 expect(screen.queryByRole('button',{name:'App action'})).toBeNull();
 expect(screen.getByRole('status').textContent).toContain('Loading workspace appearance');
 await act(async()=>m.resolve(metadata));await waitFor(()=>expect(reads.resource).toHaveBeenCalled());
 expect(screen.queryByRole('button',{name:'App action'})).toBeNull();
 await act(async()=>r.resolve(response()));
 expect(await screen.findByRole('button',{name:'App action'})).toBeTruthy();
 expect(document.documentElement.getAttribute('data-agently-theme')).toBe('design-system-2');
 expect(document.documentElement.getAttribute('data-agently-color-mode')).toBe('dark');
 expect(document.querySelector('style[data-application-resource-style]').textContent).toContain('.startup-proof');
});
it('hides chrome during sign-in recovery without remounting or losing its draft',async()=>{
 render(<AppWorkspaceStyles><input aria-label="Draft" defaultValue="Unsaved draft"/></AppWorkspaceStyles>);
 const input=await screen.findByRole('textbox',{name:'Draft'});fireEvent.change(input,{target:{value:'Keep this draft'}});
 const m=deferred();reads.metadata.mockReturnValue(m.promise);
 await act(async()=>window.dispatchEvent(new Event('agently:authorized')));
 expect(screen.queryByRole('textbox',{name:'Draft'})).toBeNull();
 await act(async()=>m.resolve({...metadata}));
 expect(await screen.findByRole('textbox',{name:'Draft'})).toBe(input);expect(input.value).toBe('Keep this draft');
});
it('honors another user-selected theme instead of forcing Design System 2.0',async()=>{
 const alternative=structuredClone(catalog.themes[0]);alternative.id='another-theme';alternative.label='Another theme';
 const available={...catalog,themes:[...catalog.themes,alternative]};
 const account=workspaceStyleAccountKey(principal,sdkBaseURL);
 localStorage.setItem(`agently.theme.v1:${JSON.stringify([account,'workspace'])}`,JSON.stringify({themeId:'another-theme',modePreference:'light'}));
 reads.resource.mockResolvedValue({contents:[{uri:globalStyle.resourceUri,text:JSON.stringify({...definition,themeCatalog:available})}]});
 render(<AppWorkspaceStyles><button>App action</button></AppWorkspaceStyles>);
 expect(await screen.findByRole('button',{name:'App action'})).toBeTruthy();
 expect(document.documentElement.getAttribute('data-agently-theme')).toBe('another-theme');
 expect(document.documentElement.getAttribute('data-agently-color-mode')).toBe('light');
});
it('does not reveal legacy chrome when Global CSS fails',async()=>{
 reads.resource.mockRejectedValue(new Error('Network unavailable'));
 render(<AppWorkspaceStyles><button>App action</button></AppWorkspaceStyles>);
 expect(await screen.findByRole('alert')).toBeTruthy();
 expect(screen.getByRole('button',{name:'Reload app'})).toBeTruthy();expect(screen.queryByRole('button',{name:'App action'})).toBeNull();
});
it('hides app chrome if its active theme catalog is withdrawn',async()=>{
 render(<AppWorkspaceStyles><button>App action</button></AppWorkspaceStyles>);
 expect(await screen.findByRole('button',{name:'App action'})).toBeTruthy();
 await act(async()=>getWorkspaceStyleManager().clearApplicationStyles());
 expect(screen.queryByRole('button',{name:'App action'})).toBeNull();
 expect(screen.getByRole('status').textContent).toContain('Loading workspace appearance');
});
it('offers metadata failure recovery rather than an endless loading screen',async()=>{
 reads.metadata.mockRejectedValue(new Error('Metadata unavailable'));
 render(<AppWorkspaceStyles><button>App action</button></AppWorkspaceStyles>);
 expect(await screen.findByRole('alert')).toBeTruthy();expect(screen.queryByRole('button',{name:'App action'})).toBeNull();
});
it('does not substitute the workspace default when account identity temporarily fails',async()=>{
 reads.me.mockRejectedValue(new Error('Identity request timed out'));
 render(<AppWorkspaceStyles><button>App action</button></AppWorkspaceStyles>);
 expect(await screen.findByRole('alert')).toBeTruthy();
 expect(screen.queryByRole('button',{name:'App action'})).toBeNull();expect(reads.resource).not.toHaveBeenCalled();
});
it('allows intentional unthemed workspaces after metadata resolves',async()=>{
 reads.metadata.mockResolvedValue({workspaceRoot:'plain'});
 render(<AppWorkspaceStyles><button>Baseline action</button></AppWorkspaceStyles>);
 expect(await screen.findByRole('button',{name:'Baseline action'})).toBeTruthy();expect(reads.resource).not.toHaveBeenCalled();
});
it('lets native sign-in run after an authoritative unauthorized event',async()=>{
 const pending=deferred();reads.metadata.mockReturnValue(pending.promise);
 render(<AppWorkspaceStyles><button>Native sign in</button></AppWorkspaceStyles>);
 await act(async()=>window.dispatchEvent(new Event('agently:unauthorized')));
 expect(screen.getByRole('button',{name:'Native sign in'})).toBeTruthy();
 await act(async()=>pending.resolve(metadata));expect(reads.resource).not.toHaveBeenCalled();
});
it('never holds the OAuth callback behind workspace metadata',()=>{
 window.history.replaceState(null,'','/v1/api/auth/oauth/callback');reads.metadata.mockReturnValue(new Promise(()=>{}));
 render(<AppWorkspaceStyles><button>OAuth callback</button></AppWorkspaceStyles>);
 expect(screen.getByRole('button',{name:'OAuth callback'})).toBeTruthy();
});
