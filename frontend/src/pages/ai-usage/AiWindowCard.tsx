import { useTranslation } from 'react-i18next';
import { Card } from 'antd';
import { ThunderboltOutlined } from '@ant-design/icons';

import { RainbowBar } from '@/components/ui';
import {
  formatTokens,
  formatUsd,
  formatWindowSpan,
  resetCountdown,
  type WindowSlot,
} from './aiUsageFormat';

const KNOWN_TIERS = new Set([
  'five_hour',
  'seven_day',
  'seven_day_opus',
  'seven_day_sonnet',
  'seven_day_fable',
  '30_day',
]);
const DAY = 86_400;

function Row({
  label,
  value,
  detail,
  strong = false,
}: {
  label: string;
  value: string;
  detail?: string;
  strong?: boolean;
}) {
  return (
    <>
      <dt>{label}</dt>
      <dd className={strong ? 'ai-window-value ai-window-strong' : 'ai-window-value'}>
        <span>{value}</span>
        {/* Every row keeps its second line, so neighbouring cards line up. */}
        <span className="ai-window-detail">{detail || ' '}</span>
      </dd>
    </>
  );
}

/**
 * One plan window of one tool: the plan's percentage, the computer's usage in the window,
 * the limit that implies and what is left. Rows and footer lines never change in number,
 * so the cards of a grid keep one size.
 */
export default function AiWindowCard({ slot, nowMs }: { slot: WindowSlot; nowMs: number }) {
  const { t, i18n } = useTranslation();
  const e = slot.estimate;
  const label = KNOWN_TIERS.has(slot.tier) ? t(`pages.aiUsage.tier.${slot.tier}`) : slot.tier;
  const reported = e?.reportedUtilization ?? slot.reading?.utilization ?? null;
  const estimatedNow = e?.estimatedUtilization ?? null;
  const shown = estimatedNow ?? reported ?? 0;
  const start = e?.start ?? null;
  const end = e?.end ?? null;
  const longWindow = start != null && end != null && end - start > DAY;
  const endIso = end != null ? new Date(end * 1000).toISOString() : (slot.reading?.resetsAt ?? '');
  const countdown = resetCountdown(endIso, nowMs);
  // Without estimates the reset time still comes from the plan's reading.
  const endUnix = end ?? (endIso ? Math.floor(Date.parse(endIso) / 1000) : null);
  const timeOf = (unix: number) =>
    new Date(unix * 1000).toLocaleString(i18n.language, {
      weekday: longWindow ? 'short' : undefined,
      hour: 'numeric',
      minute: '2-digit',
    });
  const tokens = (value: number) =>
    t('pages.aiUsage.window.tokens', { value: formatTokens(value) });
  const limit = e?.limit ?? null;

  let pace = ' ';
  let warn = false;
  if (!e) {
    pace = t('pages.aiUsage.window.needsUpdate');
  } else if (!limit) {
    pace =
      e.used.requests === 0 && (reported ?? 0) > 0
        ? t('pages.aiUsage.window.usedElsewhere')
        : start == null
          ? t('pages.aiUsage.window.noResetTime')
          : t('pages.aiUsage.window.noEstimate');
  } else if ((e.remainingCostUsd ?? 1) <= 0) {
    pace = t('pages.aiUsage.window.usedUp');
    warn = true;
  } else if (e.exhaustsAt != null) {
    pace = t('pages.aiUsage.window.runsOutAt', { time: timeOf(e.exhaustsAt) });
    warn = true;
  } else if (e.projectedUtilization != null) {
    pace = t('pages.aiUsage.window.projected', { value: Math.round(e.projectedUtilization) });
  }
  // A used-up week has nothing left to split over the 5-hour stretches.
  const budget =
    e?.perFiveHourCostUsd != null && e.fiveHourWindowsLeft != null && (e.remainingCostUsd ?? 0) > 0
      ? t('pages.aiUsage.window.perFiveHour', {
          count: e.fiveHourWindowsLeft,
          cost: formatUsd(e.perFiveHourCostUsd),
          tokens: formatTokens(e.perFiveHourTokens ?? 0),
        })
      : ' ';

  return (
    <Card className="ai-quota-card" styles={{ body: { padding: 0 } }}>
      <div className="ov-wide-head">
        <div className="ov-kicker ov-card-title ov-kicker-icon">
          <ThunderboltOutlined />
          {label}
        </div>
        <span className="ov-sub ai-window-span">
          {start != null && end != null ? formatWindowSpan(start, end, i18n.language) : ' '}
        </span>
      </div>
      <div className="ai-window-body">
        <div className="ai-window-big">
          <span className="ai-window-percent">{Math.round(reported ?? shown)}%</span>
          <span className="ai-window-caption">
            {reported != null
              ? t('pages.aiUsage.window.reported')
              : t('pages.aiUsage.window.estimated')}
          </span>
          <span className="ai-window-now">
            {reported != null && estimatedNow != null && Math.abs(estimatedNow - reported) >= 1
              ? t('pages.aiUsage.window.nowAbout', { value: Math.round(estimatedNow) })
              : ' '}
          </span>
        </div>
        <RainbowBar
          percent={Math.min(100, Math.max(0, shown))}
          label={label}
          valueText={`${Math.round(shown)}%`}
        />
        <dl className="ai-window-rows">
          <Row
            label={t('pages.aiUsage.window.used')}
            value={e ? `${formatUsd(e.used.costUsd)} · ${tokens(e.used.totalTokens)}` : '—'}
            detail={e ? t('pages.aiUsage.window.requests', { count: e.used.requests }) : ''}
          />
          <Row
            label={t('pages.aiUsage.window.limit')}
            value={limit ? `≈ ${formatUsd(limit.costUsd)} · ${tokens(limit.tokens)}` : '—'}
            detail={
              !limit
                ? ''
                : limit.basis === 'typical'
                  ? t('pages.aiUsage.window.basisTypical', {
                      count: limit.windows,
                      low: formatUsd(limit.costLow),
                      high: formatUsd(limit.costHigh),
                    })
                  : t('pages.aiUsage.window.basisCurrent', {
                      low: formatUsd(limit.costLow),
                      high: formatUsd(limit.costHigh),
                    })
            }
          />
          <Row
            label={t('pages.aiUsage.window.left')}
            value={
              e?.remainingCostUsd != null
                ? `≈ ${formatUsd(e.remainingCostUsd)} · ${tokens(e.remainingTokens ?? 0)}`
                : '—'
            }
            strong
          />
          <Row
            label={t('pages.aiUsage.window.resets')}
            value={
              countdown && endUnix != null
                ? t('pages.aiUsage.window.resetsIn', { countdown, time: timeOf(endUnix) })
                : '—'
            }
          />
        </dl>
      </div>
      <div className="ai-quota-foot ai-window-foot">
        <span className={warn ? 'ai-window-warn' : undefined} title={pace}>
          {pace}
        </span>
        <span className="ai-window-budget" title={budget}>
          {budget}
        </span>
      </div>
    </Card>
  );
}
