// @vitest-environment jsdom
import React from 'react';
import {render,screen,fireEvent,waitFor,cleanup} from '@testing-library/react';
import {test,expect,vi,afterEach} from 'vitest';
import WorkspaceApplicationSettings from './WorkspaceApplicationSettings';
afterEach(cleanup);
test('saves app aliases and visibility using stable declared identities',async()=>{
 const onChange=vi.fn().mockResolvedValue(true);
 render(<WorkspaceApplicationSettings applications={[{id:'app',title:'Example',providerIdentity:'provider',menus:[]}]} preferences={{}} onChange={onChange}/>);
 fireEvent.click(screen.getByRole('checkbox',{name:'Show Example application tab'}));
 expect(onChange).toHaveBeenCalledWith({action:'set_application_visibility',applicationId:'["provider","app"]',hidden:true});
 fireEvent.click(screen.getByRole('button',{name:'Edit Application label for Example'}));
 fireEvent.change(screen.getByRole('textbox',{name:'Application label for Example'}),{target:{value:'My application'}});
 fireEvent.click(screen.getByRole('button',{name:'Save Application label for Example'}));
 await waitFor(()=>expect(onChange).toHaveBeenCalledWith({action:'rename_application',applicationId:'["provider","app"]',label:'My application'}));
});

test('pending visibility identity does not disable independent label editing',()=>{
 render(<WorkspaceApplicationSettings applications={[{id:'app',title:'Example',menus:[]}]} preferences={{}} visibilityDisabled onChange={vi.fn()}/>);
 expect(screen.getByRole('checkbox',{name:'Show Example application tab'}).disabled).toBe(true);
 expect(screen.getByRole('button',{name:'Edit Application label for Example'}).disabled).toBe(false);
});
