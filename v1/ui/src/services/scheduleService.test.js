import { afterEach, describe, expect, it, vi } from 'vitest'

import { client } from './agentlyClient'
import { panelHasRenderableRows, scheduleService } from './scheduleService'

function lookupContext(query = '') {
  return {
    handlers: {
      dataSource: {
        peekFilter() {
          return query ? { name: query } : {}
        }
      }
    }
  }
}

function saveContext(formValues = {}) {
  const state = {
    formValues,
    loading: false,
    fetches: 0,
    errors: []
  }
  const ds = {
    peekFormData() {
      return { values: state.formValues }
    },
    getFormData() {
      return state.formValues
    },
    peekSelection() {
      return { selected: {} }
    },
    getSelection() {
      return { selected: {} }
    },
    setFormData({ values }) {
      state.formValues = values
    },
    fetchCollection() {
      state.fetches += 1
    },
    setLoading(value) {
      state.loading = value
    },
    setError(err) {
      state.errors.push(err)
    }
  }
  return {
    state,
    context: {
      Context(name) {
        if (name !== 'schedules') return null
        return { handlers: { dataSource: ds } }
      }
    }
  }
}

afterEach(() => {
  vi.restoreAllMocks()
  scheduleService._saveScheduleInFlight = false
})

describe('scheduleService SDK lookups', () => {
  it('treats non-empty table rows as renderable even before cell text is populated', () => {
    const wrapper = {
      querySelector(selector) {
        if (selector !== 'tbody') return null
        return {
          querySelectorAll(innerSelector) {
            if (innerSelector !== 'tr') return []
            return [
              {
                classList: {
                  contains(className) {
                    return className === 'empty-row' ? false : false
                  }
                }
              }
            ]
          }
        }
      }
    }

    expect(panelHasRenderableRows(wrapper)).toBe(true)
  })

  it('normalizes and filters agent LOV rows from workspace metadata', () => {
    const rows = scheduleService.onFetchAgentsLov({
      context: lookupContext('cod'),
      collection: [
        { id: 'chat', name: 'chat' },
        { id: 'coder', name: 'coder', modelRef: 'openai_gpt-5.2' }
      ]
    })

    expect(rows).toEqual([
      { id: 'coder', name: 'Coder', modelRef: 'openai_gpt-5.2', model: 'openai_gpt-5.2' }
    ])
  })

  it('normalizes and filters model LOV rows from workspace metadata', () => {
    const rows = scheduleService.onFetchModelsLov({
      context: lookupContext('o3'),
      collection: [
        { id: 'openai_o3', name: 'o3 (OpenAI)' },
        { id: 'anthropic_claude-3.7-sonnet', name: 'Claude 3.7 Sonnet' }
      ]
    })

    expect(rows).toEqual([
      { id: 'openai_o3', name: 'o3 (OpenAI)' }
    ])
  })

  it('normalizes scheduler run rows without forcing active runs to look finished', () => {
    const rows = scheduleService.onFetchRuns({
      collection: [
        {
          id: 'run-1',
          scheduleId: 'sched-1',
          scheduleName: 'Nightly',
          conversationId: 'conv-1',
          status: 'running',
          createdAt: '2026-03-23T10:00:00Z',
          startedAt: '2026-03-23T10:00:05Z',
          updatedAt: '2026-03-23T10:01:00Z'
        }
      ]
    })

    expect(rows).toEqual([
      {
        id: 'run-1',
        scheduleId: 'sched-1',
        scheduleName: 'Nightly',
        conversationId: 'conv-1',
        status: 'running',
        createdAt: '2026-03-23T10:00:00Z',
        startedAt: '2026-03-23T10:00:05Z',
        completedAt: null,
        errorMessage: undefined
      }
    ])
  })

  it('pushes schedule form dependencies when focused schedule runs fetch arrives without a bound schedule id', () => {
    const pushFormDependencies = vi.fn()
    const context = {
      handlers: {
        dataSource: {
          peekInput() {
            return { parameters: {} }
          }
        }
      },
      Context(name) {
        if (name !== 'schedules') return null
        return {
          handlers: {
            dataSource: {
              peekFormData() {
                return { values: { id: 'sched-1' } }
              },
              getFormData() {
                return { id: 'sched-1' }
              },
              peekSelection() {
                return { selected: {} }
              },
              getSelection() {
                return { selected: {} }
              },
              pushFormDependencies
            }
          }
        }
      }
    }

    const rows = scheduleService.onFetchRuns({
      context,
      collection: []
    })

    expect(rows).toEqual([])
    expect(pushFormDependencies).toHaveBeenCalledTimes(1)
  })
})

