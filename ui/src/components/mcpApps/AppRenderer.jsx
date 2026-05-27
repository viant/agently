import React, { useEffect, useMemo, useRef, useState } from 'react';
import AppFrame from './AppFrame.jsx';
import { buildSandboxedSrcDoc, readMCPUIResource, resolveMCPUIFrameConfig } from '../../services/mcpApps/resourceLoader.js';
import { buildEnvelope, MCPUI_METHODS, MCPUI_VERSION, validateEnvelope } from '../../services/mcpApps/appproto.js';
import { dispatchMCPUIApprovalRequest, subscribeMCPUIApprovalOutcomes } from '../../services/mcpApps/approvalEvents.js';
import { handleGuestEnvelope } from '../../services/mcpApps/bridge.js';
import { acceptsGuestMessage, hasSameOriginSandbox } from '../../services/mcpApps/hostAcceptance.js';

export function reduceToolResultState(previousState = {}, payload = {}, title = '') {
  const structuredContent = payload?.params?.structuredContent || {};
  const approvalId = String(structuredContent?.approval?.id || '').trim();
  const toolCallStatus = String(structuredContent?.status || '').trim();
  return {
    ...previousState,
    conversationId: String(structuredContent?.conversationId || previousState?.conversationId || '').trim(),
    toolCallStatus,
    approvalId,
    toolResult: String(payload?.params?.content?.[0]?.text || '').trim(),
    pendingApproval: toolCallStatus === 'queued' && approvalId !== '',
    pendingApprovalTitle: approvalId ? (String(title || '').trim() || 'Approval required') : '',
  };
}

export function reduceApprovalOutcomeState(previousState = {}, outcome = {}) {
  const nextStatus = String(outcome?.status || outcome?.decision || '').trim();
  const nextResult = String(outcome?.result || '').trim();
  const nextError = String(outcome?.errorMessage || '').trim();
  return {
    ...previousState,
    conversationId: String(outcome?.conversationId || previousState?.conversationId || '').trim(),
    toolCallStatus: nextStatus || previousState?.toolCallStatus || '',
    approvalId: String(outcome?.approvalId || previousState?.approvalId || '').trim(),
    toolResult: nextResult || nextError || previousState?.toolResult || '',
    pendingApproval: false,
    pendingApprovalTitle: '',
    error: nextStatus === 'failed' && nextError ? nextError : previousState?.error || '',
  };
}

export function buildInitialHostNotifications({
  windowId = '',
  resourceUri = '',
  allowedTools = [],
  allowedToolBundles = [],
  protocolVersion = MCPUI_VERSION,
  toolInput = null,
  toolInputPartial = null,
} = {}) {
  const notifications = [
    buildEnvelope(MCPUI_METHODS.HOST_READY, {
      windowId,
      resourceUri,
      allowedTools,
      allowedToolBundles,
      protocolVersion,
    }),
  ];
  if (toolInput && typeof toolInput === 'object') {
    notifications.push(buildEnvelope(MCPUI_METHODS.TOOL_INPUT, {
      windowId,
      resourceUri,
      input: toolInput,
    }));
  }
  if (toolInputPartial && typeof toolInputPartial === 'object') {
    notifications.push(buildEnvelope(MCPUI_METHODS.TOOL_INPUT_PARTIAL, {
      windowId,
      resourceUri,
      input: toolInputPartial,
    }));
  }
  return notifications;
}

function bootstrapSameOriginHostState(targetWindow, {
  windowId = '',
  resourceUri = '',
  allowedTools = [],
  allowedToolBundles = [],
  protocolVersion = MCPUI_VERSION,
} = {}) {
  if (!targetWindow || typeof targetWindow !== 'object') return;
  try {
    targetWindow.__mcpuiHost = {
      windowId,
      resourceUri,
      allowedTools,
      allowedToolBundles,
      protocolVersion,
    };
    targetWindow.dispatchEvent?.(new CustomEvent('mcpui-host-ready', { detail: targetWindow.__mcpuiHost }));
  } catch (_) {}
}

