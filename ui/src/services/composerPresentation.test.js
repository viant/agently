import { describe, expect, it } from 'vitest'

import { composerPresentation } from './composerPresentation'

describe('composerPresentation', () => {
  it('uses backend labels for explicit agent and model selections', () => {
    expect(composerPresentation.getAgentButton({
      agentOptions: [{ value: 'chat-helper', label: 'chat-helper' }],
      agentValue: 'chat-helper',
      currentLabel: ''
    })).toEqual({ text: 'Chat Helper' })

    expect(composerPresentation.getModelButton({
      modelOptions: [{ value: 'openai_o3', label: 'o3 (OpenAI)' }],
      modelValue: 'openai_o3',
      currentLabel: 'o3 (OpenAI)'
    }).text).toBe('o3 (OpenAI)')
  })

  it('falls back to default option labels for auto selections', () => {
    expect(composerPresentation.getAgentButton({
      agentOptions: [
        { value: 'auto', label: 'Auto-select agent' },
        { value: 'coder', label: 'Coder', default: true }
      ],
      agentValue: 'auto',
      currentLabel: ''
    })).toEqual({ text: 'Auto-select agent' })

    expect(composerPresentation.getModelButton({
      modelOptions: [
        { value: 'auto', label: 'Auto-select model' },
        { value: 'openai_gpt-5.2', label: 'GPT-5.2', default: true }
      ],
      modelValue: 'auto',
      currentLabel: ''
    }).text).toBe('GPT-5.2')
  })
})
