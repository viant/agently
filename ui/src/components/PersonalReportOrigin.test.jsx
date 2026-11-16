// @vitest-environment jsdom
import React from 'react';
import {act, cleanup, fireEvent, render, screen} from '@testing-library/react';
import {afterEach,expect,it,vi} from 'vitest';
import PersonalReportOrigin from './PersonalReportOrigin';
afterEach(cleanup);
const source={uri:'report://example/weekly',provider:'publisher',definition:{reportDocument:{title:'Old publisher label'}}};
function expand() {
 const details=screen.getByText('Personal copy · Source report').closest('details');
 details.open=true;fireEvent(details,new Event('toggle'));return details;
}
it('requests only admitted source identity on expansion and keeps no snapshot label',async()=>{
 const get=vi.fn().mockResolvedValue({resource:{uri:source.uri,providerIdentity:source.provider},title:'Current weekly report'});
 render(<PersonalReportOrigin source={source} getReport={get}/>);
 expect(get).not.toHaveBeenCalled();const details=expand();
 await screen.findByText('Based on Current weekly report');
 expect(get).toHaveBeenCalledWith({resource:{uri:source.uri},sourceProvider:source.provider,include:{}});
 act(()=>window.dispatchEvent(new Event('agently:session-reset')));
 expect(details.open).toBe(false);expect(screen.queryByText('Based on Current weekly report')).toBeNull();
 expect(screen.queryByText('Old publisher label')).toBeNull();
});
it.each([[403,'Original report unavailable.'],[503,'Could not load the original report.']])('keeps %i independent of the copy',async(status,message)=>{
 const get=vi.fn().mockRejectedValue(Object.assign(Error('private details'),{status}));
 render(<PersonalReportOrigin source={source} getReport={get}/>);expand();await screen.findByText(message);
 expect(!!screen.queryByRole('button',{name:'Retry'})).toBe(status===503);
 expect(screen.queryByText(/Old publisher label|private details/)).toBeNull();
});
it('does not show a late result after a session switch',async()=>{
 let finish;const get=vi.fn(()=>new Promise(resolve=>{finish=resolve}));
 render(<PersonalReportOrigin source={source} getReport={get}/>);const details=expand();
 await screen.findByText('Loading original report…');
 await act(async()=>{window.dispatchEvent(new Event('agently:session-reset'));finish({resource:{uri:source.uri,providerIdentity:source.provider},title:'Old user title'})});
 expect(details.open).toBe(false);expect(screen.queryByText(/Old user title/)).toBeNull();
});

it('discards the displayed source title at the end of its issued lease',async()=>{
 vi.useFakeTimers();
 try {
  const expires=new Date(Date.now()+60000).toISOString();
  const get=vi.fn().mockResolvedValue({resource:{uri:source.uri,providerIdentity:source.provider,validUntil:expires},title:'Current label'});
  render(<PersonalReportOrigin source={source} getReport={get}/>);
  const details=expand();await act(async()=>{});
  expect(screen.getByText('Based on Current label')).toBeTruthy();
  act(()=>vi.advanceTimersByTime(60000));
  expect(details.open).toBe(false);
  expect(screen.queryByText('Based on Current label')).toBeNull();
  expect(get).toHaveBeenCalledTimes(1);
 } finally { vi.useRealTimers(); }
});
