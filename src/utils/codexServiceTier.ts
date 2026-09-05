export type ServiceTierKind = 'fast' | 'standard' | 'flex' | 'auto' | 'unknown' | 'other';

function normalizeTier(value?: string | null): string | null {
  return value?.trim().toLowerCase() || null;
}

export function resolveServiceTierDisplay(event: {
  serviceTier?: string | null;
  responseServiceTier?: string | null;
}) {
  const requested = normalizeTier(event.serviceTier);
  const reported = normalizeTier(event.responseServiceTier);
  const tier = reported ?? requested;
  let kind: ServiceTierKind;
  switch (tier) {
    case 'priority':
    case 'fast': kind = 'fast'; break;
    case 'default':
    case 'standard': kind = 'standard'; break;
    case 'flex': kind = 'flex'; break;
    case 'auto': kind = 'auto'; break;
    case null: kind = 'unknown'; break;
    default: kind = 'other';
  }
  return {
    kind,
    tier,
    requested,
    reported,
    fastNotHonored: (requested === 'priority' || requested === 'fast')
      && reported !== null && reported !== 'auto'
      && reported !== 'priority' && reported !== 'fast',
    // "auto" is a selection policy, not a confirmed processing tier.
    confirmed: reported !== null && reported !== 'auto',
  };
}
