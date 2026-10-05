import { describe, expect, it, vi } from 'vitest';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
const effects = vi.hoisted(() => ({ feeds: vi.fn(), subscribe: vi.fn(), datasource: vi.fn(), export: vi.fn(), action: vi.fn(), auth: vi.fn() }));
vi.mock('../../services/toolFeedBus', async () => ({ ...await vi.importActual('../../services/toolFeedBus'), getActiveFeeds: effects.feeds, onFeedChange: effects.subscribe }));
vi.mock('../lookups/client.js', async () => ({ ...await vi.importActual('../lookups/client.js'), fetchDatasource: effects.datasource }));
vi.mock('../../services/reportExportService.js', async () => ({ ...await vi.importActual('../../services/reportExportService.js'), submitReportExportRequest: effects.export }));
vi.mock('../../services/forgeUIActions.js', async () => ({ ...await vi.importActual('../../services/forgeUIActions.js'), dispatchForgeUIAction: effects.action }));
vi.mock('../../services/mcpAuth', async () => ({ ...await vi.importActual('../../services/mcpAuth'), beginBrowserMCPAuth: effects.auth }));
vi.mock('./LazyRichContent', async () => ({ default: (await vi.importActual('./RichContent')).default }));
import RichContent from './RichContent';
import IterationBlock from './IterationBlock';
import BubbleMessage from './BubbleMessage';
const text = '```forge-ui\n{"dataSources":{"private":{"service":"/v1/api/private"}},"blocks":[]}\n```\n[private file](sandbox:/secret)';
const noEffects = () => { for(const effect of Object.values(effects)) expect(effect).not.toHaveBeenCalled(); };
describe('standard backend host isolation', () => {
  it('renders Forge/files as literal text without native services or active links', () => {
    const html=renderToStaticMarkup(<RichContent content={text} allowHostEffects={false} generatedFiles={[{name:'secret',url:'/v1/private'}]} />);
    expect(html).toContain('forge-ui'); expect(html).toContain('sandbox:/secret'); expect(html).not.toContain('<a '); expect(html).not.toContain('app-forge'); noEffects();
  });
  it('selects passive iteration before feed, execution, MCP, payload and linked hooks mount', () => {
    const html=renderToStaticMarkup(<IterationBlock canonicalRow={{kind:'iteration',connectionProfile:'standard',hostEffectsAllowed:false,turnId:'foreign',rounds:[{pageId:'p',content:text,toolCalls:[{toolCallId:'t',toolName:'mcp:private',uiResourceUri:'ui://private',requestPayloadId:'private-request',responsePayloadId:'private-result',linkedConversationId:'private',status:'completed'}]}],elicitations:[{status:'pending'}]}} />);
    expect(html).toContain('Execution details'); expect(html).toContain('mcp:private'); expect(html).toContain('forge-ui'); expect(html).not.toContain('<iframe'); expect(html).not.toContain('<button'); noEffects();
  });
  it('keeps standalone bubbles passive and excludes native attachments', () => {
    const html=renderToStaticMarkup(<BubbleMessage message={{role:'assistant',content:text,hostEffectsAllowed:false,connectionProfile:'standard'}} attachment={<button>Native host attachment</button>} />);
    expect(html).toContain('forge-ui'); expect(html).not.toContain('Native host attachment'); noEffects();
  });
});
