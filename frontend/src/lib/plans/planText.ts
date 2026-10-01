import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';

import { SizeFormatter } from '@/utils';

export interface PlanLimits {
  totalGB: number;
  durationDays: number;
  trafficReset: string;
  trafficResetDay: number;
  limitIp: number;
}

// Human wording for a plan's limits, shared by the plans page and the client portal.
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
