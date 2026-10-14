import { describe, expect, it, vi } from 'vitest';
vi.mock('./HostedAppRenderer.jsx', () => ({ default: function ExistingStandardAppRenderer() {} }));
import { HostedWindowApp } from './HostedWindowApp.jsx';

describe('installed hosted window rendering', () => {
 const container = {kind:'mcpApp',mcpApp:{resourceURI:'ui://owned-studio/datly-studio'}};
 const context = {windowState:{metadata:{resource:{uri:'window://authoring/DatlyStudio',providerIdentity:'owned-studio'},resourceTarget:{selectionToken:'server-selected-target'}}}};
 it('passes only resource identity to existing renderer', () => {
  const element=HostedWindowApp({container,context});
  expect(element.props.children.props.uri).toBe(container.mcpApp.resourceURI);
  expect(element.props.children.props.resourceContext.resourceTarget.selectionToken).toBe('server-selected-target');
  expect(element.props.children.props).not.toHaveProperty('rendererUrl');
  expect(element.props.children.props).not.toHaveProperty('headers');
 });
 it('keeps frame mounted when switching windows', () => {
  const element=HostedWindowApp({container,context,isActive:false});
  expect(element.props.style.display).toBe('none');
  expect(element.props.children.props.uri).toBe(container.mcpApp.resourceURI);
 });
 it('uses the current Forge context metadata rather than window navigation state', () => {
  const current = context.windowState.metadata;
  const element = HostedWindowApp({container,context:{metadata:current,windowState:{windowId:'opened-window'}}});
  expect(element.props.children.props.resourceContext.resource).toBe(current.resource);
  expect(element.props.children.props.resourceContext.resourceTarget).toBe(current.resourceTarget);
 });
 it('does not recover missing current authority from stale window metadata', () => {
  const element = HostedWindowApp({container,context:{metadata:{},windowState:context.windowState}});
  expect(element.props.role).toBe('alert');
 });
 it('rejects missing selected window authority', () => {
  expect(HostedWindowApp({container,context:{}}).props.role).toBe('alert');
  expect(HostedWindowApp({container,context:{windowState:{metadata:{resource:context.windowState.metadata.resource}}}}).props.role).toBe('alert');
 });
});
