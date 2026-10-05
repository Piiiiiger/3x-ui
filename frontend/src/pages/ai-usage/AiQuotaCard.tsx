import { useTranslation } from 'react-i18next';
import { Card, Tag } from 'antd';
import { ThunderboltOutlined } from '@ant-design/icons';

import { RainbowBar } from '@/components/ui';
import type { QuotaSlot } from './aiUsageFormat';
import { planEndDate, resetCountdown } from './aiUsageFormat';

const TOOL_NAME = { claude: 'Claude', codex: 'Codex' } as const;
const KNOWN_TIERS = new Set([
  'five_hour',
  'seven_day',
  'seven_day_opus',
  'seven_day_sonnet',
  'seven_day_fable',
  '30_day',
]);

interface AiQuotaCardProps {
  slot: QuotaSlot;
  nowMs: number;
  relativeTime: (unixSeconds?: number) => string;
}

/** One tool's plan windows as the newest synced reading saw them; empty rows keep the cards one size. */
export default function AiQuotaCard({ slot, nowMs, relativeTime }: AiQuotaCardProps) {
  const { t } = useTranslation();
  const { quota } = slot;
  const tierLabel = (name: string) =>
    KNOWN_TIERS.has(name) ? t(`pages.aiUsage.tier.${name}`) : name;

  // The footer always says where the reading stands: missing, failed, or whose and how fresh.
  let status: string;
  if (!quota) status = t('pages.aiUsage.limitsNone');
  else if (!quota.success)
    status = quota.error
      ? t('pages.aiUsage.limitsFailed', { error: quota.error })
      : t('pages.aiUsage.limitsSignedOut', { device: quota.deviceName });
  else
    status = t('pages.aiUsage.limitsRead', {
      device: quota.deviceName,
      time: relativeTime(Math.floor(quota.queriedAt / 1000)),
    });
  const endDate = quota ? planEndDate(quota.activeUntil) : null;

  return (
    <Card className="ai-quota-card" styles={{ body: { padding: 0 } }}>
      <div className="ov-wide-head">
        <div className="ov-wide-head-stack">
          <div className="ov-kicker ov-card-title ov-kicker-icon">
            <ThunderboltOutlined />
            {TOOL_NAME[slot.tool]}
            {quota?.planLabel && <Tag className="ai-plan-tag">{quota.planLabel}</Tag>}
          </div>
          <div className="ov-sub">
            {endDate
              ? t('pages.aiUsage.planActiveUntil', { date: endDate })
              : t('pages.aiUsage.limitsSub')}
          </div>
        </div>
      </div>
      <ul className="ai-quota-rows">
        {slot.tiers.map((tier, i) =>
          tier ? (
            <li key={tier.name}>
              <div className="ai-quota-row-head">
                <span>{tierLabel(tier.name)}</span>
                <span className="ai-quota-percent">{Math.round(tier.utilization)}%</span>
              </div>
              <RainbowBar
                percent={Math.min(100, Math.max(0, tier.utilization))}
                label={tierLabel(tier.name)}
                valueText={`${Math.round(tier.utilization)}%`}
              />
              <div className="ai-quota-reset">
                {resetCountdown(tier.resetsAt, nowMs)
                  ? t('pages.aiUsage.resetsIn', { time: resetCountdown(tier.resetsAt, nowMs) })
                  : ' '}
              </div>
            </li>
          ) : (
            <li key={`empty-${i}`} className="ai-quota-row-empty" aria-hidden="true">
              <div className="ai-quota-row-head">
                <span>—</span>
              </div>
              <div className="ai-quota-empty-bar" />
              <div className="ai-quota-reset">{' '}</div>
            </li>
          ),
        )}
      </ul>
      <div
        className={quota && !quota.success ? 'ai-quota-foot ai-quota-foot-warn' : 'ai-quota-foot'}
      >
        {status}
      </div>
    </Card>
  );
}
