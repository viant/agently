import { useMemo, useState } from 'react';

export function useSteerQueue({ isRunning = false, service = null, conversationId = '' } = {}) {
  const [mode, setMode] = useState(isRunning ? 'steer' : 'queue');
  const [queue, setQueue] = useState([]);

  const api = useMemo(() => ({
    async submit(content, runningTurnId) {
      if (!content) return;
      if (mode === 'steer' && runningTurnId && service?.steerTurn) {
        await service.steerTurn({ conversationID: conversationId, turnID: runningTurnId, content });
        return;
      }
      setQueue((current) => [...current, { id: `local:${Date.now()}`, preview: content }]);
    },
    async forceSteerItem(turnId) {
      if (!service?.forceSteerQueuedTurn) return;
      await service.forceSteerQueuedTurn({ conversationID: conversationId, turnID: turnId });
    },
    async editItem(turnId, content) {
      if (!service?.editQueuedTurn) return;
      await service.editQueuedTurn({ conversationID: conversationId, turnID: turnId, content });
    },
    async deleteItem(turnId) {
      if (!service?.cancelQueuedTurnByID) return;
      await service.cancelQueuedTurnByID({ conversationID: conversationId, turnID: turnId });
    },
    setMode,
    mode,
    queue,
    setQueue
  }), [mode, queue, service, conversationId]);

  return api;
}
