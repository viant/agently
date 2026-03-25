import React, { useState, useEffect } from 'react';
import { getUsage, onUsageChange } from '../services/usageBus';

/**
 * Compact usage/cost bar rendered below the composer.
 * Shows: cost · total tokens (cached X) · model
 */
export default function UsageBar() {
  const [usage, setUsage] = useState(getUsage);

  useEffect(() => {
    return onUsageChange(() => setUsage(getUsage()));
  }, []);

  const { costText, tokensWithCacheText, model } = usage || {};
  if (!costText && !tokensWithCacheText) return null;

  return (
    <div className="app-usage-bar">
      {costText ? <span className="app-usage-bar-cost">{costText}</span> : null}
      {costText && tokensWithCacheText ? <span className="app-usage-bar-sep">·</span> : null}
      {tokensWithCacheText ? <span className="app-usage-bar-tokens">{tokensWithCacheText} tokens</span> : null}
      {model ? (
        <>
          <span className="app-usage-bar-sep">·</span>
          <span className="app-usage-bar-model">{model}</span>
        </>
      ) : null}
    </div>
  );
}
