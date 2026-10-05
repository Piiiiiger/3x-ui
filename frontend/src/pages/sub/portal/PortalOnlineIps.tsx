import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';

import { keys } from '@/api/queryKeys';
import { OnlineIpsList } from '@/components/clients/ClientOnlineIps';
import { ClientOnlineIpsSchema, type ClientOnlineIps } from '@/generated/zod';
import { formatIpSlots } from '@/lib/clients/online-ips';
import { PortalSessionEnded } from './portalSession';

const POLL_INTERVAL_MS = 10_000;

async function fetchOnlineIps(base: string): Promise<{ data: ClientOnlineIps; at: number }> {
  const res = await fetch(`${base}/online-ips`, {
    credentials: 'same-origin',
    headers: { Accept: 'application/json' },
  });
  if (!res.ok) {
    await res.text().catch(() => '');
    if (res.status === 401) throw new PortalSessionEnded();
    throw new Error(`portal online ips: HTTP ${res.status}`);
  }
  return { data: ClientOnlineIpsSchema.parse(await res.json()), at: Date.now() };
}

interface PortalOnlineIpsProps {
  base: string;
  email: string;
  onSessionEnded: () => void;
}

// The signed-in person's IP slots in use, the IPs online and any IP banned for
// going over the limit, with the time left; there is nothing to act on here.
export default function PortalOnlineIps({ base, email, onSessionEnded }: PortalOnlineIpsProps) {
  const { t } = useTranslation();
  const query = useQuery({
    queryKey: keys.portal.onlineIps(base, email),
    queryFn: () => fetchOnlineIps(base),
    refetchInterval: POLL_INTERVAL_MS,
    refetchIntervalInBackground: false,
    staleTime: 0,
    gcTime: 0,
    retry: false,
  });

  const sessionEnded = query.error instanceof PortalSessionEnded;
  useEffect(() => {
    if (sessionEnded) onSessionEnded();
  }, [sessionEnded, onSessionEnded]);

  if (!query.data) return null;
  const { data, at } = query.data;
  return (
    <section className="portal-section">
      <div className="portal-section-head">
        <span className="portal-section-title">{t('pages.clients.onlineIps')}</span>
        <span className="portal-section-meta">{formatIpSlots(data.count, data.limit)}</span>
      </div>
      <OnlineIpsList
        data={data}
        nowMs={at}
        bannedHint={
          data.limit > 0 ? t('subscription.portal.ipBannedHint', { limit: data.limit }) : undefined
        }
      />
    </section>
  );
}
