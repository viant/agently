import { Brain, Lightbulb } from '@phosphor-icons/react';
import { displayAgentLabel, displayModelLabel } from './workspaceMetadata';

function titleCase(value) {
  const text = String(value || '').trim().toLowerCase();
  if (!text) return '';
  return text.charAt(0).toUpperCase() + text.slice(1);
}

function resolveAgentLabel(agentOptions, agentValue, currentLabel) {
  const current = String(agentValue || '').trim();
  const list = Array.isArray(agentOptions) ? agentOptions : [];
  if (current.toLowerCase() === 'auto') {
    return 'Auto-select agent';
  }
  if (!current) {
    const defaultOption = list.find((opt) => !!opt?.default) || list.find((opt) => String(opt?.value ?? opt?.id ?? '').trim().toLowerCase() !== 'auto');
    const fallbackLabel = displayAgentLabel(defaultOption);
    return fallbackLabel || 'Agent';
  }
  const match = list.find((opt) => String(opt?.value ?? opt?.id ?? '').trim() === current);
  const label = displayAgentLabel(match || { id: current, name: currentLabel || current });
  return label || current;
}

function resolveModelLabel(modelOptions, modelValue, currentLabel) {
  const current = String(modelValue || '').trim();
  const list = Array.isArray(modelOptions) ? modelOptions : [];
  if (!current || current.toLowerCase() === 'auto') {
    const defaultOption = list.find((opt) => !!opt?.default) || list.find((opt) => String(opt?.value ?? opt?.id ?? '').trim().toLowerCase() !== 'auto');
    return displayModelLabel(defaultOption) || 'Model';
  }
  const match = list.find((opt) => String(opt?.value ?? opt?.id ?? '').trim() === current);
  const label = displayModelLabel(match || { id: current, name: currentLabel || current });
  return label || current;
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
    const text = resolveModelLabel(modelOptions, modelValue, currentLabel);
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
