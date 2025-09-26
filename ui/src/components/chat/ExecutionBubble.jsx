// ExecutionBubble.jsx – chat bubble that embeds ExecutionDetails for messages
// that include execution traces.  Derived from the original Forge MessageCard.

import React from "react";
import { Icon } from "@blueprintjs/core";
import { format as formatDate } from "date-fns";

import ExecutionDetails from "./ExecutionDetails.jsx";
import { setStage } from '../../utils/stageBus.js';

// Inline styles for a subtle hourglass bobbing animation (horizontal/vertical)
const hourglassStyles = `
.hg-anim { display: inline-block; }
@keyframes hg-bob-x { 0% { transform: translateX(0); } 50% { transform: translateX(2px); } 100% { transform: translateX(0); } }
@keyframes hg-bob-y { 0% { transform: translateY(0); } 50% { transform: translateY(-1px); } 100% { transform: translateY(0); } }
.hg-anim.h { animation: hg-bob-x 1s infinite ease-in-out; }
.hg-anim.v { animation: hg-bob-y 1s infinite ease-in-out; }
`;

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
    const avatarColour = msg.role === "user" ? "var(--blue4)"
        : msg.role === "assistant" ? "var(--light-gray4)"
        : "var(--orange3)";

    const iconName = msg.role === "assistant" ? "chat" : msg.role === "tool" ? "wrench" : "person";

    const bubbleClass = (msg.role === "user"   ? "chat-bubble chat-user"
                      : msg.role === "assistant" ? "chat-bubble chat-bot"
                      :                            "chat-bubble chat-tool") + " has-executions";

    return (
        <div className={`chat-row ${msg.role}`}> {/* alignment flex row */}
            <div style={{ display: "flex", alignItems: "flex-start" }}>
                <div className="avatar" style={{ background: avatarColour }}>
                    <Icon icon={iconName} color="var(--black)" size={12} />
                </div>
                <div className={bubbleClass} data-ts={(function(){ try { const d = new Date(msg.createdAt); return isNaN(d) ? '' : formatDate(d, 'HH:mm'); } catch(_) { return ''; } })()}> 
                    <div className="prose max-w-full text-sm" dangerouslySetInnerHTML={{ __html: renderMarkdown(msg.content) }} />

                    {(msg.executions?.length > 0 || true) && (() => {
                        const steps = Array.isArray(msg.executions) && msg.executions[0] && Array.isArray(msg.executions[0].steps)
                            ? msg.executions[0].steps
                            : [];
                        const allowed = new Set(['thinking', 'tool_call', 'elicitation']);
                        const totalCount = steps.filter(s => allowed.has(String(s?.reason || '').toLowerCase())).length;
                        const countLabel = String(totalCount);
                        // Status + timer for the turn
                        const turnStatus = (msg.turnStatus || '').toLowerCase();
                        const isDone = turnStatus === 'succeeded' || turnStatus === 'completed' || turnStatus === 'done' || turnStatus === 'accepted' || turnStatus === 'failed' || turnStatus === 'error' || turnStatus === 'canceled';
                        const isError = turnStatus === 'failed' || turnStatus === 'error';
                        // Animated hourglass state
                        const [tick, setTick] = React.useState(0);
                        const [elapsed, setElapsed] = React.useState('');
                        React.useEffect(() => {
                            // If done and backend provides elapsedInSec, use it to freeze the final elapsed
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
                        // Render animated hourglass when running; ❗ on error; ✅ when done
                        const Hourglass = () => {
                            if (isError) return <span>❗</span>;
                            if (isDone)  return <span>✅</span>;
                            const glyph = (tick % 2 === 0) ? '⏳' : '⌛';
                            const orientClass = (Math.floor(tick / 2) % 2 === 0) ? 'h' : 'v';
                            return <span className={`hg-anim ${orientClass}`}>{glyph}</span>;
                        };

                        return (
                        <details className="mt-2">
                            <summary className="cursor-pointer text-xs text-blue-500">
                                <style>{hourglassStyles}</style>
                                <Hourglass />
                                {' '}Execution details ({countLabel}) {elapsed ? `• ${elapsed}` : ''}
                            </summary>
                            <ExecutionDetails executions={msg.executions} context={context} messageId={msg.id} resizable useCodeMirror />
                        </details>
                        );
                    })()}
                </div>
            </div>
        </div>
    );
}
import { getLogger, ForgeLog } from 'forge/utils/logger';
const log = getLogger('agently');
