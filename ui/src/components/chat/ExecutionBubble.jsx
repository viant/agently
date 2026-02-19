// ExecutionBubble.jsx – chat bubble that embeds ExecutionDetails for messages
// that include execution traces.  Derived from the original Forge MessageCard.

import React from "react";
import { Icon } from "@blueprintjs/core";
import { format as formatDate } from "date-fns";

import ExecutionDetails from "./ExecutionDetails.jsx";
import CollapsibleCard from "./CollapsibleCard.jsx";
import ToolFeed from "./ToolFeed.jsx";
import { setStage } from '../../utils/stageBus.js';
import CodeFenceRenderer from '../CodeFenceRenderer.jsx';
import { useExecVisibility } from '../../utils/execFeedBus.js';
import { selectedTabId } from 'forge/core';
// no endpoints import here; backend-only delete is not exposed in UI

// (removed hourglass animation; using a clock icon instead)

// ---------------------------------------------------------------------------
// Minimal markdown → HTML renderer (copied from Forge)
// ---------------------------------------------------------------------------
function renderMarkdown(md = "") {
    const trimmed = (md || '').trim();
    if (trimmed.startsWith('<table') || /<table\b/i.test(trimmed)) {
        return md; // already HTML table produced by normalizer
    }
    const escaped = md
        .replace(/&/g, "&amp;")
        .replace(/</g, "&lt;")
        .replace(/>/g, "&gt;");

    const withCodeBlocks = escaped.replace(/```([\s\S]*?)```/g, (match, p1) => `<pre><code>${p1}</code></pre>`);
    const withInlineCode = withCodeBlocks.replace(/`([^`]+?)`/g, "<code>$1</code>");
    const withBold   = withInlineCode.replace(/\*\*(.*?)\*\*/g, "<strong>$1</strong>");
    const withItalic = withBold.replace(/\*(.*?)\*/g, "<em>$1</em>");
    const withStrike = withItalic.replace(/~~(.*?)~~/g, "<del>$1</del>");
    const withLinks  = withStrike.replace(/\[([^\]]+)\]\(([^\)]+)\)/g, '<a href="$2" target="_blank" rel="noopener noreferrer">$1</a>');
    return withLinks.replace(/\n/g, "<br/>");
}

function ExecutionBubble({ message: msg, context }) {
    log.debug('[chat][render] ExecutionBubble', { id: msg?.id, role: msg?.role, ts: Date.now() });
    const { execution: showExecution, toolFeed: showToolFeed } = useExecVisibility();
    const bubbleRef = React.useRef(null);
    const avatarColour = msg.role === "user" ? "var(--blue4)"
        : msg.role === "assistant" ? "var(--light-gray4)"
        : "var(--orange3)";

    // Role-based icon with execution status awareness
    const turnStatus = (msg.turnStatus || '').toLowerCase();
    const isDone = turnStatus === 'succeeded' || turnStatus === 'completed' || turnStatus === 'done' || turnStatus === 'accepted';
    const isError = turnStatus === 'failed' || turnStatus === 'error' || turnStatus === 'canceled';
    const iconName = msg.role === "execution"
        ? (isError ? 'issue' : (isDone ? 'tick-circle' : 'time'))
        : (msg.role === "assistant" ? "chat" : (msg.role === "tool" ? "wrench" : "person"));

    const bubbleClass = (msg.role === "user"   ? "chat-bubble chat-user"
                      : msg.role === "assistant" ? "chat-bubble chat-bot"
                      :                            "chat-bubble chat-tool") + " has-executions";

    return (
        <div ref={bubbleRef} className={`chat-row ${msg.role}`}> {/* alignment flex row */}
            <div style={{ display: "flex", alignItems: "center" }}>
                <div className="avatar" style={{ background: avatarColour, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                    <Icon icon={iconName} color="var(--black)" size={12} />
                </div>
                <div className={bubbleClass} data-ts={(function(){ try { const d = new Date(msg.createdAt); return isNaN(d) ? '' : formatDate(d, 'HH:mm'); } catch(_) { return ''; } })()}> 
                    <CodeFenceRenderer text={msg.content || ''} />

                    {showExecution && (
                        <ExecutionTurnDetails msg={msg} context={context} bubbleRef={bubbleRef} />
                    )}

                    {/* Tool feed moved to its own card (ToolFeedBubble). */}
                </div>
            </div>
        </div>
    );
}
// Memoize heavy bubble; re-render only when relevant fields change
function areEqual(prev, next) {
    const a = prev.message || {};
    const b = next.message || {};
    // Stable identity by id + key fields that affect render
    if (a.id !== b.id) return false;
    if ((a.content || '') !== (b.content || '')) return false;
    if ((a.turnStatus || '') !== (b.turnStatus || '')) return false;
    if ((a._execSignature || '') !== (b._execSignature || '')) return false;
    if (!!a.isLastTurn !== !!b.isLastTurn) return false;
    if ((a.turnUpdatedAt || '') !== (b.turnUpdatedAt || '')) return false;
    return true;
}
export default React.memo(ExecutionBubble, areEqual);
function ExecutionTurnDetails({ msg, context, bubbleRef }) {
    const steps = Array.isArray(msg.executions) && msg.executions[0] && Array.isArray(msg.executions[0].steps)
        ? msg.executions[0].steps
        : [];
    const allowed = new Set(['thinking', 'tool_call', 'elicitation']);
    const totalCount = steps.filter(s => allowed.has(String(s?.reason || '').toLowerCase())).length;
    const countLabel = String(totalCount);
    const turnStatus = (msg.turnStatus || '').toLowerCase();
    const isDone = turnStatus === 'succeeded' || turnStatus === 'completed' || turnStatus === 'done' || turnStatus === 'accepted' || turnStatus === 'failed' || turnStatus === 'error' || turnStatus === 'canceled';
    const isError = turnStatus === 'failed' || turnStatus === 'error';

    const [tick, setTick] = React.useState(0);
    const [elapsed, setElapsed] = React.useState('');
    React.useEffect(() => {
        const providedSec = (typeof msg.turnElapsedSec === 'number' && isFinite(msg.turnElapsedSec) && msg.turnElapsedSec >= 0) ? Math.floor(msg.turnElapsedSec) : undefined;
        if (isDone && typeof providedSec === 'number') {
            const mm = String(Math.floor(providedSec / 60)).padStart(2, '0');
            const ss = String(providedSec % 60).padStart(2, '0');
            setElapsed(`${mm}:${ss}`);
            setTick(providedSec);
            return; // no interval
        }
        const start = msg.turnCreatedAt ? new Date(msg.turnCreatedAt) : (msg.createdAt ? new Date(msg.createdAt) : null);
        let endFixed = null;
        if (isDone) {
            endFixed = msg.turnUpdatedAt ? new Date(msg.turnUpdatedAt) : new Date();
        }
        function update() {
            try {
                const now = endFixed || new Date();
                if (!start || isNaN(start)) { setElapsed(''); return; }
                const diff = Math.max(0, now - start);
                const secs = Math.floor(diff / 1000);
                setTick(secs);
                const mm = String(Math.floor(secs / 60)).padStart(2, '0');
                const ss = String(secs % 60).padStart(2, '0');
                setElapsed(`${mm}:${ss}`);
            } catch(_) {}
        }
        update();
        if (isDone) return; // freeze when done
        const t = setInterval(update, 1000);
        return () => clearInterval(t);
    }, [msg.turnCreatedAt, msg.turnUpdatedAt, msg.createdAt, msg.turnElapsedSec, isDone]);

    // Update global stage based on turn status
    React.useEffect(() => {
        if (!turnStatus) return;
        // Only update stage for the active (selected) tab window.
        try {
            const activeWinId = selectedTabId?.value || '';
            const winId = context?.identity?.windowId || context?.handlers?.window?.windowId || context?.handlers?.window?.id || '';
            if (activeWinId && winId && String(activeWinId) !== String(winId)) {
                return;
            }
        } catch (_) {}
        // Only update stage from the currently visible (active) chat panel.
        try {
            const el = bubbleRef?.current;
            if (el) {
                const visible = !!(el.offsetParent || el.getClientRects().length);
                if (!visible) return;
            }
        } catch (_) {}
        // Only update when this bubble belongs to the selected conversation.
        try {
            const convCtx = context?.Context?.('conversations');
            const selectedId = convCtx?.handlers?.dataSource?.getSelection?.()?.selected?.id
                || convCtx?.handlers?.dataSource?.peekFormData?.()?.id
                || '';
            const msgConvId = msg?.conversationId || msg?.ConversationId || '';
            if (selectedId && msgConvId && String(selectedId) !== String(msgConvId)) {
                return;
            }
        } catch (_) {}
        // Update global stage
        const isRunning = (turnStatus === 'running' || turnStatus === 'open' || turnStatus === 'pending' || turnStatus === 'thinking');
        const isDoneOk = (turnStatus === 'succeeded' || turnStatus === 'completed' || turnStatus === 'done' || turnStatus === 'accepted');
        const isErrored = (turnStatus === 'failed' || turnStatus === 'error');
        
        // Determine whether finish ring is enabled for the current agent
        let ringEnabled = false;
        try {
            const metaForm = context?.Context?.('meta')?.handlers?.dataSource?.peekFormData?.() || {};
            const convForm = context?.Context?.('conversations')?.handlers?.dataSource?.peekFormData?.() || {};
            const agentKey = String(convForm?.agent || metaForm?.agent || '');
            const agentInfo = (metaForm?.agentInfo && agentKey) ? (metaForm.agentInfo[agentKey] || {}) : {};
            // Accept both top-level settings form value (from Settings toggle)
            // and agentInfo entry delivered by metadata aggregator.
            const topLevel = !!(metaForm.ringOnFinish || metaForm.finishRing || metaForm.notifyOnFinish);
            const fromAgent = !!(agentInfo.ringOnFinish || agentInfo.finishRing || agentInfo.notifyOnFinish);
            const ring = topLevel || fromAgent;
            // Allow user override via localStorage
            const localToggle = (localStorage.getItem('agently_finish_ring') || '').toLowerCase();
            const localEnabled = localToggle === '1' || localToggle === 'true' || localToggle === 'yes';
            ringEnabled = ring || localEnabled;
        } catch(_) {}

        const stagePayload = { turnId: (msg.turnId || msg.TurnId || msg.id || msg.Id), ringEnabled };
        if (isRunning) {
            setStage({phase: 'executing', ...stagePayload});
        } else if (isErrored) {
            setStage({phase: 'error', ...stagePayload});
        } else {
            setStage({phase: 'done', ...stagePayload});
        }

        // Also update conversations form running flag to drive data-driven abort visibility
        try {
            const convCtx = context?.Context?.('conversations');
            if (convCtx?.handlers?.dataSource?.setFormField) {
                const value = isRunning ? true : (isDoneOk || isErrored) ? false : undefined;
                if (value !== undefined) {
                    convCtx.handlers.dataSource.setFormField({ item: { id: 'running' }, value });
                    
                }
            }
        } catch(_) { /* ignore */ }
    }, [turnStatus]);

    // header uses a clock icon via CollapsibleCard

    return (
        <div style={{ marginTop: 8 }}>
            <div style={{ width: '80vw' }}>
            <CollapsibleCard
                title={`Execution details (${countLabel})${elapsed ? ` • ${elapsed}` : ''}`}
                icon={isError ? 'issue' : (isDone ? 'tick-circle' : 'time')}
                defaultOpen={!!msg.isLastTurn}
                width="100%"
                intent={isError ? 'danger' : (isDone ? 'success' : 'primary')}
                right={null}
            >
                <div style={{ width: '100%', overflowX: 'auto' }}>
                    <ExecutionDetails
                        executions={msg.executions}
                        context={context}
                        messageId={msg.id}
                        conversationId={msg.conversationId || msg.ConversationId}
                        turnStatus={msg.turnStatus}
                        turnError={msg.turnError}
                        resizable
                        useCodeMirror
                    />
                </div>
            </CollapsibleCard>
            </div>
        </div>
    );
}
import { getLogger, ForgeLog } from 'forge/utils/logger';
const log = getLogger('agently');
