import { beforeEach, describe, expect, it } from 'vitest';
import {
  clearPendingElicitation,
  getPendingElicitation,
  getPendingElicitations,
  removePendingElicitation,
  replacePendingElicitationsForConversation,
  setPendingElicitation,
} from './elicitationBus';

const schema = { type: 'object', properties: { value: { type: 'string' } } };

describe('elicitationBus', () => {
  beforeEach(() => {
    clearPendingElicitation();
  });

  it('keeps concurrent pending elicitations instead of replacing the first one', () => {
    setPendingElicitation({
      conversationId: 'root',
      elicitationId: 'b3',
      message: 'B3 input',
      requestedSchema: schema,
    });
    setPendingElicitation({
      conversationId: 'root',
      elicitationId: 'a3',
      message: 'A3 input',
      requestedSchema: schema,
    });

    expect(getPendingElicitations()).toMatchObject([
      { conversationId: 'root', elicitationId: 'b3', message: 'B3 input' },
      { conversationId: 'root', elicitationId: 'a3', message: 'A3 input' },
    ]);
    expect(getPendingElicitation()).toMatchObject({ elicitationId: 'b3' });
  });

  it('removes only the resolved elicitation when another one is still pending', () => {
    setPendingElicitation({ conversationId: 'root', elicitationId: 'b3', requestedSchema: schema });
    setPendingElicitation({ conversationId: 'root', elicitationId: 'a3', requestedSchema: schema });

    removePendingElicitation({ conversationId: 'root', elicitationId: 'a3' });

    expect(getPendingElicitations()).toMatchObject([
      { conversationId: 'root', elicitationId: 'b3' },
    ]);
  });

  it('removes root proxy and child duplicate entries by shared elicitation id', () => {
    setPendingElicitation({ conversationId: 'root', elicitationId: 'same', requestedSchema: schema });
    setPendingElicitation({ conversationId: 'child', elicitationId: 'same', requestedSchema: schema });
    setPendingElicitation({ conversationId: 'root', elicitationId: 'other', requestedSchema: schema });

    removePendingElicitation({ conversationId: 'child', elicitationId: 'same' });

    expect(getPendingElicitations()).toMatchObject([
      { conversationId: 'root', elicitationId: 'other' },
    ]);
  });

  it('reconciles one conversation without deleting pending elicitations from another conversation', () => {
    setPendingElicitation({ conversationId: 'root', elicitationId: 'old-root', requestedSchema: schema });
    setPendingElicitation({ conversationId: 'child', elicitationId: 'child-pending', requestedSchema: schema });

    replacePendingElicitationsForConversation('root', [{
      conversationId: 'root',
      elicitationId: 'new-root',
      content: 'new root',
      elicitation: { requestedSchema: schema },
    }]);

    expect(getPendingElicitations()).toMatchObject([
      { conversationId: 'child', elicitationId: 'child-pending' },
      { conversationId: 'root', elicitationId: 'new-root', message: 'new root' },
    ]);
  });
});
