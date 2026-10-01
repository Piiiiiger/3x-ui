import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';

import { useInboundOptions } from '@/api/queries/useInboundOptions';
import { useNodesQuery } from '@/api/queries/useNodesQuery';
import { formatInboundLabel } from '@/lib/inbounds/label';
import { SizeFormatter } from '@/utils';

interface PlanLimits {
  totalGB: number;
  durationDays: number;
  trafficReset: string;
  trafficResetDay: number;
  limitIp: number;
}

// Human wording for a plan's limits, shared by the plans page and the clients table.
export function usePlanText() {
  const { t } = useTranslation();
  return useCallback(
    (plan: PlanLimits) => ({
      quota: plan.totalGB > 0 ? SizeFormatter.sizeFormat(plan.totalGB) : t('unlimited'),
      duration:
        plan.durationDays > 0
          ? `${plan.durationDays} ${t('pages.plans.daysUnit')}`
          : t('pages.plans.permanent'),
      reset:
        plan.trafficReset === 'monthly'
          ? t('pages.plans.monthlyOn', { day: plan.trafficResetDay })
          : t(`pages.inbounds.periodicTrafficReset.${plan.trafficReset || 'never'}`),
      ipLimit: plan.limitIp > 0 ? String(plan.limitIp) : t('unlimited'),
    }),
    [t],
  );
}

export interface InboundChoice {
  label: string;
  value: number;
}

// Inbounds grouped by the panel that runs them: this one first, then each node.
export function useInboundChoices() {
  const { t } = useTranslation();
  const { data: inbounds } = useInboundOptions();
  const { nodes } = useNodesQuery();
  return useMemo(() => {
    const nodeName = new Map(nodes.map((n) => [n.id, n.name || n.remark || `#${n.id}`]));
    const groups = new Map<string, InboundChoice[]>();
    const byId = new Map<number, string>();
    for (const ib of inbounds ?? []) {
      const panel =
        ib.nodeId != null
          ? (nodeName.get(ib.nodeId) ?? `#${ib.nodeId}`)
          : t('pages.inbounds.localPanel');
      const label = formatInboundLabel(ib.tag, ib.remark);
      byId.set(ib.id, label);
      groups.set(panel, [...(groups.get(panel) ?? []), { label, value: ib.id }]);
    }
    const options = [...groups.entries()].map(([panel, items]) => ({
      label: panel,
      title: panel,
      options: items,
    }));
    const flat = options.flatMap((g) => g.options);
    return { options, flat, labelOf: (id: number) => byId.get(id) ?? `#${id}` };
  }, [inbounds, nodes, t]);
}
