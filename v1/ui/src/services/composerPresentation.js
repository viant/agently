import { Brain, Lightbulb } from '@phosphor-icons/react';

function titleCase(value) {
  const text = String(value || '').trim().toLowerCase();
  if (!text) return '';
  return text.charAt(0).toUpperCase() + text.slice(1);
}

function humanizeKey(value) {
  const raw = String(value || '').trim();
  if (!raw) return '';
  const spaced = raw
    .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
    .replace(/[._/-]+/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();
  if (!spaced) return raw;
  return spaced
    .split(' ')
    .filter(Boolean)
    .map((word) => {
      const lower = word.toLowerCase();
      if (lower.length <= 3 && lower === word) return lower.toUpperCase();
      return lower.charAt(0).toUpperCase() + lower.slice(1);
    })
    .join(' ');
}

function shortModelLabel(label, value) {
  const rawValue = String(value || '').trim();
  const rawLabel = String(label || rawValue || '').trim();
  if (!rawLabel) return 'Model';
  if (rawValue.toLowerCase() === 'auto' || rawLabel.toLowerCase() === 'auto-select model') return 'Auto';

  let shortened = rawLabel === rawValue ? rawValue : rawLabel;
  if (shortened.includes('/')) {
    shortened = shortened.split('/').pop() || shortened;
  }
  shortened = shortened.replace(/^[a-z0-9]+_/i, '');
  shortened = shortened.replace(/^\s*(openai|anthropic|google|meta|mistral|inceptionlabs|xai|vertexai|bedrock)[\s:/_-]+/i, '');
  shortened = shortened.replace(/\(([^)]*)\)/g, '');
  shortened = shortened.replace(/\bOpenAI\b|\bAnthropic\b|\bGoogle\b|\bxAI\b|\bVertex AI\b|\bAWS Bedrock\b/gi, '');
  shortened = shortened.replace(/_/g, '-');
  shortened = shortened.replace(/\s+/g, ' ').trim();
  if (!shortened) return rawLabel;
  const normalized = shortened
    .replace(/^gpt-?(\d+)-?(\d+)$/i, 'GPT-$1.$2')
    .replace(/^gpt-?(\d+)$/i, 'GPT-$1')
    .replace(/^gpt-?(\d+o(?:-mini)?)$/i, 'GPT-$1')
    .replace(/^o(\d+)(-mini)?$/i, (_, num, suffix = '') => `o${num}${suffix}`)
    .replace(/^claude[- ]?(.*)$/i, (_, rest = '') => `Claude ${String(rest).trim()}`.trim())
    .replace(/^gemini[- ]?(.*)$/i, (_, rest = '') => `Gemini ${String(rest).trim()}`.trim())
    .replace(/^grok[- ]?(.*)$/i, (_, rest = '') => `Grok ${String(rest).trim()}`.trim())
    .replace(/\bgpt\b/gi, 'GPT')
    .replace(/-mini\b/gi, ' Mini')
    .replace(/-codex$/i, ' Codex');
  return normalized || rawLabel;
}

function resolveAgentLabel(agentOptions, agentValue, currentLabel) {
  const current = String(agentValue || '').trim();
  const list = Array.isArray(agentOptions) ? agentOptions : [];
  if (!current || current.toLowerCase() === 'auto') {
    const defaultOption = list.find((opt) => !!opt?.default) || list.find((opt) => String(opt?.value ?? opt?.id ?? '').trim().toLowerCase() !== 'auto');
    const fallbackLabel = String(defaultOption?.label || defaultOption?.name || defaultOption?.title || '').trim();
    return fallbackLabel || 'Agent';
  }
  const match = list.find((opt) => String(opt?.value ?? opt?.id ?? '').trim() === current);
  const label = String(
    match?.label
    || match?.name
    || match?.title
    || currentLabel
    || ''
  ).trim();
  if (!label || label === current) return humanizeKey(current);
  return label;
}

function resolveReasoningLevel(reasoningValue, modelValue, modelInfo, modelOptions) {
  const normalized = String(reasoningValue || '').trim().toLowerCase();
  if (normalized === 'low' || normalized === 'medium' || normalized === 'high') return normalized;

  const modelID = String(modelValue || '').trim();
  const modelName = String(
    modelInfo?.[modelID]?.name
    || modelInfo?.[modelID]?.Name
    || (Array.isArray(modelOptions)
      ? (modelOptions.find((opt) => String(opt?.value ?? opt?.id ?? '').trim() === modelID)?.label || '')
      : '')
    || modelID
  ).toLowerCase();

  if (!modelName || modelID.toLowerCase() === 'auto') return '';
  if (/(mini|nano|flash|haiku)/.test(modelName)) return 'low';
  if (/(gpt-5|o1|o3|reasoning|sonnet|opus|pro\b)/.test(modelName)) return 'high';
  return 'medium';
}

function reasoningIconWeight(level) {
  switch (String(level || '').toLowerCase()) {
    case 'low':
      return 'regular';
    case 'high':
      return 'fill';
    default:
      return 'duotone';
  }
}

export const composerPresentation = {
  reasoningPlacement: 'after_model',
  getAgentButton({ agentOptions, agentValue, currentLabel }) {
    const raw = resolveAgentLabel(agentOptions, agentValue, currentLabel);
    if (!raw || raw === '—') return { text: 'Agent' };
    return { text: raw };
  },
  getModelButton({ modelOptions, modelValue, currentLabel }) {
    const current = String(modelValue || '').trim().toLowerCase();
    const defaults = Array.isArray(modelOptions) ? modelOptions : [];
    const defaultOption = defaults.find((opt) => !!opt?.default) || defaults.find((opt) => String(opt?.value ?? opt?.id ?? '').trim().toLowerCase() !== 'auto');
    const fallbackLabel = String(defaultOption?.label || defaultOption?.name || defaultOption?.title || '').trim();
    const text = (current && current !== 'auto')
      ? shortModelLabel(currentLabel, modelValue)
      : shortModelLabel(fallbackLabel, defaultOption?.value || '');
    return {
      text,
      icon: {
        component: Brain,
        props: { size: 20, weight: 'duotone', color: '#6a7ff2' }
      }
    };
  },
  getReasoningButton({ reasoningValue, modelValue, modelInfo, modelOptions }) {
    const level = resolveReasoningLevel(reasoningValue, modelValue, modelInfo, modelOptions);
    return {
      text: level ? titleCase(level) : 'Medium',
      icon: {
        component: Lightbulb,
        props: { size: 20, weight: reasoningIconWeight(level) }
      }
    };
  }
};
