import { describe, expect, it } from 'vitest'

import {
  normalizeWorkspaceAgentInfos,
  normalizeWorkspaceAgentOptions,
  normalizeWorkspaceModelInfos,
  normalizeWorkspaceModelOptions
} from './workspaceMetadata'

describe('workspaceMetadata', () => {
  it('keeps descriptive backend agent names unchanged and lightly cleans model labels', () => {
    expect(normalizeWorkspaceAgentInfos([
      { id: 'chat-helper', name: 'Chat Helper', modelRef: 'openai_gpt-5.2' }
    ])).toEqual([
      { id: 'chat-helper', name: 'Chat Helper', modelRef: 'openai_gpt-5.2', model: 'openai_gpt-5.2' }
    ])

    expect(normalizeWorkspaceModelInfos([
      { id: 'openai_o3', name: 'o3 (OpenAI)' },
      { id: 'openai_o4-mini', name: 'o4 - mini (OpenAI)' },
      { id: 'bedrock_claude_4-5', name: 'Claude 4.5 Sonnet' }
    ])).toEqual([
      { id: 'openai_o3', name: 'o3' },
      { id: 'openai_o4-mini', name: 'o4 - mini' },
      { id: 'bedrock_claude_4-5', name: 'Claude 4.5 Sonnet' }
    ])
  })

  it('humanizes raw agent ids and keeps raw model ids when backend labels are missing', () => {
    expect(normalizeWorkspaceAgentOptions(['chat_helper'], '')).toEqual([
      { value: 'chat_helper', label: 'Chat Helper', modelRef: '', default: false }
    ])

    expect(normalizeWorkspaceModelOptions(['openai_gpt-5.2'], 'openai_gpt-5.2')).toEqual([
      { value: 'openai_gpt-5.2', label: 'openai_gpt-5.2', default: true }
    ])
  })

  it('filters internal agents from UI-facing agent infos and options', () => {
    expect(normalizeWorkspaceAgentInfos([
      { id: 'public-agent', name: 'Public Agent' },
      { id: 'internal-agent', name: 'Internal Agent', internal: true }
    ])).toEqual([
      { id: 'public-agent', name: 'Public Agent', modelRef: '', model: '' }
    ])

    expect(normalizeWorkspaceAgentOptions([
      { id: 'public-agent', name: 'Public Agent' },
      { id: 'internal-agent', name: 'Internal Agent', internal: true }
    ], '')).toEqual([
      { value: 'public-agent', label: 'Public Agent', modelRef: '', default: false, id: 'public-agent', name: 'Public Agent' }
    ])
  })
})
