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
  // The log badge describes the outgoing request. Keep the response value for
  // details instead of interpreting a differing value as a confirmed downgrade.
  const tier = requested ?? reported;
  const source = requested !== null ? 'request' : reported !== null ? 'response' : 'unknown';
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
    source,
  };
}
