import React from 'react';
import IterationBlock from './IterationBlock.jsx';

export default function IterationRowBlock({ message, context, iterationRow = null }) {
  if (!message) return null;
  return <IterationBlock message={message} canonicalRow={iterationRow} context={context} showToolFeedDetail />;
}
