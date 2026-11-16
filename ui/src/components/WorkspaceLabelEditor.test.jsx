// @vitest-environment jsdom
import React from 'react';
import {render,screen,fireEvent,waitFor,cleanup} from '@testing-library/react';
import {test,expect,vi,afterEach} from 'vitest';
import WorkspaceLabelEditor from './WorkspaceLabelEditor';
afterEach(cleanup);
test('retains an unsaved label after failure and resets explicitly to the original',async()=>{
 const onSave=vi.fn().mockResolvedValueOnce(false).mockResolvedValueOnce(true);
 render(<WorkspaceLabelEditor label="Workspace title" value="My desk" original="My workspace" onSave={onSave}/>);
 fireEvent.click(screen.getByRole('button',{name:'Edit Workspace title'}));
 fireEvent.change(screen.getByRole('textbox',{name:'Workspace title'}),{target:{value:'Review desk'}});
 fireEvent.click(screen.getByRole('button',{name:'Save Workspace title'}));
 await waitFor(()=>expect(onSave).toHaveBeenCalledWith('Review desk'));
 expect(screen.getByRole('textbox',{name:'Workspace title'}).value).toBe('Review desk');
 fireEvent.click(screen.getByRole('button',{name:'Save Workspace title'}));
 await waitFor(()=>expect(screen.queryByRole('textbox',{name:'Workspace title'})).toBeNull());
 fireEvent.click(screen.getByRole('button',{name:'Reset Workspace title'}));
 expect(onSave).toHaveBeenLastCalledWith('');
});
