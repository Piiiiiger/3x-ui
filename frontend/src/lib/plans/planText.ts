import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';

import { SizeFormatter } from '@/utils';

export interface UserLimits {
  totalGB: number;
  trafficReset: string;
  trafficResetDay: number;
  limitIp: number;
}

// Human wording for a person's own limits, shown with their plan in the portal.
export function useLimitsText() {
  const { t } = useTranslation();
  return useCallback(
    (limits: UserLimits) => ({
      quota: limits.totalGB > 0 ? SizeFormatter.sizeFormat(limits.totalGB) : t('unlimited'),
      reset:
        limits.trafficReset === 'monthly'
          ? t('pages.plans.monthlyOn', { day: limits.trafficResetDay })
          : t(`pages.inbounds.periodicTrafficReset.${limits.trafficReset || 'never'}`),
      ipLimit: limits.limitIp > 0 ? String(limits.limitIp) : t('unlimited'),
    }),
    [t],
  );
}
