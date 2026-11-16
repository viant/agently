// @vitest-environment jsdom
import React from 'react';
import {act,fireEvent,render,screen,waitFor,within,cleanup} from '@testing-library/react';
import {expect,test,vi,afterEach} from 'vitest';
import StarterTaskSettings from './StarterTaskSettings';
import {request} from '../services/httpClient';
import {getAuthMeSilently,refreshWorkspaceMetadata} from '../services/agentlyClient';

afterEach(()=>{cleanup();vi.clearAllMocks();});
const alpha='startertask://alpha/default#review';
const beta='startertask://beta/default#review';
vi.mock('../services/agentlyClient',()=>({getAuthMeSilently:vi.fn(async()=>({subject:'alice'})),refreshWorkspaceMetadata:vi.fn().mockResolvedValue({
  preferenceOwnerId:'alice-id',namespaceStarterTasks:{tasks:[
    {id:'startertask://alpha/default#review',title:'Alpha',prompt:'Review alpha.'},
    {id:'startertask://beta/default#review',title:'Beta',prompt:'Review beta.'},
  ]},
})}));
vi.mock('../services/httpClient',()=>({request:vi.fn().mockResolvedValue(null)}));

test('saves namespace task visibility and order alongside a personally authored task',async()=>{
  render(<StarterTaskSettings/>);
  fireEvent.click(await screen.findByRole('checkbox',{name:'Show Alpha'}));
  await waitFor(()=>expect(screen.getByRole('checkbox',{name:'Show Alpha'}).checked).toBe(false));
  fireEvent.click(screen.getByRole('button',{name:'Reorder'}));
  fireEvent.click(screen.getByRole('button',{name:'Move Beta up'}));
  await waitFor(()=>expect(within(screen.getByRole('list')).getAllByRole('listitem')[0].textContent).toContain('Beta'));
  fireEvent.click(screen.getByRole('button',{name:'Done reordering'}));
  fireEvent.click(screen.getByRole('button',{name:'New task'}));
  fireEvent.change(screen.getByLabelText('Task title'),{target:{value:'Own task'}});
  fireEvent.change(screen.getByLabelText('Prompt'),{target:{value:'Review my document.'}});
  fireEvent.click(screen.getByRole('button',{name:'Add personal task'}));
  await screen.findByText('Own task');
  expect(request).toHaveBeenCalledTimes(3);
  const [path, options]=request.mock.lastCall;
  expect(path).toBe('/v1/api/me/preferences');
  expect(options.method).toBe('PATCH');
  expect(JSON.parse(options.body)).toEqual({expectedOwnerId:'alice-id',starterTasks:{
    hidden:[alpha],order:[beta,alpha],own:[{id:expect.any(String),title:'Own task',prompt:'Review my document.'}],
  }});
});

test('customizes a published starting prompt as a personal task without editing the publisher',async()=>{
 render(<StarterTaskSettings/>);
 fireEvent.click(await screen.findByRole('button',{name:'Customize Alpha'}));
 expect(request).not.toHaveBeenCalled();
 fireEvent.change(screen.getByLabelText('Task title'),{target:{value:'My Alpha'}});
 fireEvent.change(screen.getByLabelText('Prompt'),{target:{value:'Review my chosen scope.'}});
 fireEvent.click(screen.getByRole('button',{name:'Save personal task'}));
 await screen.findByText('My Alpha');
 const patch=JSON.parse(request.mock.lastCall[1].body).starterTasks;
 expect(patch.hidden).toEqual([alpha]);
 expect(patch.own).toEqual([{id:expect.any(String),title:'My Alpha',prompt:'Review my chosen scope.'}]);
 expect(patch.order).toEqual([`personal#${patch.own[0].id}`,alpha,beta]);
 expect(request.mock.lastCall[0]).toBe('/v1/api/me/preferences');
 expect(screen.getByRole('checkbox',{name:'Show Alpha'}).checked).toBe(false);
});

