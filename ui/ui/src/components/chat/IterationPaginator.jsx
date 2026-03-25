import React from 'react';
import { ButtonGroup, Button, Tooltip } from '@blueprintjs/core';

export default function IterationPaginator({ message, context }) {
  const hiddenCount = Number(message?.hiddenCount || 0);
  const totalCount = Number(message?.totalCount || 0);
  const visibleCount = Number(message?.visibleCount || 0);
  const chat = context?.services?.chat;
  if (!hiddenCount) {
    return null;
  }

  const loadOlder = async (all = false) => {
    if (!chat?.loadOlderExecutions) return;
    await chat.loadOlderExecutions({ context, all });
  };

  const resetWindow = async () => {
    if (!chat?.loadOlderExecutions) return;
    await chat.loadOlderExecutions({ context, reset: true });
  };

  return (
    <div className="app-iteration-paginator">
      <Tooltip content="Load earlier executions into the timeline">
        <ButtonGroup minimal>
          <Button icon="history" small onClick={() => void loadOlder(true)}>
            Load all
          </Button>
          <Button small onClick={() => void loadOlder(false)}>
            {hiddenCount} earlier executions hidden
          </Button>
          <Button icon="reset" small onClick={() => void resetWindow()}>
            Show latest
          </Button>
        </ButtonGroup>
      </Tooltip>
      {totalCount > 0 ? (
        <div className="app-iteration-paginator-meta">
          Showing {Math.min(visibleCount, totalCount)} of {totalCount}
        </div>
      ) : null}
    </div>
  );
}
