import assert from 'node:assert/strict';
import test from 'node:test';
import { resolveServiceTierDisplay } from './codexServiceTier.ts';

test('upstream priority is labeled as a response when the request tier is absent', () => {
  const result = resolveServiceTierDisplay({ responseServiceTier: 'priority' });
  assert.equal(result.kind, 'fast');
  assert.equal(result.source, 'response');
  assert.equal(result.requested, null);
});

test('Fast requests stay Fast while retaining differing and missing response values for details', () => {
  for (const reported of ['default', 'standard', 'flex', 'auto', 'priority', 'future-tier', '', null, undefined]) {
    const result = resolveServiceTierDisplay({ serviceTier: 'priority', responseServiceTier: reported });
    assert.equal(result.kind, 'fast');
    assert.equal(result.source, 'request');
    assert.equal(result.requested, 'priority');
    assert.equal(result.reported, reported || null);
  }
});

test('a Fast request without an upstream tier is displayed as requested', () => {
  const result = resolveServiceTierDisplay({ serviceTier: ' FAST ' });
  assert.equal(result.kind, 'fast');
  assert.equal(result.source, 'request');
  assert.equal(result.requested, 'fast');
  assert.equal(result.reported, null);
});

test('missing historical data is unknown, never assumed Standard or Fast', () => {
  assert.equal(resolveServiceTierDisplay({}).kind, 'unknown');
  assert.equal(resolveServiceTierDisplay({ serviceTier: ' ', responseServiceTier: ' ' }).source, 'unknown');
});

test('auto, Flex and future tier names retain their meaning', () => {
  assert.equal(resolveServiceTierDisplay({ serviceTier: 'auto' }).kind, 'auto');
  assert.equal(resolveServiceTierDisplay({ responseServiceTier: 'auto' }).source, 'response');
  assert.equal(resolveServiceTierDisplay({ responseServiceTier: 'flex' }).kind, 'flex');
  assert.equal(resolveServiceTierDisplay({ responseServiceTier: 'ultrafast' }).kind, 'other');
  for (const requested of ['auto', 'flex', 'standard', 'future-tier']) {
    const result = resolveServiceTierDisplay({ serviceTier: requested, responseServiceTier: 'priority' });
    assert.equal(result.tier, requested);
    assert.equal(result.source, 'request');
    assert.equal(result.reported, 'priority');
  }
});
