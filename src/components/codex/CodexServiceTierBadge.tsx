import { Zap } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { resolveServiceTierDisplay } from '../../utils/codexServiceTier';

export function CodexServiceTierBadge(props: {
  serviceTier?: string | null;
  responseServiceTier?: string | null;
}) {
  const { t } = useTranslation();
  const display = resolveServiceTierDisplay(props);
  const isFast = display.kind === 'fast' || display.tier === 'ultrafast';
  const labels = {
    fast: 'Fast',
    standard: t('codex.apiService.logs.speedStandard', 'Standard'),
    flex: 'Flex',
    auto: t('codex.apiService.logs.speedAuto', 'Auto'),
    unknown: t('codex.apiService.logs.speedUnknown', 'Speed unknown'),
    other: display.tier === 'ultrafast' ? t('codex.speed.ultrafast', '超高速') : display.tier,
  };
  const label = labels[display.kind];
  const text = display.kind === 'unknown'
    ? label
    : display.source === 'request'
      ? t('codex.apiService.logs.speedRequested', { mode: label, defaultValue: '{{mode}} requested' })
      : t('codex.apiService.logs.speedReported', { mode: label, defaultValue: '{{mode}} · reported' });
  const missing = t('codex.apiService.logs.speedNotRecorded', 'Not recorded');
  const title = t('codex.apiService.logs.speedDetails', {
    requested: display.requested ?? missing,
    reported: display.reported ?? missing,
    defaultValue: 'Requested: {{requested}}; upstream reported: {{reported}}',
  });

  return (
    <span
      className={`codex-api-service-pill ${isFast ? 'speed-fast' : 'muted'}`}
      title={title}
      aria-label={`${text}. ${title}`}
    >
      {isFast && <Zap size={12} aria-hidden="true" />}
      {text}
    </span>
  );
}
