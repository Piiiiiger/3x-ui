import { useTranslation } from 'react-i18next';
import { Tag, Tooltip } from 'antd';

import { IntlUtil } from '@/utils';
import type { ClientState } from './clientState';

const STATE_COLORS: Record<ClientState, string | undefined> = {
  enabled: 'green',
  disabled: undefined,
  exhausted: 'red',
  expired: 'red',
};

export function ClientStateTag({ state }: { state: ClientState }) {
  const { t } = useTranslation();
  return (
    <Tag color={STATE_COLORS[state]} className="client-state">
      {t(`pages.clients.state.${state}`)}
    </Tag>
  );
}

export function OnlineDot({ online, lastOnline }: { online: boolean; lastOnline: number }) {
  const { t } = useTranslation();
  const label = online ? t('pages.clients.online') : t('pages.clients.offline');
  const seen = lastOnline > 0 ? IntlUtil.formatDate(lastOnline) : '-';
  return (
    <Tooltip title={online ? label : `${t('lastOnline')}: ${seen}`}>
      <span role="img" aria-label={label} className={online ? 'online-dot' : 'offline-dot'} />
    </Tooltip>
  );
}

/** 剩余天数: days to the expiry, red once past it; a first-use client has none yet. */
export function DaysLeftText({ days, delayed }: { days: number | null; delayed: boolean }) {
  const { t } = useTranslation();
  if (days === null) {
    return <span className="client-muted">{delayed ? t('pages.clients.delayedStart') : '—'}</span>;
  }
  if (days <= 0) {
    return <Tag color="red">{t('pages.clients.expiredDaysAgo', { count: -days })}</Tag>;
  }
  return (
    <Tag color={days <= 3 ? 'orange' : undefined}>
      {t('pages.clients.daysLeftValue', { count: days })}
    </Tag>
  );
}
