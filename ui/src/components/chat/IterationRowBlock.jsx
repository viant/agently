import React from 'react';
import IterationBlock from './IterationBlock.jsx';

export default function IterationRowBlock({ message, context }) {
  if (!message) return null;
  return <IterationBlock message={message} context={context} showToolFeedDetail />;
}