test('retries a failed metadata load without writing preferences',async()=>{
 refreshWorkspaceMetadata.mockRejectedValueOnce(Object.assign(new Error('server unavailable'),{status:503}));
 render(<StarterTaskSettings/>);
 fireEvent.click(await screen.findByRole('button',{name:'Retry loading starting tasks'}));
 await screen.findByRole('checkbox',{name:'Show Alpha'});
 expect(screen.queryByRole('alert')).toBeNull();expect(request).not.toHaveBeenCalled();
});

test('discards metadata from a previous session after recovery',async()=>{
 let finish;
 refreshWorkspaceMetadata.mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve;}));
 render(<StarterTaskSettings/>);
 await act(async()=>window.dispatchEvent(new CustomEvent('agently:authorized')));
 await screen.findByRole('checkbox',{name:'Show Alpha'});
 await act(async()=>finish({preferenceOwnerId:'alice-id',namespaceStarterTasks:{tasks:[{id:'old',title:'Previous user',prompt:'Old'}]}}));
 expect(screen.queryByText('Previous user')).toBeNull();
 expect(screen.getByRole('checkbox',{name:'Show Alpha'})).toBeTruthy();
});

test('filters by source and reorders the complete list',async()=>{
 render(<StarterTaskSettings/>);
 const search=await screen.findByRole('searchbox',{name:'Find a starting task'});
 fireEvent.change(search,{target:{value:'beta'}});
 expect(screen.queryByRole('checkbox',{name:'Show Alpha'})).toBeNull();
 expect(screen.getByRole('checkbox',{name:'Show Beta'})).toBeTruthy();
 fireEvent.click(screen.getByRole('button',{name:'Reorder'}));
 expect(within(screen.getByRole('list')).getAllByRole('listitem')).toHaveLength(2);
 expect(search.disabled).toBe(true);
 expect(request).not.toHaveBeenCalled();
});

test('discards a previous account’s prompt before sending a preference patch',async()=>{
 let subject='alice';
 getAuthMeSilently.mockImplementation(async()=>({subject}));
 render(<StarterTaskSettings/>);
 fireEvent.click(await screen.findByRole('button',{name:'Customize Alpha'}));
 fireEvent.change(screen.getByLabelText('Prompt'),{target:{value:'A private draft'}});
 subject='bob';
 fireEvent.click(screen.getByRole('button',{name:'Save personal task'}));
 await waitFor(()=>expect(screen.queryByRole('dialog')).toBeNull());
 expect(request).not.toHaveBeenCalled();
});

test('hides a personal prompt from chat home while retaining its definition and position',async()=>{
 const own=[{id:'weekly',title:'Weekly review',prompt:'Review the week',description:'Weekly totals'}];
 const order=['personal#weekly',alpha];
 const metadata={preferenceOwnerId:'alice-id',namespaceStarterTasks:{tasks:[{id:alpha,title:'Alpha',prompt:'Alpha prompt'}]},starterTaskPreferences:{own,order}};
 refreshWorkspaceMetadata.mockResolvedValueOnce(metadata);
 render(<StarterTaskSettings/>);
 fireEvent.click(await screen.findByRole('checkbox',{name:'Show Weekly review'}));
 await waitFor(()=>expect(screen.getByRole('checkbox',{name:'Show Weekly review'}).checked).toBe(false));
 const saved=JSON.parse(request.mock.lastCall[1].body).starterTasks;
 expect(saved).toEqual({own,order,hidden:['personal#weekly']});
 const {starterTaskPreferenceEntries}=await import('../services/starterTaskPreferences');
 expect(starterTaskPreferenceEntries(metadata,saved).map(task=>task.title)).toEqual(['Alpha']);
 fireEvent.click(screen.getByRole('checkbox',{name:'Show Weekly review'}));
 await waitFor(()=>expect(screen.getByRole('checkbox',{name:'Show Weekly review'}).checked).toBe(true));
 const restored=JSON.parse(request.mock.lastCall[1].body).starterTasks;
 expect(starterTaskPreferenceEntries(metadata,restored).map(task=>task.title)).toEqual(['Weekly review','Alpha']);
});
