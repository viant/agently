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

export default function ExecutionBubble({ message: msg, context }) {
    log.debug('[chat][render] ExecutionBubble', { id: msg?.id, role: msg?.role, ts: Date.now() });
    const { execution: showExecution, toolFeed: showToolFeed } = useExecVisibility();
    const avatarColour = msg.role === "user" ? "var(--blue4)"
        : msg.role === "assistant" ? "var(--light-gray4)"
        : "var(--orange3)";

    // Use clock for execution; keep original for others
    const iconName = msg.role === "execution"
        ? "time"
        : (msg.role === "assistant" ? "chat" : (msg.role === "tool" ? "wrench" : "person"));

    const bubbleClass = (msg.role === "user"   ? "chat-bubble chat-user"
                      : msg.role === "assistant" ? "chat-bubble chat-bot"
                      :                            "chat-bubble chat-tool") + " has-executions";

    return (
        <div className={`chat-row ${msg.role}`}> {/* alignment flex row */}
            <div style={{ display: "flex", alignItems: "center" }}>
                <div className="avatar" style={{ background: avatarColour, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                    <Icon icon={iconName} color="var(--black)" size={12} />
                </div>
                <div className={bubbleClass} data-ts={(function(){ try { const d = new Date(msg.createdAt); return isNaN(d) ? '' : formatDate(d, 'HH:mm'); } catch(_) { return ''; } })()}> 
                    <CodeFenceRenderer text={msg.content || ''} />

                    {showExecution && (
                        <ExecutionTurnDetails msg={msg} context={context} />
                    )}

                    {/* Tool feed moved to its own card (ToolFeedBubble). */}
                </div>
            </div>
        </div>
    );
}
function ExecutionTurnDetails({ msg, context }) {
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
        // Update global stage
        const isRunning = (turnStatus === 'running' || turnStatus === 'open' || turnStatus === 'pending' || turnStatus === 'thinking');
        const isDoneOk = (turnStatus === 'succeeded' || turnStatus === 'completed' || turnStatus === 'done' || turnStatus === 'accepted');
        const isErrored = (turnStatus === 'failed' || turnStatus === 'error');
        try { console.debug('[chat][turn]', {turnStatus, isRunning, isDoneOk, isErrored}); } catch(_) {}
        if (isRunning) {
            setStage({phase: 'executing'});
        } else if (isErrored) {
            setStage({phase: 'error'});
        } else {
            setStage({phase: 'done'});
        }
        // Nudge messages DS loading flag so Forge Chat shows Abort button while running
        try {
            const msgCtx = context?.Context?.('messages');
            const ctrlSig = msgCtx?.signals?.control;
            if (ctrlSig) {
                const prev = (typeof ctrlSig.peek === 'function') ? (ctrlSig.peek() || {}) : (ctrlSig.value || {});
                if (isRunning) {
                    try { console.debug('[chat][ds][control] set loading=true (running)', {prev}); } catch(_) {}
                    ctrlSig.value = {...prev, loading: true};
                } else if (isDoneOk || isErrored) {
                    try { console.debug('[chat][ds][control] set loading=false (finished)', {prev}); } catch(_) {}
                    ctrlSig.value = {...prev, loading: false};
                }
            }
        } catch(_) { /* ignore */ }

        // Also update conversations form running flag to drive data-driven abort visibility
        try {
            const convCtx = context?.Context?.('conversations');
            if (convCtx?.handlers?.dataSource?.setFormField) {
                const value = isRunning ? true : (isDoneOk || isErrored) ? false : undefined;
                if (value !== undefined) {
                    convCtx.handlers.dataSource.setFormField({ item: { id: 'running' }, value });
                    try { console.debug('[chat][conv] set running', { value }); } catch(_) {}
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
                icon="time"
                defaultOpen={!!msg.isLastTurn}
                width="100%"
                intent={isError ? 'danger' : (isDone ? 'success' : 'primary')}
                right={null}
            >
                <div style={{ width: '100%', overflowX: 'auto' }}>
                    <ExecutionDetails executions={msg.executions} context={context} messageId={msg.id} resizable useCodeMirror />
                </div>
            </CollapsibleCard>
            </div>
        </div>
    );
}
import { getLogger, ForgeLog } from 'forge/utils/logger';
const log = getLogger('agently');