describe('scheduleService saveSchedule', () => {
  it('assigns a client-side id for new schedules before saving', async () => {
    const { context, state } = saveContext({
      name: 'nightly',
      agentRef: 'chat',
      enabled: true,
      scheduleMode: 'daily',
      dailyTime: '09:00 AM',
      weekdays: ['mon'],
      timezone: 'UTC',
      taskPrompt: 'Run it'
    })
    const upsertSpy = vi.spyOn(client, 'upsertSchedules').mockResolvedValue(undefined)

    const ok = await scheduleService.saveSchedule({ context })

    expect(ok).toBe(true)
    expect(upsertSpy).toHaveBeenCalledTimes(1)
    expect(upsertSpy.mock.calls[0][0][0].id).toBeTruthy()
    expect(state.formValues.id).toBe(upsertSpy.mock.calls[0][0][0].id)
  })

  it('serializes calendar every-N-hours schedules to cron', async () => {
    const { context } = saveContext({
      name: 'weekend-poll',
      agentRef: 'chat',
      enabled: true,
      scheduleEditorKind: 'calendar',
      calendarPattern: 'every',
      calendarTime: '09:00 AM',
      calendarIntervalHours: 2,
      weekdays: ['mon', 'sat'],
      timezone: 'UTC',
      taskPrompt: 'Run it'
    })
    const upsertSpy = vi.spyOn(client, 'upsertSchedules').mockResolvedValue(undefined)

    const ok = await scheduleService.saveSchedule({ context })

    expect(ok).toBe(true)
    expect(upsertSpy).toHaveBeenCalledTimes(1)
    expect(upsertSpy.mock.calls[0][0][0].scheduleType).toBe('cron')
    expect(upsertSpy.mock.calls[0][0][0].cronExpr).toBe('0 9-23/2 * * 1,6')
    expect(upsertSpy.mock.calls[0][0][0].intervalSeconds).toBeNull()
  })

  it('serializes elapsed schedules to interval seconds', async () => {
    const { context } = saveContext({
      name: 'poller',
      agentRef: 'chat',
      enabled: true,
      scheduleEditorKind: 'elapsed',
      elapsedIntervalValue: 15,
      elapsedIntervalUnit: 'minutes',
      timezone: 'UTC',
      taskPrompt: 'Run it'
    })
    const upsertSpy = vi.spyOn(client, 'upsertSchedules').mockResolvedValue(undefined)

    const ok = await scheduleService.saveSchedule({ context })

    expect(ok).toBe(true)
    expect(upsertSpy).toHaveBeenCalledTimes(1)
    expect(upsertSpy.mock.calls[0][0][0].scheduleType).toBe('interval')
    expect(upsertSpy.mock.calls[0][0][0].intervalSeconds).toBe(900)
    expect(upsertSpy.mock.calls[0][0][0].cronExpr).toBe('*/15 * * * *')
  })

  it('serializes localized start and end dates to UTC ISO strings', async () => {
    const { context } = saveContext({
      name: 'nightly',
      agentRef: 'chat',
      enabled: true,
      scheduleEditorKind: 'calendar',
      calendarPattern: 'once',
      calendarTime: '09:00 AM',
      weekdays: ['mon'],
      timezone: 'UTC',
      taskPrompt: 'Run it',
      startAt: '3/1/2026',
      endAt: '3/31/2026'
    })
    const upsertSpy = vi.spyOn(client, 'upsertSchedules').mockResolvedValue(undefined)

    const ok = await scheduleService.saveSchedule({ context })

    expect(ok).toBe(true)
    expect(upsertSpy).toHaveBeenCalledTimes(1)
    expect(upsertSpy.mock.calls[0][0][0].startAt).toBe('2026-03-01T00:00:00.000Z')
    expect(upsertSpy.mock.calls[0][0][0].endAt).toBe('2026-03-31T00:00:00.000Z')
  })

  it('serializes picker Date values without shifting the selected UTC calendar date', async () => {
    const { context } = saveContext({
      name: 'nightly',
      agentRef: 'chat',
      enabled: true,
      scheduleEditorKind: 'calendar',
      calendarPattern: 'once',
      calendarTime: '09:00 AM',
      weekdays: ['mon'],
      timezone: 'UTC',
      taskPrompt: 'Run it',
      startAt: new Date(2026, 2, 1, 0, 0, 0),
      endAt: new Date(2026, 2, 31, 0, 0, 0)
    })
    const upsertSpy = vi.spyOn(client, 'upsertSchedules').mockResolvedValue(undefined)

    const ok = await scheduleService.saveSchedule({ context })

    expect(ok).toBe(true)
    expect(upsertSpy).toHaveBeenCalledTimes(1)
    expect(upsertSpy.mock.calls[0][0][0].startAt).toBe('2026-03-01T00:00:00.000Z')
    expect(upsertSpy.mock.calls[0][0][0].endAt).toBe('2026-03-31T00:00:00.000Z')
  })

  it('rejects invalid start dates before calling the scheduler API', async () => {
    const { context, state } = saveContext({
      name: 'nightly',
      agentRef: 'chat',
      enabled: true,
      scheduleEditorKind: 'calendar',
      calendarPattern: 'once',
      calendarTime: '09:00 AM',
      weekdays: ['mon'],
      timezone: 'UTC',
      taskPrompt: 'Run it',
      startAt: 'not-a-date'
    })
    const upsertSpy = vi.spyOn(client, 'upsertSchedules').mockResolvedValue(undefined)

    const ok = await scheduleService.saveSchedule({ context })

    expect(ok).toBe(false)
    expect(upsertSpy).not.toHaveBeenCalled()
    expect(state.errors).toHaveLength(1)
    expect(String(state.errors[0]?.message || state.errors[0])).toContain('Start Date must be a valid date/time')
  })

  it('defaults blank visibility to private when saving', async () => {
    const { context } = saveContext({
      name: 'nightly',
      agentRef: 'chat',
      enabled: true,
      scheduleMode: 'daily',
      dailyTime: '09:00 AM',
      weekdays: ['mon'],
      timezone: 'UTC',
      taskPrompt: 'Run it',
      visibility: ''
    })
    const upsertSpy = vi.spyOn(client, 'upsertSchedules').mockResolvedValue(undefined)

    const ok = await scheduleService.saveSchedule({ context })

    expect(ok).toBe(true)
    expect(upsertSpy).toHaveBeenCalledTimes(1)
    expect(upsertSpy.mock.calls[0][0][0].visibility).toBe('private')
  })

  it('sends edited description and taskPrompt for an existing schedule', async () => {
    const existing = {
      id: 'sched-1',
      name: 'nightly',
      description: 'before',
      agentRef: 'chat',
      enabled: true,
      scheduleMode: 'daily',
      dailyTime: '09:00 AM',
      weekdays: ['mon'],
      timezone: 'UTC',
      taskPrompt: 'hello'
    }
    const { context } = saveContext({
      ...existing,
      description: 'after',
      taskPrompt: 'goodbye'
    })
    context.Context = (name) => {
      if (name !== 'schedules') return null
      return {
        handlers: {
          dataSource: {
            peekFormData() {
              return { ...existing, description: 'after', taskPrompt: 'goodbye' }
            },
            getFormData() {
              return { ...existing, description: 'after', taskPrompt: 'goodbye' }
            },
            peekSelection() {
              return { selected: existing }
            },
            getSelection() {
              return { selected: existing }
            },
            setFormData() {},
            fetchCollection() {},
            setLoading() {},
            setError() {}
          }
        }
      }
    }
    const upsertSpy = vi.spyOn(client, 'upsertSchedules').mockResolvedValue(undefined)

    const ok = await scheduleService.saveSchedule({ context })

    expect(ok).toBe(true)
    expect(upsertSpy).toHaveBeenCalledTimes(1)
    expect(upsertSpy.mock.calls[0][0][0].description).toBe('after')
    expect(upsertSpy.mock.calls[0][0][0].taskPrompt).toBe('goodbye')
  })

  it('refreshes an existing schedule from getSchedule after save', async () => {
    const state = {
      formValues: {
        id: 'sched-1',
        name: 'nightly',
        description: 'after',
        agentRef: 'chat',
        enabled: true,
        scheduleMode: 'daily',
        dailyTime: '09:00 AM',
        weekdays: ['mon'],
        timezone: 'UTC',
        taskPrompt: 'goodbye'
      },
      collection: [{ id: 'sched-1', name: 'nightly', description: 'before', taskPrompt: 'hello' }],
      selected: { selected: { id: 'sched-1', name: 'nightly', description: 'before', taskPrompt: 'hello' }, rowIndex: 0 },
      setFormDataCalls: [],
      setCollectionCalls: [],
      setSelectedCalls: [],
      fetches: 0
    }
    const ds = {
      peekFormData() {
        return state.formValues
      },
      getFormData() {
        return state.formValues
      },
      peekSelection() {
        return state.selected
      },
      getSelection() {
        return state.selected
      },
      setFormData({ values }) {
        state.formValues = values
        state.setFormDataCalls.push(values)
      },
      peekCollection() {
        return state.collection
      },
      getCollection() {
        return state.collection
      },
      setCollection(records) {
        state.collection = records
        state.setCollectionCalls.push(records)
      },
      setSelected(next) {
        state.selected = next
        state.setSelectedCalls.push(next)
      },
      fetchCollection() {
        state.fetches += 1
      },
      setLoading() {},
      setError() {}
    }
    const context = {
      Context(name) {
        if (name !== 'schedules') return null
        return { handlers: { dataSource: ds } }
      }
    }
    vi.spyOn(client, 'upsertSchedules').mockResolvedValue(undefined)
    vi.spyOn(client, 'getSchedule').mockResolvedValue({
      id: 'sched-1',
      name: 'nightly',
      description: 'persisted',
      agentRef: 'chat',
      enabled: true,
      scheduleType: 'cron',
      cronExpr: '0 9 * * 1',
      timezone: 'UTC',
      taskPrompt: 'persisted prompt'
    })

    const ok = await scheduleService.saveSchedule({ context })

    expect(ok).toBe(true)
    expect(client.getSchedule).toHaveBeenCalledWith('sched-1')
    expect(state.fetches).toBe(0)
    expect(state.formValues.description).toBe('persisted')
    expect(state.formValues.taskPrompt).toBe('persisted prompt')
    expect(state.collection[0].description).toBe('persisted')
    expect(state.selected.selected.description).toBe('persisted')
  })

  it('keeps a newly created private schedule selected after save refresh', async () => {
    const state = {
      formValues: {
        name: 'nightly',
        agentRef: 'chat',
        enabled: true,
        scheduleMode: 'daily',
        dailyTime: '09:00 AM',
        weekdays: ['mon'],
        timezone: 'UTC',
        taskPrompt: 'goodbye',
        visibility: 'private'
      },
      collection: [],
      selected: { selected: null, rowIndex: -1 },
      setFormDataCalls: [],
      setCollectionCalls: [],
      setSelectedCalls: [],
      fetches: 0
    }
    const ds = {
      peekFormData() {
        return state.formValues
      },
      getFormData() {
        return state.formValues
      },
      peekSelection() {
        return state.selected
      },
      getSelection() {
        return state.selected
      },
      setFormData({ values }) {
        state.formValues = values
        state.setFormDataCalls.push(values)
      },
      peekCollection() {
        return state.collection
      },
      getCollection() {
        return state.collection
      },
      setCollection(records) {
        state.collection = records
        state.setCollectionCalls.push(records)
      },
      setSelected(next) {
        state.selected = next
        state.setSelectedCalls.push(next)
      },
      fetchCollection() {
        state.fetches += 1
      },
      setLoading() {},
      setError() {}
    }
    const context = {
      Context(name) {
        if (name !== 'schedules') return null
        return { handlers: { dataSource: ds } }
      }
    }
    vi.spyOn(client, 'upsertSchedules').mockResolvedValue(undefined)
    vi.spyOn(client, 'getSchedule').mockResolvedValue({
      id: 'sched-new',
      name: 'nightly',
      agentRef: 'chat',
      enabled: true,
      scheduleType: 'cron',
      cronExpr: '0 9 * * 1',
      timezone: 'UTC',
      taskPrompt: 'goodbye',
      visibility: 'private'
    })

    const ok = await scheduleService.saveSchedule({ context })

    expect(ok).toBe(true)
    expect(state.fetches).toBe(1)
    expect(state.formValues.visibility).toBe('private')
    expect(state.collection).toHaveLength(1)
    expect(state.collection[0].visibility).toBe('private')
    expect(state.selected.rowIndex).toBe(0)
    expect(state.selected.selected.visibility).toBe('private')
  })

  it('deduplicates concurrent save invocations', async () => {
    const { context } = saveContext({
      name: 'nightly',
      agentRef: 'chat',
      enabled: true,
      scheduleMode: 'daily',
      dailyTime: '09:00 AM',
      weekdays: ['mon'],
      timezone: 'UTC',
      taskPrompt: 'Run it'
    })
    let resolveSave
    const upsertSpy = vi.spyOn(client, 'upsertSchedules').mockImplementation(() => new Promise((resolve) => {
      resolveSave = resolve
    }))

    const first = scheduleService.saveSchedule({ context })
    const second = scheduleService.saveSchedule({ context })
    await Promise.resolve()

    expect(upsertSpy).toHaveBeenCalledTimes(1)

    resolveSave()
    await Promise.all([first, second])
  })
})

