import {describe,it,expect,vi,afterEach} from 'vitest';
import {readComposerDefaults,saveComposerDefaults} from './composerDefaults';
afterEach(()=>vi.unstubAllGlobals());
describe('composer defaults',()=>{it('isolates workspaces and preserves explicit defaults',()=>{const values=new Map();vi.stubGlobal('localStorage',{getItem:key=>values.get(key),setItem:(key,value)=>values.set(key,value)});saveComposerDefaults({workspaceId:'a'},{agent:'coder',model:'mercury'});expect(readComposerDefaults({workspaceId:'a'})).toEqual({agent:'coder',model:'mercury'});expect(readComposerDefaults({workspaceId:'b'})).toEqual({agent:'',model:''});});it('recovers from malformed stored data',()=>{vi.stubGlobal('localStorage',{getItem:()=>'{'});expect(readComposerDefaults({workspaceId:'a'})).toEqual({});});});
