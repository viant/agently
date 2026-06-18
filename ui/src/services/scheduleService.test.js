import { afterEach, describe, expect, it, vi } from 'vitest'
import * as forgeCore from 'forge/core'
import * as httpClient from './httpClient'
import * as dialogBus from '../utils/dialogBus'

vi.mock('./conversationWindow', () => ({
  openConversationInMainWindow: vi.fn()
}))

import { client } from './agentlyClient'
import { openConversationInMainWindow } from './conversationWindow'
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

function deleteContext({ selection, formValues, collection } = {}) {
  const state = {
    formValues: formValues || {},
    selected: selection ? { selected: selection, rowIndex: 0 } : { selected: {}, rowIndex: -1 },
    collection: Array.isArray(collection) ? collection.slice() : [],
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
      return state.selected
    },
    getSelection() {
      return state.selected
    },
    peekCollection() {
      return state.collection
    },
    getCollection() {
      return state.collection
    },
    setCollection(rows) {
      state.collection = rows
    },
    setSelected(next) {
      state.selected = next
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
  vi.useRealTimers()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  scheduleService._saveScheduleInFlight = false
  scheduleService._visibilityHookInstalled = false
  scheduleService._validationHookInstalled = false
  scheduleService._automationStateByWindow = undefined
  scheduleService._suppressNextScheduleSelection = false
  scheduleService._runSelectedInFlight = false
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
                querySelectorAll(cellSelector) {
                  if (cellSelector !== 'td') return []
                  return [
                    {
                      classList: {
                        contains() {
                          return false
                        }
                      }
                    }
                  ]
                }
              }
            ]
          }
        }
      }
    }

    expect(panelHasRenderableRows(wrapper)).toBe(true)
  })

  it('treats backfill empty-row cells as non-renderable', () => {
    const wrapper = {
      querySelector(selector) {
        if (selector !== 'tbody') return null
        return {
          querySelectorAll(innerSelector) {
            if (innerSelector !== 'tr') return []
            return [
              {
                querySelectorAll(cellSelector) {
                  if (cellSelector !== 'td') return []
                  return [
                    {
                      classList: {
                        contains(className) {
                          return className === 'empty-row'
                        }
                      }
                    }
                  ]
                }
              }
            ]
          }
        }
      }
    }

    expect(panelHasRenderableRows(wrapper)).toBe(false)
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

  it('opens run conversations through the main conversation navigation helper', () => {
    const ok = scheduleService.openRunConversation({
      row: {
        id: 'run-1',
        conversationId: 'conv-1'
      }
    })

    expect(ok).toBe(true)
    expect(openConversationInMainWindow).toHaveBeenCalledWith('conv-1')
  })

  it('clears stale focused run rows and refetches when selected schedule changes', () => {
    const runsState = {
      input: { parameters: { scheduleId: 'old-schedule' } },
      collection: [{ id: 'old-run', conversationId: 'old-conv' }],
      selection: { selected: { id: 'old-run' }, rowIndex: 0 },
      fetches: 0
    }
    const runsDS = {
      peekInput() {
        return runsState.input
      },
      setInputParameters(parameters) {
        runsState.input = { ...runsState.input, parameters }
      },
      setCollection(records) {
        runsState.collection = records
      },
      setSelected(selection) {
        runsState.selection = selection
      },
      fetchCollection() {
        runsState.fetches += 1
      }
    }
    const context = {
      identity: { windowId: 'schedule-window-test' },
      Context(name) {
        if (name === 'runs') return { handlers: { dataSource: runsDS }, identity: { windowId: 'schedule-window-test' } }
        return null
      }
    }

    scheduleService.onSelectSchedule({
      context,
      selected: { id: 'test31', name: 'test31', agentRef: 'chatter', taskPrompt: 'hi' }
    })

    expect(runsState.input.parameters.scheduleId).toBe('test31')
    expect(runsState.collection).toEqual([])
    expect(runsState.selection).toEqual({ selected: null, rowIndex: -1 })
    expect(runsState.fetches).toBe(1)
  })

  it('confirms Run Now before running, refreshes run history, and opens Run History after success', async () => {
    vi.useFakeTimers()
    const runsState = {
      input: { parameters: {} },
      collection: [{ id: 'old-run' }],
      selection: { selected: { id: 'old-run' }, rowIndex: 0 },
      fetches: 0
    }
    const schedulesDS = {
      peekSelection() {
        return { selected: { id: 'sched-1', name: 'Nightly', agentRef: 'chat' }, rowIndex: 0 }
      },
      getSelection() {
        return { selected: { id: 'sched-1', name: 'Nightly', agentRef: 'chat' }, rowIndex: 0 }
      }
    }
    const runsDS = {
      peekInput() {
        return runsState.input
      },
      setInputParameters(parameters) {
        runsState.input = { ...runsState.input, parameters }
      },
      setCollection(records) {
        runsState.collection = records
      },
      setSelected(selection) {
        runsState.selection = selection
      },
      fetchCollection() {
        runsState.fetches += 1
      }
    }
    const bus = {
      value: [],
      peek() {
        return this.value
      }
    }
    const context = {
      identity: { windowId: 'schedule-window-test' },
      Context(name) {
        if (name === 'schedules') return { handlers: { dataSource: schedulesDS, setLoading() {}, setError() {} } }
        if (name === 'runs') return { handlers: { dataSource: runsDS }, identity: { windowId: 'schedule-window-test' } }
        return null
      }
    }
    const fetchSpy = vi.fn().mockResolvedValue({
      ok: true,
      status: 204
    })
    vi.stubGlobal('fetch', fetchSpy)
    vi.spyOn(forgeCore, 'getBusSignal').mockReturnValue(bus)
    let confirmOptions
    const openSpy = vi.spyOn(dialogBus, 'openConfirmDialog').mockImplementation((options) => {
      confirmOptions = options
    })

    const ok = await scheduleService.runSelected({ context })

    expect(ok).toBe(true)
    expect(openSpy).toHaveBeenCalledWith(expect.objectContaining({
      title: 'Run Schedule Now',
      message: 'Start an immediate manual run for schedule "Nightly"?',
      confirmText: 'Run Now',
      cancelText: 'Cancel',
      loadingText: 'Starting...',
      intent: 'primary'
    }))
    expect(fetchSpy).not.toHaveBeenCalled()

    const confirmPromise = confirmOptions.onConfirm()
    await Promise.resolve()

    expect(fetchSpy).toHaveBeenCalledWith(
      '/v1/api/agently/scheduler/run-now/sched-1',
      expect.objectContaining({ method: 'POST', credentials: 'include' })
    )
    expect(runsState.fetches).toBe(0)
    expect(bus.value).toEqual([])

    await vi.advanceTimersByTimeAsync(999)

    expect(runsState.fetches).toBe(0)
    expect(bus.value).toEqual([])

    await vi.advanceTimersByTimeAsync(1)
    const confirmResult = await confirmPromise

    expect(confirmResult).toBe(true)
    expect(runsState.input.parameters.scheduleId).toBe('sched-1')
    expect(runsState.collection).toEqual([])
    expect(runsState.selection).toEqual({ selected: null, rowIndex: -1 })
    expect(runsState.fetches).toBe(1)
    expect(bus.value).toEqual([{ type: 'selectTab', tabId: 'scheduleRuns' }])
  })

  it('does not call Run Now when the confirmation is not accepted', async () => {
    const schedulesDS = {
      peekSelection() {
        return { selected: { id: 'sched-1', name: 'Nightly', agentRef: 'chat' }, rowIndex: 0 }
      },
      getSelection() {
        return { selected: { id: 'sched-1', name: 'Nightly', agentRef: 'chat' }, rowIndex: 0 }
      }
    }
    const bus = {
      value: [],
      peek() {
        return this.value
      }
    }
    const context = {
      identity: { windowId: 'schedule-window-test' },
      Context(name) {
        if (name === 'schedules') return { handlers: { dataSource: schedulesDS, setLoading() {}, setError() {} } }
        return null
      }
    }
    const fetchSpy = vi.fn()
    vi.stubGlobal('fetch', fetchSpy)
    vi.spyOn(forgeCore, 'getBusSignal').mockReturnValue(bus)
    const openSpy = vi.spyOn(dialogBus, 'openConfirmDialog').mockImplementation(() => {})

    const ok = await scheduleService.runSelected({ context })

    expect(ok).toBe(true)
    expect(openSpy).toHaveBeenCalledTimes(1)
    expect(fetchSpy).not.toHaveBeenCalled()
    expect(bus.value).toEqual([])
  })

  it('runs the schedule from form state when selection metadata is missing', async () => {
    vi.useFakeTimers()
    const schedulesDS = {
      peekSelection() {
        return { selected: {}, rowIndex: -1 }
      },
      getSelection() {
        return { selected: {}, rowIndex: -1 }
      },
      peekFormData() {
        return { values: { id: 'sched-form', name: 'From form', agentRef: 'chat' } }
      },
      getFormData() {
        return {}
      }
    }
    const context = {
      Context(name) {
        if (name === 'schedules') return { handlers: { dataSource: schedulesDS, setLoading() {}, setError() {} } }
        return null
      }
    }
    const fetchSpy = vi.fn().mockResolvedValue({
      ok: true,
      status: 204
    })
    vi.stubGlobal('fetch', fetchSpy)
    let confirmOptions
    vi.spyOn(dialogBus, 'openConfirmDialog').mockImplementation((options) => {
      confirmOptions = options
    })

    const ok = await scheduleService.runSelected({ context })
    const confirmPromise = confirmOptions.onConfirm()
    await Promise.resolve()
    await vi.advanceTimersByTimeAsync(1000)
    const confirmResult = await confirmPromise

    expect(ok).toBe(true)
    expect(confirmResult).toBe(true)
    expect(confirmOptions).toEqual(expect.objectContaining({
      message: 'Start an immediate manual run for schedule "From form"?'
    }))
    expect(fetchSpy).toHaveBeenCalledWith(
      '/v1/api/agently/scheduler/run-now/sched-form',
      expect.objectContaining({ method: 'POST', credentials: 'include' })
    )
  })

  it('shows the backend Run Now error message when rate limited', async () => {
    const errors = []
    const schedulesDS = {
      peekSelection() {
        return { selected: { id: 'sched-1', name: 'Nightly', agentRef: 'chat' }, rowIndex: 0 }
      },
      getSelection() {
        return { selected: { id: 'sched-1', name: 'Nightly', agentRef: 'chat' }, rowIndex: 0 }
      }
    }
    const bus = {
      value: [],
      peek() {
        return this.value
      }
    }
    const context = {
      identity: { windowId: 'schedule-window-test' },
      Context(name) {
        if (name !== 'schedules') return null
        return { handlers: { dataSource: schedulesDS, setLoading() {}, setError(err) { errors.push(err) } } }
      }
    }
    const message = "You can't run this schedule more than once per minute. Please wait before using Run Now again."
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 429,
      statusText: 'Too Many Requests',
      text: async () => JSON.stringify({ error: message })
    }))
    const toastSpy = vi.spyOn(httpClient, 'showToast').mockImplementation(() => {})
    vi.spyOn(forgeCore, 'getBusSignal').mockReturnValue(bus)
    let confirmOptions
    vi.spyOn(dialogBus, 'openConfirmDialog').mockImplementation((options) => {
      confirmOptions = options
    })

    const ok = await scheduleService.runSelected({ context })
    const confirmResult = await confirmOptions.onConfirm()

    expect(ok).toBe(true)
    expect(confirmResult).toBe(false)
    expect(toastSpy).toHaveBeenCalledWith(message, expect.objectContaining({ intent: 'warning', ttlMs: 6200 }))
    expect(errors).toHaveLength(1)
    expect(bus.value).toEqual([])
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

  it('keeps general field edits available when saving from another definition tab', async () => {
    const { context } = saveContext({
      scheduleEditorKind: 'calendar',
      calendarPattern: 'once',
      calendarTime: '11:25 AM',
      timezone: 'UTC'
    })
    const upsertSpy = vi.spyOn(client, 'upsertSchedules').mockResolvedValue(undefined)

    scheduleService.syncScheduleFields({ context, item: { id: 'name' }, value: 'codex-create-real' })
    scheduleService.syncScheduleFields({ context, item: { id: 'agentRef' }, value: 'chatter' })
    scheduleService.syncScheduleFields({ context, item: { id: 'taskPrompt' }, value: 'Run from saved general state' })
    scheduleService.syncScheduleFields({ context, item: { id: 'description' }, value: 'General tab was hidden before Save' })

    const ok = await scheduleService.saveSchedule({ context })

    expect(ok).toBe(true)
    expect(upsertSpy).toHaveBeenCalledTimes(1)
    expect(upsertSpy.mock.calls[0][0][0]).toMatchObject({
      name: 'codex-create-real',
      agentRef: 'chatter',
      taskPrompt: 'Run from saved general state',
      description: 'General tab was hidden before Save',
      scheduleType: 'cron',
      cronExpr: '25 11 * * *'
    })
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
    expect(state.errors).toHaveLength(0)
    expect(String(state.formValues.validationErrors?.startAt || '')).toContain('Start Date must be a valid date/time')
  })

  it('rejects missing schedule name before calling the scheduler API', async () => {
    const { context, state } = saveContext({
      name: '',
      agentRef: 'chat',
      enabled: true,
      scheduleEditorKind: 'calendar',
      calendarPattern: 'once',
      calendarTime: '09:00 AM',
      weekdays: ['mon'],
      timezone: 'UTC',
      taskPrompt: 'Run it'
    })
    const upsertSpy = vi.spyOn(client, 'upsertSchedules').mockResolvedValue(undefined)

    const ok = await scheduleService.saveSchedule({ context })

    expect(ok).toBe(false)
    expect(upsertSpy).not.toHaveBeenCalled()
    expect(state.errors).toHaveLength(0)
    expect(String(state.formValues.validationErrors?.name || '')).toContain('Schedule Name is required')
  })

  it('rejects missing agent before calling the scheduler API', async () => {
    const { context, state } = saveContext({
      name: 'nightly',
      agentRef: '',
      enabled: true,
      scheduleEditorKind: 'calendar',
      calendarPattern: 'once',
      calendarTime: '09:00 AM',
      weekdays: ['mon'],
      timezone: 'UTC',
      taskPrompt: 'Run it'
    })
    const upsertSpy = vi.spyOn(client, 'upsertSchedules').mockResolvedValue(undefined)

    const ok = await scheduleService.saveSchedule({ context })

    expect(ok).toBe(false)
    expect(upsertSpy).not.toHaveBeenCalled()
    expect(state.errors).toHaveLength(0)
    expect(String(state.formValues.validationErrors?.agentRef || '')).toContain('Agent is required')
  })

  it('rejects missing task prompt payload before calling the scheduler API', async () => {
    const { context, state } = saveContext({
      name: 'nightly',
      agentRef: 'chat',
      enabled: true,
      scheduleEditorKind: 'calendar',
      calendarPattern: 'once',
      calendarTime: '09:00 AM',
      weekdays: ['mon'],
      timezone: 'UTC',
      taskPrompt: '',
      taskPromptUri: ''
    })
    const upsertSpy = vi.spyOn(client, 'upsertSchedules').mockResolvedValue(undefined)

    const ok = await scheduleService.saveSchedule({ context })

    expect(ok).toBe(false)
    expect(upsertSpy).not.toHaveBeenCalled()
    expect(state.errors).toHaveLength(0)
    expect(String(state.formValues.validationErrors?.taskPrompt || '')).toContain('Task Prompt or Task Prompt URI is required')
  })

  it('allows task prompt uri to satisfy the required task payload', async () => {
    const { context } = saveContext({
      name: 'nightly',
      agentRef: 'chat',
      enabled: true,
      scheduleEditorKind: 'calendar',
      calendarPattern: 'once',
      calendarTime: '09:00 AM',
      weekdays: ['mon'],
      timezone: 'UTC',
      taskPrompt: '',
      taskPromptUri: 'gs://bucket/path/prompt.txt'
    })
    const upsertSpy = vi.spyOn(client, 'upsertSchedules').mockResolvedValue(undefined)

    const ok = await scheduleService.saveSchedule({ context })

    expect(ok).toBe(true)
    expect(upsertSpy).toHaveBeenCalledTimes(1)
    expect(upsertSpy.mock.calls[0][0][0].taskPrompt).toBe('')
    expect(upsertSpy.mock.calls[0][0][0].taskPromptUri).toBe('gs://bucket/path/prompt.txt')
  })

  it('sends blank strings for cleared optional URL fields so updates can remove them', async () => {
    const { context } = saveContext({
      id: 'sched-1',
      name: 'nightly',
      agentRef: 'chat',
      enabled: true,
      scheduleEditorKind: 'calendar',
      calendarPattern: 'once',
      calendarTime: '09:00 AM',
      weekdays: ['mon'],
      timezone: 'UTC',
      taskPrompt: 'Run it',
      taskPromptUri: '',
      userCredUrl: ''
    })
    const upsertSpy = vi.spyOn(client, 'upsertSchedules').mockResolvedValue(undefined)
    vi.spyOn(client, 'getSchedule').mockRejectedValue(new Error('skip refresh'))

    const ok = await scheduleService.saveSchedule({ context })

    expect(ok).toBe(true)
    expect(upsertSpy).toHaveBeenCalledTimes(1)
    expect(upsertSpy.mock.calls[0][0][0].taskPromptUri).toBe('')
    expect(upsertSpy.mock.calls[0][0][0].userCredUrl).toBe('')
  })

  it('rejects blank advanced cron expressions before calling the scheduler API', async () => {
    const { context, state } = saveContext({
      name: 'nightly',
      agentRef: 'chat',
      enabled: true,
      scheduleEditorKind: 'advanced',
      cronExpr: '',
      timezone: 'UTC',
      taskPrompt: 'Run it'
    })
    const upsertSpy = vi.spyOn(client, 'upsertSchedules').mockResolvedValue(undefined)

    const ok = await scheduleService.saveSchedule({ context })

    expect(ok).toBe(false)
    expect(upsertSpy).not.toHaveBeenCalled()
    expect(state.errors).toHaveLength(0)
    expect(String(state.formValues.validationErrors?.cronExpr || '')).toContain('Cron Expression is required')
  })

  it('rejects blank calendar time before calling the scheduler API', async () => {
    const { context, state } = saveContext({
      name: 'nightly',
      agentRef: 'chat',
      enabled: true,
      scheduleEditorKind: 'calendar',
      calendarPattern: 'once',
      calendarTime: '',
      weekdays: ['mon'],
      timezone: 'UTC',
      taskPrompt: 'Run it'
    })
    const upsertSpy = vi.spyOn(client, 'upsertSchedules').mockResolvedValue(undefined)

    const ok = await scheduleService.saveSchedule({ context })

    expect(ok).toBe(false)
    expect(upsertSpy).not.toHaveBeenCalled()
    expect(state.errors).toHaveLength(0)
    expect(String(state.formValues.validationErrors?.calendarTime || '')).toContain('At / Starting At is required')
  })

  it('rejects blank elapsed interval before calling the scheduler API', async () => {
    const { context, state } = saveContext({
      name: 'nightly',
      agentRef: 'chat',
      enabled: true,
      scheduleEditorKind: 'elapsed',
      elapsedIntervalValue: '',
      elapsedIntervalUnit: 'minutes',
      timezone: 'UTC',
      taskPrompt: 'Run it'
    })
    const upsertSpy = vi.spyOn(client, 'upsertSchedules').mockResolvedValue(undefined)

    const ok = await scheduleService.saveSchedule({ context })

    expect(ok).toBe(false)
    expect(upsertSpy).not.toHaveBeenCalled()
    expect(state.errors).toHaveLength(0)
    expect(String(state.formValues.validationErrors?.elapsedIntervalValue || '')).toContain('Repeat Every is required')
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

  it('defaults blank timeout to 300 seconds when saving', async () => {
    const { context } = saveContext({
      name: 'nightly',
      agentRef: 'chat',
      enabled: true,
      scheduleMode: 'daily',
      dailyTime: '09:00 AM',
      weekdays: ['mon'],
      timezone: 'UTC',
      taskPrompt: 'Run it',
      timeoutSeconds: ''
    })
    const upsertSpy = vi.spyOn(client, 'upsertSchedules').mockResolvedValue(undefined)

    const ok = await scheduleService.saveSchedule({ context })

    expect(ok).toBe(true)
    expect(upsertSpy).toHaveBeenCalledTimes(1)
    expect(upsertSpy.mock.calls[0][0][0].timeoutSeconds).toBe(300)
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

  it('keeps a newly created private schedule selected without a follow-up fetch', async () => {
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
    expect(state.fetches).toBe(0)
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

  it('keeps the current editor subtab after a successful save', async () => {
    vi.useFakeTimers()
    const { context } = saveContext({
      name: 'nightly',
      agentRef: 'chat',
      enabled: true,
      scheduleEditorKind: 'calendar',
      calendarPattern: 'once',
      calendarTime: '09:00 AM',
      weekdays: ['mon'],
      timezone: 'UTC',
      taskPrompt: 'Run it'
    })
    context.identity = { windowId: 'win-1' }

    const bus = {
      value: [],
      peek() {
        return this.value
      }
    }
    vi.spyOn(forgeCore, 'getBusSignal').mockReturnValue(bus)
    vi.spyOn(client, 'upsertSchedules').mockResolvedValue(undefined)
    vi.spyOn(client, 'getSchedule').mockRejectedValue(new Error('skip refresh'))

    const pending = scheduleService.saveSchedule({ context })
    await Promise.resolve()
    await vi.runAllTimersAsync()
    const ok = await pending

    expect(ok).toBe(true)
    expect(bus.value).toEqual([])
  })

  it('clears only the edited field validation error', () => {
    const { context, state } = saveContext({
      name: '',
      agentRef: '',
      scheduleEditorKind: 'calendar',
      calendarPattern: 'once',
      calendarTime: '09:00 AM',
      weekdays: ['mon'],
      timezone: 'UTC',
      taskPrompt: 'Run it',
      validationErrors: {
        name: 'Schedule Name is required',
        agentRef: 'Agent is required'
      }
    })

    scheduleService.syncScheduleFields({
      context,
      item: { id: 'name' },
      value: 'nightly'
    })

    expect(state.formValues.validationErrors?.name).toBeUndefined()
    expect(state.formValues.validationErrors?.agentRef).toBe('Agent is required')
    expect(state.formValues.name).toBe('nightly')
  })

  it('shows field names in the validation toast without changing tabs', async () => {
    vi.useFakeTimers()
    const { context, state } = saveContext({
      name: '',
      agentRef: '',
      scheduleEditorKind: 'calendar',
      calendarPattern: 'once',
      calendarTime: '09:00 AM',
      weekdays: ['mon'],
      timezone: 'UTC',
      taskPrompt: 'Run it'
    })
    context.identity = { windowId: 'win-1' }

    const bus = {
      value: [],
      peek() {
        return this.value
      }
    }
    const toastSpy = vi.spyOn(httpClient, 'showToast').mockImplementation(() => {})
    vi.spyOn(forgeCore, 'getBusSignal').mockReturnValue(bus)
    const upsertSpy = vi.spyOn(client, 'upsertSchedules').mockResolvedValue(undefined)

    const pending = scheduleService.saveSchedule({ context })
    await Promise.resolve()
    await vi.runAllTimersAsync()
    const ok = await pending

    expect(ok).toBe(false)
    expect(upsertSpy).not.toHaveBeenCalled()
    expect(toastSpy).toHaveBeenCalledTimes(1)
    expect(String(toastSpy.mock.calls[0][0] || '')).toContain("Schedule can't be saved")
    expect(String(toastSpy.mock.calls[0][0] || '')).toContain('Schedule Name')
    expect(String(toastSpy.mock.calls[0][0] || '')).toContain('Agent')
    expect(bus.value).toEqual([])
    expect(scheduleService._validationHookInstalled).toBe(true)
    expect(state.formValues.validationErrors?.name).toBe('Schedule Name is required')
    expect(state.formValues.validationErrors?.agentRef).toBe('Agent is required')
  })
})

describe('scheduleService deleteSchedule', () => {
  it('calls DELETE and removes the row locally', async () => {
    const schedule = {
      id: 'sched-1',
      name: 'nightly',
      description: 'Nightly run',
      agentRef: 'chat',
      enabled: true,
      scheduleType: 'interval',
      intervalSeconds: 60,
      cronExpr: '',
      timezone: 'UTC',
      taskPrompt: 'Run it',
      timeoutSeconds: 300
    }
    const { context, state } = deleteContext({
      selection: { id: 'sched-1', name: 'nightly' },
      collection: [schedule, { id: 'sched-2', name: 'other' }]
    })
    const deleteSpy = vi.spyOn(client, 'deleteSchedule').mockResolvedValue(undefined)
    const openSpy = vi.spyOn(dialogBus, 'openConfirmDialog').mockImplementation(({ onConfirm }) => onConfirm?.())

    const ok = await scheduleService.deleteSchedule({ context })

    expect(ok).toBe(true)
    expect(openSpy).toHaveBeenCalledWith(expect.objectContaining({
      title: 'Delete Schedule',
      confirmText: 'Delete',
      loadingText: 'Deleting...'
    }))
    expect(deleteSpy).toHaveBeenCalledWith('sched-1')
    expect(state.collection).toEqual([{ id: 'sched-2', name: 'other' }])
    expect(state.selected).toEqual({ selected: {}, rowIndex: -1 })
    expect(state.fetches).toBe(1)
    expect(state.loading).toBe(false)
  })

  it('falls back to form state when selection metadata is missing', async () => {
    const { context } = deleteContext({
      formValues: {
        id: 'sched-1',
        name: 'nightly',
        agentRef: 'chat',
        enabled: true,
        scheduleType: 'adhoc',
        timezone: 'UTC'
      }
    })
    const deleteSpy = vi.spyOn(client, 'deleteSchedule').mockResolvedValue(undefined)
    vi.spyOn(dialogBus, 'openConfirmDialog').mockImplementation(({ onConfirm }) => onConfirm?.())

    const ok = await scheduleService.deleteSchedule({ context })

    expect(ok).toBe(true)
    expect(deleteSpy).toHaveBeenCalledWith('sched-1')
  })

  it('surfaces delete API errors', async () => {
    const { context, state } = deleteContext({
      selection: { id: 'sched-1', name: 'nightly' }
    })
    vi.spyOn(client, 'deleteSchedule').mockRejectedValue(new Error('forbidden'))
    vi.spyOn(dialogBus, 'openConfirmDialog').mockImplementation(({ onConfirm }) => onConfirm?.())

    const ok = await scheduleService.deleteSchedule({ context })

    expect(ok).toBe(true)
    expect(state.errors).toHaveLength(1)
    expect(String(state.errors[0]?.message || state.errors[0])).toContain('forbidden')
  })

  it('opens a confirmation dialog before deleting', async () => {
    const { context } = deleteContext({
      selection: { id: 'sched-1', name: 'nightly' }
    })
    const deleteSpy = vi.spyOn(client, 'deleteSchedule').mockResolvedValue(undefined)
    const openSpy = vi.spyOn(dialogBus, 'openConfirmDialog').mockImplementation(() => {})

    const ok = await scheduleService.deleteSchedule({ context })

    expect(ok).toBe(true)
    expect(openSpy).toHaveBeenCalledTimes(1)
    expect(deleteSpy).not.toHaveBeenCalled()
  })
})

describe('scheduleService editor sync', () => {
  it('initializes new schedule drafts with private visibility', () => {
    const { context, state } = saveContext({})

    scheduleService.onInit({ context })

    expect(state.formValues.visibility).toBe('private')
    expect(state.formValues.timeoutSeconds).toBe(300)
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