describe('scheduleService editor sync', () => {
  it('initializes new schedule drafts with private visibility', () => {
    const { context, state } = saveContext({})

    scheduleService.onInit({ context })

    expect(state.formValues.visibility).toBe('private')
  })

  it('pushes form dependencies on init when a focused schedule exists without selection', () => {
    const state = {
      formValues: { id: 'sched-1', name: 'nightly' },
      selected: { selected: {} }
    }
    const pushFormDependencies = vi.fn()
    const ds = {
      peekFormData() {
        return { values: state.formValues }
      },
      getFormData() {
        return state.formValues
      },
      peekSelection() {
        return state.selected
      },
      getSelection() {
        return state.selected
      },
      setFormData({ values }) {
        state.formValues = values
      },
      pushFormDependencies
    }
    const context = {
      Context(name) {
        if (name !== 'schedules') return null
        return { handlers: { dataSource: ds } }
      }
    }

    scheduleService.onInit({ context })

    expect(pushFormDependencies).toHaveBeenCalledTimes(1)
  })

  it('applies radio changes from the in-flight event payload', () => {
    const { context, state } = saveContext({
      scheduleEditorKind: 'calendar',
      calendarPattern: 'once',
      calendarTime: '09:00 AM',
      weekdays: ['mon'],
      timezone: 'UTC'
    })

    scheduleService.syncScheduleFields({
      context,
      item: { id: 'scheduleEditorKind' },
      value: 'elapsed'
    })

    expect(state.formValues.scheduleEditorKind).toBe('elapsed')
    expect(state.formValues.scheduleType).toBe('interval')
    expect(state.formValues.intervalSeconds).toBe(86400)
  })

  it('switches an existing interval schedule to calendar from a raw radio event payload', () => {
    const state = {
      formValues: {},
      selected: {
        selected: {
          id: 'sched-1',
          scheduleType: 'interval',
          intervalSeconds: 86400,
          scheduleEditorKind: 'elapsed',
          elapsedIntervalValue: 1,
          elapsedIntervalUnit: 'days',
          timezone: 'UTC'
        }
      }
    }
    const ds = {
      peekFormData() {
        return { values: state.formValues }
      },
      getFormData() {
        return state.formValues
      },
      peekSelection() {
        return state.selected
      },
      getSelection() {
        return state.selected
      },
      setFormData({ values }) {
        state.formValues = values
      }
    }
    const context = {
      Context(name) {
        if (name !== 'schedules') return null
        return { handlers: { dataSource: ds } }
      }
    }

    scheduleService.syncScheduleFields({
      context,
      item: { id: 'scheduleEditorKind' },
      value: undefined,
      event: 'calendar'
    })

    expect(state.formValues.scheduleEditorKind).toBe('calendar')
    expect(state.formValues.scheduleType).toBe('cron')
    expect(state.formValues.cronExpr).toBe('0 9 * * *')
    expect(state.formValues.intervalSeconds).toBeNull()
  })

  it('seeds advanced cron from the current elapsed builder state on first switch', () => {
    const { context, state } = saveContext({
      scheduleEditorKind: 'elapsed',
      elapsedIntervalValue: 5,
      elapsedIntervalUnit: 'hours',
      cronExpr: '0 */4 * * *',
      intervalSeconds: 14400,
      timezone: 'UTC'
    })

    scheduleService.syncScheduleFields({
      context,
      item: { id: 'scheduleEditorKind' },
      event: 'advanced'
    })

    expect(state.formValues.scheduleEditorKind).toBe('advanced')
    expect(state.formValues.cronExpr).toBe('0 */5 * * *')
    expect(state.formValues.scheduleSummary).toBe('At 00 minutes past every 5th hour from 00:00 through 20:00 on every day (UTC)')
  })

  it('refreshes advanced cron when an elapsed blur arrives after the mode switch', () => {
    const { context, state } = saveContext({
      scheduleEditorKind: 'advanced',
      elapsedIntervalValue: 4,
      elapsedIntervalUnit: 'hours',
      cronExpr: '0 */4 * * *',
      timezone: 'UTC'
    })

    scheduleService.syncScheduleFields({
      context,
      item: { id: 'elapsedIntervalValue' },
      value: 5
    })

    expect(state.formValues.scheduleEditorKind).toBe('advanced')
    expect(state.formValues.cronExpr).toBe('0 */5 * * *')
    expect(state.formValues.scheduleSummary).toBe('At 00 minutes past every 5th hour from 00:00 through 20:00 on every day (UTC)')
  })

  it('builds a precise summary for calendar every-N-hours schedules', () => {
    const { context, state } = saveContext({
      scheduleEditorKind: 'calendar',
      calendarPattern: 'every',
      calendarTime: '10:00 AM',
      calendarIntervalHours: 2,
      weekdays: ['tue', 'wed', 'thu', 'fri', 'sat', 'sun'],
      timezone: 'UTC'
    })

    scheduleService.syncScheduleFields({ context })

    expect(state.formValues.scheduleSummary).toBe('At 00 minutes past every 2nd hour from 10:00 through 22:00 on Tue-Sun (UTC)')
  })
})
