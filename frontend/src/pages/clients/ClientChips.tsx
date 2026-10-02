import { useTranslation } from 'react-i18next';
import { Button } from 'antd';

import type { ClientsSummary } from '@/hooks/useClients';
import type { PlanSummary } from '@/generated/zod';

/** The status chips, each naming the server bucket it filters on. */
export const STATUS_CHIPS = [
  {
    bucket: 'expiring',
    labelKey: 'depletingSoon',
    count: (s: ClientsSummary) => s.expiringCount,
    dot: 'dot-orange',
  },
  {
    bucket: 'exhausted',
    labelKey: 'pages.clients.chips.exhausted',
    count: (s: ClientsSummary) => s.exhaustedCount,
    dot: 'dot-red',
  },
  {
    bucket: 'expired',
    labelKey: 'pages.clients.chips.expired',
    count: (s: ClientsSummary) => s.expiredCount,
    dot: 'dot-red',
  },
  {
    bucket: 'deactive',
    labelKey: 'pages.clients.chips.disabled',
    count: (s: ClientsSummary) => s.deactiveCount,
    dot: 'dot-gray',
  },
] as const;

interface ClientChipsProps {
  plans: PlanSummary[];
  summary: ClientsSummary;
  planFilter: number[];
  bucketFilter: string[];
  onShowAll: () => void;
  onPlan: (planId: number) => void;
  onBucket: (bucket: string) => void;
}

/** 妙妙屋X's plan chips with their head counts, then the status chips. */
export default function ClientChips({
  plans,
  summary,
  planFilter,
  bucketFilter,
  onShowAll,
  onPlan,
  onBucket,
}: ClientChipsProps) {
  const { t } = useTranslation();
  const onPlans = plans.reduce((sum, p) => sum + p.memberCount, 0);
  const withoutPlan = summary.total - onPlans;
  const chips = [
    ...plans.map((p) => ({ id: p.id, name: p.name, count: p.memberCount })),
    ...(plans.length > 0 && withoutPlan > 0
      ? [{ id: 0, name: t('pages.plans.noPlan'), count: withoutPlan }]
      : []),
  ];
  const onePlan = planFilter.length === 1 ? planFilter[0] : null;
  const oneBucket = bucketFilter.length === 1 ? bucketFilter[0] : null;
  return (
    <div className="client-chips">
      <Button
        type={planFilter.length === 0 && bucketFilter.length === 0 ? 'primary' : 'default'}
        aria-pressed={planFilter.length === 0 && bucketFilter.length === 0}
        onClick={onShowAll}
      >
        {t('pages.clients.chips.all')} ({summary.total})
      </Button>
      {chips.map((chip) => (
        <Button
          key={chip.id}
          type={onePlan === chip.id ? 'primary' : 'default'}
          aria-pressed={onePlan === chip.id}
          onClick={() => onPlan(chip.id)}
        >
          {chip.name} ({chip.count})
        </Button>
      ))}
      <span className="client-chips-divider" />
      {STATUS_CHIPS.map((chip) => (
        <Button
          key={chip.bucket}
          type={oneBucket === chip.bucket ? 'primary' : 'default'}
          aria-pressed={oneBucket === chip.bucket}
          onClick={() => onBucket(chip.bucket)}
        >
          <span className={`dot ${chip.dot}`} />
          {t(chip.labelKey)} ({chip.count(summary)})
        </Button>
      ))}
    </div>
  );
}
