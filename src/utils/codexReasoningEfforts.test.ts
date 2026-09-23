import assert from 'node:assert/strict';
import test from 'node:test';
import { codexReasoningEffortOptions } from './codexReasoningEfforts';

test('GPT-6 subscription models retain their distinct reasoning capabilities', () => {
  assert.deepEqual(codexReasoningEffortOptions('gpt-6-sol'), ['low', 'medium', 'high', 'xhigh', 'max', 'ultra']);
  assert.deepEqual(codexReasoningEffortOptions('gpt-6-luna'), ['low', 'medium', 'high', 'xhigh', 'max']);
});

test('GPT-6 API routes offer none without Codex-only ultra', () => {
  for (const id of ['openai/gpt-6-sol', 'cpa/gpt-6-luna', ' OPENAI/GPT-6-SOL ']) {
    assert.deepEqual(codexReasoningEffortOptions(id), ['none', 'low', 'medium', 'high', 'xhigh', 'max']);
  }
  assert.ok(codexReasoningEffortOptions('gpt-6-astra').includes('ultra'));
});
