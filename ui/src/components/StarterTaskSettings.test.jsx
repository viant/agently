// @vitest-environment jsdom
import React from 'react';
import {fireEvent,render,screen,waitFor,within} from '@testing-library/react';
import {expect,test,vi} from 'vitest';
import StarterTaskSettings from './StarterTaskSettings';
import {request} from '../services/httpClient';

const alpha='startertask://alpha/default#review';
const beta='startertask://beta/default#review';
vi.mock('../services/agentlyClient',()=>({refreshWorkspaceMetadata:vi.fn().mockResolvedValue({
  namespaceStarterTasks:{tasks:[
    {id:'startertask://alpha/default#review',title:'Alpha',prompt:'Review alpha.'},
    {id:'startertask://beta/default#review',title:'Beta',prompt:'Review beta.'},
  ]},
})}));
vi.mock('../services/httpClient',()=>({request:vi.fn().mockResolvedValue(null)}));

test('saves namespace task visibility and order alongside a personally authored task',async()=>{
  render(<StarterTaskSettings/>);
  fireEvent.click(await screen.findByRole('checkbox',{name:'Show Alpha'}));
  await waitFor(()=>expect(screen.getByRole('checkbox',{name:'Show Alpha'}).checked).toBe(false));
  fireEvent.click(screen.getByRole('button',{name:'Move Beta up'}));
  await waitFor(()=>expect(within(screen.getByRole('list')).getAllByRole('listitem')[0].textContent).toContain('Beta'));
  fireEvent.change(screen.getByLabelText('Task title'),{target:{value:'Own task'}});
  fireEvent.change(screen.getByLabelText('Prompt'),{target:{value:'Review my document.'}});
  fireEvent.click(screen.getByRole('button',{name:'Add personal task'}));
  await screen.findByText('Own task');
  expect(request).toHaveBeenCalledTimes(3);
  const [path, options]=request.mock.lastCall;
  expect(path).toBe('/v1/api/me/preferences');
  expect(options.method).toBe('PATCH');
  expect(JSON.parse(options.body)).toEqual({starterTasks:{
    hidden:[alpha],order:[beta,alpha],own:[{id:expect.any(String),title:'Own task',prompt:'Review my document.'}],
  }});
});
