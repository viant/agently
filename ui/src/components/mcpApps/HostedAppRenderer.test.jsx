// @vitest-environment jsdom
import React from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';

const bridge = vi.hoisted(() => ({
  connect: vi.fn(),
  close: vi.fn(async () => {}),
  sendToolInput: vi.fn(async () => {}),
}));

vi.mock('@modelcontextprotocol/ext-apps/app-bridge', () => ({
  AppBridge: class {
    connect = bridge.connect;
    close = bridge.close;
    sendToolInput = bridge.sendToolInput;
  },
  PostMessageTransport: class {
    constructor(readTarget, writeTarget) {
      this.readTarget = readTarget;
      this.writeTarget = writeTarget;
    }
  },
}));

vi.mock('../../services/mcpApps/resourceLoader.js', () => ({
  readMCPUIResource: vi.fn(async () => ({})),
  resolveMCPUIFrameConfig: vi.fn(() => ({
    rendererUrl: 'https://studio.example.test/app',
    sandbox: 'allow-scripts allow-same-origin allow-forms allow-popups',
  })),
}));

vi.mock('./AppFrame.jsx', async () => {
  const { default: React } = await import('react');
  return {
    default: React.forwardRef(({ src, onLoad, sandbox }, ref) => {
      React.useImperativeHandle(ref, () => ({ contentWindow: window }), []);
      return <button data-testid="hosted-frame" data-src={src || ''} data-sandbox={sandbox} onClick={onLoad}>frame</button>;
    }),
  };
});

import HostedAppRenderer from './HostedAppRenderer.jsx';
import { readMCPUIResource } from '../../services/mcpApps/resourceLoader.js';

beforeEach(() => {
  bridge.connect.mockReset().mockResolvedValue(undefined);
  vi.mocked(readMCPUIResource).mockReset().mockResolvedValue({});
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe('hosted MCP app connection', () => {
  it('loads the selected renderer before connecting its bridge', async () => {
    bridge.connect.mockImplementation(() => new Promise(() => {}));
    render(<HostedAppRenderer uri="ui://studio/window" resourceContext={{ resource: { uri: 'window://studio/window' } }} />);

    await waitFor(() => expect(screen.getByTestId('hosted-frame').dataset.src).toBe('https://studio.example.test/app'));
    expect(bridge.connect).not.toHaveBeenCalled();

    fireEvent.click(screen.getByTestId('hosted-frame'));
    await waitFor(() => expect(bridge.connect).toHaveBeenCalledTimes(1));
    const transport = bridge.connect.mock.calls[0][0];
    expect(transport.readTarget).toBe(window);
    expect(transport.writeTarget).toBe(window);
  });

  it('ignores the initial blank frame load while waiting for the resource', async () => {
    let resolveRead;
    vi.mocked(readMCPUIResource).mockImplementationOnce(() => new Promise(resolve => { resolveRead = resolve; }));
    render(<HostedAppRenderer uri="ui://studio/window" resourceContext={{ resource: { uri: 'window://studio/window' } }} />);

    fireEvent.click(screen.getByTestId('hosted-frame'));
    expect(bridge.connect).not.toHaveBeenCalled();

    resolveRead({});
    await waitFor(() => expect(screen.getByTestId('hosted-frame').dataset.src).toBe('https://studio.example.test/app'));
    expect(bridge.connect).not.toHaveBeenCalled();

    fireEvent.click(screen.getByTestId('hosted-frame'));
    await waitFor(() => expect(bridge.connect).toHaveBeenCalledTimes(1));
  });
});
