import { afterEach, describe, expect, it, vi } from 'vitest'

import { client } from './agentlyClient'
import { scheduleService } from './scheduleService'

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
      { id: 'openai_o3', name: 'o3' }
    ])
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

  it('does not force private visibility when the form leaves it blank', async () => {
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
    expect(upsertSpy.mock.calls[0][0][0]).not.toHaveProperty('visibility')
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
