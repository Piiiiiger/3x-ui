import { useTranslation } from 'react-i18next';
import { Tag, Tooltip } from 'antd';

/** A host's traffic multiplier, said only when it is not 1. */
export function TrafficMultiplierTag({ multiplier }: { multiplier: number }) {
  const { t } = useTranslation();
  if (multiplier === 1) return null;
  const text = String(Number(multiplier.toFixed(4)));
  return (
    <Tooltip title={t('pages.nodes.trafficMultiplierTip', { multiplier: text })}>
      <Tag color="gold">{`${text}×`}</Tag>
    </Tooltip>
  );
}
