import { useEffect } from 'react';
import { useQuery } from '@tanstack/react-query';

import { keys } from '@/api/queryKeys';
import { BanBanner, BanHistory } from '@/components/abuse/BanHistory';
import { AbuseHistorySchema, type AbuseHistory } from '@/generated/zod';
import { PortalSessionEnded } from './portalSession';

const POLL_INTERVAL_MS = 30_000;

async function fetchBans(base: string): Promise<{ data: AbuseHistory; at: number }> {
  const res = await fetch(`${base}/bans`, {
    credentials: 'same-origin',
    headers: { Accept: 'application/json' },
  });
  if (!res.ok) {
    await res.text().catch(() => '');
    if (res.status === 401) throw new PortalSessionEnded();
    throw new Error(`portal bans: HTTP ${res.status}`);
  }
  return { data: AbuseHistorySchema.parse(await res.json()), at: Date.now() };
}

function usePortalBans(base: string, email: string) {
  return useQuery({
    queryKey: keys.portal.bans(base, email),
    queryFn: () => fetchBans(base),
    refetchInterval: POLL_INTERVAL_MS,
    refetchIntervalInBackground: false,
    staleTime: 0,
    gcTime: 0,
    retry: false,
  });
}

// PortalBanBanner puts a running ban or a lock above every view: someone who
// cannot connect comes here first to see why.
export function PortalBanBanner({ base, email }: { base: string; email: string }) {
  const query = usePortalBans(base, email);
  const ban = query.data?.data.status.ban;
  if (!query.data || !ban) return null;
  return <BanBanner ban={ban} nowMs={query.data.at} />;
}

interface PortalBansProps {
  base: string;
  email: string;
  onSessionEnded: () => void;
}

// The signed-in person's bans of the last 90 days, when and why; only an admin
// can end a ban or a lock.
export default function PortalBans({ base, email, onSessionEnded }: PortalBansProps) {
  const query = usePortalBans(base, email);

  const sessionEnded = query.error instanceof PortalSessionEnded;
  useEffect(() => {
    if (sessionEnded) onSessionEnded();
  }, [sessionEnded, onSessionEnded]);

  if (!query.data) return null;
  const { data, at } = query.data;
  return (
    <section className="portal-section">
      <div className="portal-section-head">
        <span className="portal-section-title">封禁记录</span>
        <span className="portal-section-meta">最近 90 天</span>
      </div>
      <BanHistory history={data} nowMs={at} showBanner={false} />
    </section>
  );
}
