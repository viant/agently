import { describe, expect, it } from 'vitest';
import { scheduleHistoryFilter } from './ScheduleConversationHistory';

describe('scheduleHistoryFilter', () => {
  it('reads the selected schedule identity from window parameters', () => {
    expect(scheduleHistoryFilter({
      parameters: {
        scheduleId: 'sched-1',
        scheduleName: 'Nightly',
      },
    })).toEqual({
      scheduleId: 'sched-1',
      scheduleName: 'Nightly',
    });
  });

  it('does not invent a schedule id', () => {
    expect(scheduleHistoryFilter({})).toEqual({
      scheduleId: '',
      scheduleName: 'Automation',
    });
  });
});
