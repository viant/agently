import {expect, it, vi} from 'vitest';
import {createReportChoiceLoader} from './reportChoiceService';
import {APIRequestError} from './httpClient';

it('uses generic datasource fetch with a logical report owner and no additional MCP operation', async () => {
  const request = vi.fn(async () => new Response(JSON.stringify({rows: [{value: 0, label: 'Zero'}]})));
  const load = createReportChoiceLoader({uri: 'report://example/summary', providerIdentity: 'owner', revision: '7'}, request);
  const signal = new AbortController().signal;
  expect((await load('options', {scope: 0}, {signal})).data).toEqual([{value: 0, label: 'Zero'}]);
  expect(request).toHaveBeenCalledOnce();
  expect(request.mock.calls[0][0]).toBe('/v1/api/datasources/options/fetch');
  expect(JSON.parse(request.mock.calls[0][1].body)).toEqual({owner: {uri: 'report://example/summary'}, inputs: {scope: 0}});
  expect(request.mock.calls[0][1]).toMatchObject({credentials: 'include', signal});
  expect(request.mock.calls.every(([url]) => !url.includes('reports/choices') && !url.includes('/tools/'))).toBe(true);
});

it('does not fall back to a separate tool or standalone datasource when the owner fetch fails', async () => {
  const request = vi.fn(async () => new Response('{}', {status: 403}));
  await expect(createReportChoiceLoader({uri: 'report://example/summary'}, request)('options', {})).rejects.toThrow('403');
  expect(request).toHaveBeenCalledOnce();
});

it.each([403,503])('preserves structured datasource failure at status %s without exposing its body as the message', async status => {
  const payload = {code: status === 403 ? 'datasource_denied' : 'dependency_unavailable', scope: 'dataset', error: 'Synthetic producer diagnostic'};
  const request = vi.fn(async () => new Response(JSON.stringify(payload), {status}));
  const load = createReportChoiceLoader({uri:'report://example/summary'}, request);
  const error = await load('calendar', {ids:[17]}).catch(error => error);
  expect(error).toBeInstanceOf(APIRequestError);
  expect(error).toMatchObject({status,payload,method:'POST',path:'/api/datasources/calendar/fetch'});
  expect(error.message).not.toContain(payload.error);
  expect(request).toHaveBeenCalledOnce();
});

it('preserves cancellation identity without converting it into a datasource denial', async () => {
  const canceled = new DOMException('Synthetic cancellation','AbortError');
  const request = vi.fn(async () => {throw canceled;});
  await expect(createReportChoiceLoader({uri:'report://example/summary'},request)('calendar',{})).rejects.toBe(canceled);
});