export function buildTeardownNotification({ windowId = '', resourceUri = '' } = {}) {
  return buildEnvelope(MCPUI_METHODS.TEARDOWN, {
    windowId,
    resourceUri,
  });
}

export function acceptGuestEnvelopeEvent(event, {
  targetWindow = null,
  windowId = '',
  resourceUri = '',
  sandbox = '',
  pageOrigin = '',
} = {}) {
  if (!targetWindow || event?.source !== targetWindow) {
    return { ok: false, error: 'source mismatch' };
  }
  const parsed = validateEnvelope(event?.data);
  if (!parsed.ok) {
    return parsed;
  }
  const params = parsed.envelope?.params || {};
  if (String(params.windowId || '').trim() !== String(windowId || '').trim()) {
    return { ok: false, error: 'windowId mismatch' };
  }
  if (String(params.resourceUri || '').trim() !== String(resourceUri || '').trim()) {
    return { ok: false, error: 'resourceUri mismatch' };
  }
  const sameOriginFrame = String(sandbox || '').includes('allow-same-origin');
  const origin = String(event?.origin || '').trim();
  if (sameOriginFrame) {
    if (origin !== String(pageOrigin || '').trim()) {
      return { ok: false, error: 'origin mismatch' };
    }
  } else if (origin !== 'null') {
    return { ok: false, error: 'opaque origin mismatch' };
  }
  return { ok: true, envelope: parsed.envelope };
}

export function buildApprovalOutcomeToolResultEnvelope({ windowId = '', resourceUri = '', outcome = {}, protocolVersion = MCPUI_VERSION } = {}) {
  const normalizedStatus = String(outcome?.status || outcome?.decision || '').trim();
  const resultText = String(outcome?.result || outcome?.errorMessage || '').trim();
  return buildEnvelope(MCPUI_METHODS.TOOL_RESULT, {
    windowId,
    resourceUri,
    toolName: String(outcome?.toolName || '').trim(),
    content: resultText ? [{ type: 'text', text: resultText }] : [],
    structuredContent: {
      approval: { id: String(outcome?.approvalId || '').trim() },
      action: String(outcome?.action || '').trim(),
      decision: String(outcome?.decision || '').trim(),
      status: normalizedStatus,
      conversationId: String(outcome?.conversationId || '').trim(),
      turnId: String(outcome?.turnId || '').trim(),
      messageId: String(outcome?.messageId || '').trim(),
      toolName: String(outcome?.toolName || '').trim(),
      result: String(outcome?.result || '').trim(),
      errorMessage: String(outcome?.errorMessage || '').trim(),
    },
    _meta: {},
    protocolVersion,
  });
}

