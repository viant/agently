// @vitest-environment jsdom
import React from 'react';
import {render, screen, fireEvent, cleanup} from '@testing-library/react';
import {afterEach, expect, it, vi} from 'vitest';
import {ConversationViewContext} from '../../context/ConversationViewContext';

const theme = vi.hoisted(() => ({id: 'design-system-2'}));
vi.mock('../WorkspaceStyleProvider', () => ({useWorkspaceStyle: () => ({state: {themeId: theme.id}})}));
import IterationBlock from './IterationBlock.jsx';

afterEach(() => {cleanup(); theme.id = 'design-system-2';});
const row = {
  kind: 'iteration', turnId: 'example-turn', lifecycle: 'completed',
  rounds: ['completed', 'failed', 'running', 'waiting_for_user'].map((status, index) => ({
    pageId: `example-page-${index}`, status,
    modelSteps: [{modelCallId: `example-model-${index}`, status}],
    toolCalls: [{toolCallId: `example-tool-${index}`, toolName: 'example/read', status}],
  })),
};

it('uses status glyphs in DS2 working steps and preserves legacy document glyphs', () => {
  const content = () => <ConversationViewContext.Provider value={{presentation: 'document', developerMode: true}}>
    <IterationBlock canonicalRow={row} message={{id: 'example-message'}}/>
  </ConversationViewContext.Provider>;
  const view = render(content());
  const toggle = screen.getByRole('button', {name: /Worked for|Work completed|Show working steps/});
  fireEvent.click(toggle);
  expect(view.container.querySelectorAll('.app-iteration-model-icon .lucide-check').length).toBeGreaterThan(0);
  expect(view.container.querySelectorAll('.app-iteration-tool-icon .lucide-circle-alert').length).toBeGreaterThan(0);
  expect(view.container.querySelector('.app-iteration-tool-icon .lucide-loader-circle')).toBeTruthy();
  expect(view.container.querySelector('.app-iteration-model-icon .lucide-message-square')).toBeNull();
  theme.id = 'baseline';
  view.rerender(content());
  expect(view.container.querySelector('.app-iteration-model-icon .lucide-message-square')).toBeTruthy();
  expect(view.container.querySelector('.app-iteration-tool-icon .lucide-sliders-horizontal')).toBeTruthy();
});
