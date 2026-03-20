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
