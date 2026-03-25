import React, { useEffect, useRef } from 'react';
import RichContent from './RichContent';
import { AvatarIcon } from 'forge/components';

const AUTO_SCROLL_THRESHOLD_PX = 96;

export function getScrollContainer(node) {
  if (typeof window === 'undefined') return null;
  let current = node?.parentElement || null;
  while (current) {
    const style = window.getComputedStyle(current);
    const overflowY = String(style?.overflowY || '').toLowerCase();
    const canScroll = overflowY === 'auto' || overflowY === 'scroll' || overflowY === 'overlay';
    if (canScroll && current.scrollHeight > current.clientHeight + 1) {
      return current;
    }
    current = current.parentElement;
  }
  return document.scrollingElement || document.documentElement || document.body || null;
}

export function shouldAutoScrollStreamingRow(rowRect, containerRect, threshold = AUTO_SCROLL_THRESHOLD_PX) {
  const rowBottom = Number(rowRect?.bottom);
  const containerBottom = Number(containerRect?.bottom);
  if (!Number.isFinite(rowBottom) || !Number.isFinite(containerBottom)) return true;
  return rowBottom - containerBottom <= threshold;
}

export default function BubbleMessage({ message, messageIndex = 0 }) {
  const rowRef = useRef(null);
  const role = String(message?.role || 'assistant').toLowerCase();
  const isUser = role === 'user';
  const isPreamble = !!message?._preambleBubble;
  const terminalStatus = ['completed', 'succeeded', 'success', 'done'].includes(String(message?.turnStatus || message?.status || '').toLowerCase());
  const isStreaming = !isUser
    && !terminalStatus
    && Number(message?.interim ?? 1) !== 0
    && (message?.isStreaming === true || (message?._type === 'stream' && message?.isStreaming !== false));

  useEffect(() => {
    if (!isStreaming || typeof window === 'undefined') return undefined;
    const rowNode = rowRef.current;
    if (!rowNode) return undefined;
    const scrollContainer = getScrollContainer(rowNode);
    const rowRect = rowNode.getBoundingClientRect();
    const containerRect = scrollContainer && scrollContainer !== document.body && scrollContainer !== document.documentElement
      ? scrollContainer.getBoundingClientRect()
      : { bottom: window.innerHeight };
    if (!shouldAutoScrollStreamingRow(rowRect, containerRect)) return undefined;
    const rafId = window.requestAnimationFrame(() => {
      rowNode.scrollIntoView({ block: 'end', inline: 'nearest', behavior: 'auto' });
    });
    return () => window.cancelAnimationFrame(rafId);
  }, [isStreaming, message?.content, message?.id]);

  if (isPreamble) {
    return (
      <div className="app-preamble-bubble-row" data-testid={`chat-message-${message?.id || messageIndex}-row`} ref={rowRef}>
        <div className="app-preamble-bubble">
          <span className="app-preamble-bubble-indicator" />
          <div className="app-preamble-bubble-content">
            <RichContent content={String(message?.content || '').trim()} />
          </div>
        </div>
      </div>
    );
  }

  const bubbleClass = isUser ? 'app-bubble app-bubble-user' : 'app-bubble app-bubble-assistant';
  const rowClass = isUser ? 'app-bubble-row app-bubble-row-user' : 'app-bubble-row app-bubble-row-assistant';
  const avatarClass = isUser ? 'app-bubble-avatar app-bubble-avatar-user' : 'app-bubble-avatar app-bubble-avatar-assistant';
  const avatarIcon = isUser ? 'User' : 'SealCheck';
  const avatarColor = isUser ? '#2855b7' : '#58657a';
  const avatarWeight = isUser ? 'fill' : 'regular';

  return (
    <div className={rowClass} data-testid={`chat-message-${message?.id || messageIndex}-row`} ref={rowRef}>
      <div className={avatarClass} data-testid={`chat-message-${message?.id || messageIndex}-avatar`}>
        <AvatarIcon name={avatarIcon} size={14} weight={avatarWeight} color={avatarColor} />
      </div>
      <div className={bubbleClass}>
        <div className="app-bubble-content">
          <RichContent content={String(message?.content || '').trim()} />
          {isStreaming ? <span className="app-stream-caret" aria-label="streaming">▍</span> : null}
        </div>
      </div>
    </div>
  );
}