export default function AppRenderer({ uri = '', title = 'MCP UI Preview', toolInput = null, toolInputPartial = null }) {
  const frameRef = useRef(null);
  const windowId = useMemo(() => `mcpui-preview:${String(uri || '').trim()}`, [uri]);
  const [frameLoaded, setFrameLoaded] = useState(false);
  const [state, setState] = useState({
    loading: true,
    error: '',
    srcDoc: '',
    src: '',
    sandbox: 'allow-scripts',
    allowedTools: [],
    allowedToolBundles: [],
    conversationId: '',
    protocolVersion: MCPUI_VERSION,
    messages: [],
    openLink: '',
    toolCallStatus: '',
    approvalId: '',
    toolResult: '',
    pendingApproval: false,
    pendingApprovalTitle: '',
  });

  useEffect(() => {
    let active = true;
    setState((prev) => ({
      ...prev,
      loading: true,
      error: '',
      srcDoc: '',
      src: '',
      sandbox: 'allow-scripts',
      messages: [],
      allowedTools: [],
      allowedToolBundles: [],
      conversationId: '',
      openLink: '',
      toolCallStatus: '',
      approvalId: '',
      toolResult: '',
      pendingApproval: false,
      pendingApprovalTitle: '',
    }));
    readMCPUIResource(uri)
      .then((payload) => {
        if (!active) return;
        const uiMeta = payload?._meta?.ui || {};
        const frame = resolveMCPUIFrameConfig(payload);
        setState({
          loading: false,
          error: '',
          srcDoc: buildSandboxedSrcDoc({
            html: payload.text,
            title,
            csp: String(uiMeta.csp || '').trim(),
            cspPolicy: uiMeta.cspPolicy || null,
          }),
          src: frame.rendererUrl,
          sandbox: frame.sandbox,
          allowedTools: Array.isArray(uiMeta.allowedTools) ? uiMeta.allowedTools : [],
          allowedToolBundles: Array.isArray(uiMeta.allowedToolBundles) ? uiMeta.allowedToolBundles : [],
          conversationId: '',
          protocolVersion: String(uiMeta.protocolVersion || MCPUI_VERSION).trim() || MCPUI_VERSION,
          messages: [],
          openLink: '',
          toolCallStatus: '',
          approvalId: '',
          toolResult: '',
          pendingApproval: false,
          pendingApprovalTitle: '',
        });
        setFrameLoaded(false);
      })
      .catch((err) => {
        if (!active) return;
        setState({
          loading: false,
          error: err?.message || 'Failed to load MCP UI resource',
          srcDoc: '',
          src: '',
          sandbox: 'allow-scripts',
          allowedTools: [],
          allowedToolBundles: [],
          conversationId: '',
          protocolVersion: MCPUI_VERSION,
          messages: [],
          openLink: '',
          toolCallStatus: '',
          approvalId: '',
          toolResult: '',
          pendingApproval: false,
          pendingApprovalTitle: '',
        });
        setFrameLoaded(false);
      });
    return () => {
      active = false;
    };
  }, [uri, title]);

  useEffect(() => {
    if (!frameLoaded || (!state.srcDoc && !state.src) || !frameRef.current?.contentWindow) return;
    const targetWindow = frameRef.current.contentWindow;
    const notifications = buildInitialHostNotifications({
      windowId,
      resourceUri: uri,
      allowedTools: state.allowedTools,
      allowedToolBundles: state.allowedToolBundles,
      protocolVersion: state.protocolVersion,
      toolInput,
      toolInputPartial,
    });
    if (state.src && String(state.sandbox || '').includes('allow-same-origin')) {
      bootstrapSameOriginHostState(targetWindow, {
        windowId,
        resourceUri: uri,
        allowedTools: state.allowedTools,
        allowedToolBundles: state.allowedToolBundles,
        protocolVersion: state.protocolVersion,
      });
    }
    notifications.forEach((notification) => targetWindow.postMessage(notification, '*'));
  }, [frameLoaded, state.srcDoc, state.src, state.sandbox, state.allowedTools, state.allowedToolBundles, state.protocolVersion, uri, windowId, toolInput, toolInputPartial]);

  useEffect(() => () => {
    const targetWindow = frameRef.current?.contentWindow;
    if (!targetWindow) return;
    try {
      targetWindow.postMessage(buildTeardownNotification({ windowId, resourceUri: uri }), '*');
    } catch (_) {}
  }, [uri, windowId]);

  useEffect(() => subscribeMCPUIApprovalOutcomes((outcome) => {
    if (String(outcome?.approvalId || '').trim() === '' || String(outcome?.approvalId || '').trim() !== String(state.approvalId || '').trim()) {
      return;
    }
    setState((prev) => reduceApprovalOutcomeState(prev, outcome));
    frameRef.current?.contentWindow?.postMessage(
      buildApprovalOutcomeToolResultEnvelope({
        windowId,
        resourceUri: uri,
        outcome,
        protocolVersion: state.protocolVersion,
      }),
      '*',
    );
  }), [state.approvalId, state.protocolVersion, uri, windowId]);

  useEffect(() => {
    async function onMessage(event) {
      const accepted = acceptGuestEnvelopeEvent(event, {
        targetWindow: frameRef.current?.contentWindow || null,
        windowId,
        resourceUri: uri,
        sandbox: state.sandbox,
        pageOrigin: typeof window !== 'undefined' ? window.location.origin : '',
      });
      if (!accepted.ok) return;
      try {
        const outcome = await handleGuestEnvelope(accepted.envelope, {
          allowedTools: state.allowedTools,
          allowedToolBundles: state.allowedToolBundles,
          conversationId: state.conversationId,
          protocolVersion: state.protocolVersion,
        });
        if (outcome.type === 'message') {
          setState((prev) => ({ ...prev, messages: [...prev.messages, String(outcome.payload.content || '').trim()] }));
          return;
        }
        if (outcome.type === 'open-link') {
          setState((prev) => ({ ...prev, openLink: String(outcome.payload.url || '').trim() }));
          return;
        }
        if (outcome.type === 'tool-result') {
          setState((prev) => reduceToolResultState(prev, outcome.payload, title));
          const approvalId = String(outcome.payload.params?.structuredContent?.approval?.id || '').trim();
          const toolCallStatus = String(outcome.payload.params?.structuredContent?.status || '').trim();
          if (toolCallStatus === 'queued' && approvalId) {
            dispatchMCPUIApprovalRequest({
              approvalId,
              resourceUri: uri,
              toolName: String(outcome.payload.params?.toolName || '').trim(),
              title: String(title || '').trim(),
            });
          }
          frameRef.current?.contentWindow?.postMessage(outcome.payload, '*');
        }
      } catch (err) {
        setState((prev) => ({ ...prev, error: err?.message || 'Guest action failed' }));
      }
    }
    window.addEventListener('message', onMessage);
    return () => window.removeEventListener('message', onMessage);
  }, [state.allowedToolBundles, state.allowedTools, state.conversationId, state.protocolVersion]);

  if (state.loading) {
    return <div data-testid="mcpui-loading">Loading MCP UI resource...</div>;
  }
  if (state.error) {
    return <div data-testid="mcpui-error" style={{ color: '#b42318' }}>{state.error}</div>;
  }
  return (
    <div>
      <AppFrame ref={frameRef} title={title} src={state.src} srcDoc={state.srcDoc} sandbox={state.sandbox} onLoad={() => setFrameLoaded(true)} />
      <div style={{ marginTop: 12 }} data-testid="mcpui-host-state">
        {state.pendingApproval && state.approvalId ? (
          <div data-testid="mcpui-approval-pending">
            <strong>{state.pendingApprovalTitle || 'Approval required'}:</strong>{' '}
            <span>waiting for review</span>{' '}
            <button
              type="button"
              onClick={() => dispatchMCPUIApprovalRequest({
                approvalId: state.approvalId,
                resourceUri: uri,
                title,
              })}
            >
              Review approval
            </button>
          </div>
        ) : null}
        {state.messages.length > 0 ? (
          <div>
            <strong>Host messages:</strong>
            <ul>
              {state.messages.map((message, index) => <li key={`${index}:${message}`}>{message}</li>)}
            </ul>
          </div>
        ) : null}
        {state.toolResult ? (
          <div>
            <strong>Latest tool result:</strong> <span>{state.toolResult}</span>
          </div>
        ) : null}
        {state.toolCallStatus ? (
          <div>
            <strong>Latest tool status:</strong> <span>{state.toolCallStatus}</span>
            {state.approvalId ? <> <strong>Approval:</strong> <span>{state.approvalId}</span></> : null}
          </div>
        ) : null}
        {state.openLink ? (
          <div>
            <strong>Open link request:</strong>{' '}
            <a href={state.openLink} target="_blank" rel="noreferrer">Open requested link</a>
          </div>
        ) : null}
      </div>
    </div>
  );
}
