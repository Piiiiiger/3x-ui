import { useTranslation } from 'react-i18next';
import { Tag, Tooltip } from 'antd';

import { IntlUtil } from '@/utils';
import { formatIpSlots } from '@/lib/clients/online-ips';
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

// Slots of the IP limit in use now, e.g. "2/3"; red while a network is banned
// for going over, so the list shows who hit the limit without opening anything.
export function IpSlots({ count, limit, bans }: { count: number; limit: number; bans: number }) {
  const { t } = useTranslation();
  if (limit <= 0 && count === 0 && bans === 0) return null;
  const full = limit > 0 && count >= limit;
  return (
    <Tooltip
      title={
        bans > 0 ? t('pages.clients.ipBansCount', { count: bans }) : t('pages.clients.onlineIps')
      }
    >
      <Tag color={bans > 0 ? 'red' : full ? 'orange' : undefined} style={{ margin: 0 }}>
        {formatIpSlots(count, limit)}
      </Tag>
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
