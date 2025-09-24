// ExecutionBubble.jsx – chat bubble that embeds ExecutionDetails for messages
// that include execution traces.  Derived from the original Forge MessageCard.

import React from "react";
import { Icon } from "@blueprintjs/core";
import { format as formatDate } from "date-fns";

import ExecutionDetails from "./ExecutionDetails.jsx";

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
                        return (
                        <details className="mt-2">
                            <summary className="cursor-pointer text-xs text-blue-500">
                                Execution details ({countLabel})
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
