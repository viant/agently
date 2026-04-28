import React from 'react';
import IterationBlock from './IterationBlock.jsx';

export default function IterationRowBlock({ context, iterationRow = null, suppressBubble = false }) {
  if (!iterationRow) return null;
  return <IterationBlock canonicalRow={iterationRow} context={context} showToolFeedDetail suppressBubble={suppressBubble} />;
}
