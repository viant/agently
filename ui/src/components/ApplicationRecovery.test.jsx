// @vitest-environment jsdom
import React,{lazy,Suspense} from 'react';
import {act,cleanup,fireEvent,render,screen} from '@testing-library/react';
import {afterEach,expect,it,vi} from 'vitest';
import {createMemoryRouter,RouterProvider} from 'react-router-dom';
import {ApplicationRecovery,AssetLoadBoundary,ApplicationRouteError} from './ApplicationRecovery.jsx';

afterEach(()=>{cleanup();vi.restoreAllMocks();});
it('contains a failed lazy chunk without removing the surrounding conversation',async()=>{
 vi.spyOn(console,'error').mockImplementation(()=>{});
 const Missing=lazy(()=>Promise.reject(new TypeError('Failed to fetch dynamically imported module: https://example.invalid/assets/content-old.js')));
 const reload=vi.fn();
 render(<><input aria-label="Message" defaultValue="Unsaved draft"/><AssetLoadBoundary onReload={reload}><Suspense fallback="Loading"><Missing/></Suspense></AssetLoadBoundary></>);
 expect((await screen.findByRole('alert')).textContent).toContain('application files');
 expect(screen.getByRole('textbox',{name:'Message'}).value).toBe('Unsaved draft');
 expect(reload).not.toHaveBeenCalled();
 fireEvent.click(screen.getByRole('button',{name:'Reload app'}));
 expect(reload).toHaveBeenCalledTimes(1);
 expect(screen.queryByText(/content-old/)).toBeNull();
});
it('replaces the router developer stack with recoverable generic copy',async()=>{
 vi.spyOn(console,'error').mockImplementation(()=>{});
 function Broken(){throw new Error('private internal diagnostic');}
 const router=createMemoryRouter([{path:'/',element:<Broken/>,errorElement:<ApplicationRouteError/>}]);
 await act(async()=>{render(<RouterProvider router={router}/>);});
 expect(await screen.findByRole('button',{name:'Reload app'})).toBeTruthy();
 expect(screen.queryByText(/private internal diagnostic/)).toBeNull();
 expect(screen.queryByText(/Hey developer/)).toBeNull();
});
it('does not reload automatically for network or render errors',()=>{
 const reload=vi.fn();render(<ApplicationRecovery error={new Error('offline')} onReload={reload}/>);
 expect(reload).not.toHaveBeenCalled();
 expect(screen.getByRole('alert').textContent).toContain('could not');
});
