import type { CodexReasoningEffort } from '../types/codex';

const CODEX_EFFORTS: CodexReasoningEffort[] = ['low', 'medium', 'high', 'xhigh', 'max', 'ultra'];

/** Namespaced GPT-6 models are API routes; bare IDs use the Codex catalog. */
export function codexReasoningEffortOptions(modelId: string): CodexReasoningEffort[] {
  const parts = modelId.trim().toLowerCase().split('/');
  const model = parts[parts.length - 1];
  if (model === 'gpt-6-sol' || model === 'gpt-6-luna') {
    if (parts.length > 1) return ['none', 'low', 'medium', 'high', 'xhigh', 'max'];
    if (model === 'gpt-6-luna') return CODEX_EFFORTS.filter((effort) => effort !== 'ultra');
  }
  return [...CODEX_EFFORTS];
}
