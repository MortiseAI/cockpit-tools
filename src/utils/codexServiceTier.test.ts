import assert from 'node:assert/strict';
import test from 'node:test';
import { resolveServiceTierDisplay } from './codexServiceTier.ts';

test('upstream priority confirms Fast even when the caller omitted the tier', () => {
  const result = resolveServiceTierDisplay({ responseServiceTier: 'priority' });
  assert.equal(result.kind, 'fast');
  assert.equal(result.confirmed, true);
  assert.equal(result.requested, null);
});

test('a standard upstream response takes precedence over a Fast request', () => {
  const result = resolveServiceTierDisplay({ serviceTier: 'priority', responseServiceTier: 'default' });
  assert.equal(result.kind, 'standard');
  assert.equal(result.confirmed, true);
  assert.equal(result.requested, 'priority');
  assert.equal(result.fastNotHonored, true);
});

test('a Fast request without an upstream tier remains unconfirmed', () => {
  const result = resolveServiceTierDisplay({ serviceTier: ' FAST ' });
  assert.equal(result.kind, 'fast');
  assert.equal(result.confirmed, false);
  assert.equal(result.fastNotHonored, false);
});

test('missing historical data is unknown, never assumed Standard or Fast', () => {
  assert.equal(resolveServiceTierDisplay({}).kind, 'unknown');
  assert.equal(resolveServiceTierDisplay({ responseServiceTier: ' ' }).confirmed, false);
});

test('auto, Flex and future tier names retain their meaning', () => {
  assert.equal(resolveServiceTierDisplay({ serviceTier: 'auto' }).kind, 'auto');
  assert.equal(resolveServiceTierDisplay({ responseServiceTier: 'auto' }).confirmed, false);
  assert.equal(resolveServiceTierDisplay({ responseServiceTier: 'flex' }).kind, 'flex');
  assert.equal(resolveServiceTierDisplay({ responseServiceTier: 'ultrafast' }).kind, 'other');
});
