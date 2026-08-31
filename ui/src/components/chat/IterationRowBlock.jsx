import React from 'react';
import IterationBlock from './IterationBlock.jsx';

export default function IterationRowBlock({ context, iterationRow = null, showToolFeedDetail = true, suppressBubble = false, retryPrompt = '', attachment = null }) {
  if (!iterationRow) return null;
  return <IterationBlock canonicalRow={iterationRow} context={context} showToolFeedDetail={showToolFeedDetail} suppressBubble={suppressBubble} retryPrompt={retryPrompt} attachment={attachment} />;
}
