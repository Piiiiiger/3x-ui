import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Empty, Spin } from 'antd';
import { useQuery } from '@tanstack/react-query';

import { keys } from '@/api/queryKeys';
import ProbeServerCard from '@/components/probe/ProbeServerCard';
import { PortalProbeSchema, type PortalProbe as PortalProbeData } from '@/generated/zod';
import { TimeFormatter } from '@/utils';

const POLL_INTERVAL_MS = 5000;

class PortalSessionEnded extends Error {}

async function fetchPortalProbe(base: string): Promise<PortalProbeData> {
  const res = await fetch(`${base}/probe`, {
    credentials: 'same-origin',
    headers: { Accept: 'application/json' },
  });
  if (!res.ok) {
    // Drain the body so the request completes instead of lingering open.
    await res.text().catch(() => '');
    if (res.status === 401) throw new PortalSessionEnded();
    throw new Error(`portal probe: HTTP ${res.status}`);
  }
  return PortalProbeSchema.parse(await res.json());
}

interface PortalProbeProps {
  base: string;
  email: string;
  onSessionEnded: () => void;
}

export default function PortalProbe({ base, email, onSessionEnded }: PortalProbeProps) {
  const { t } = useTranslation();
  const probe = useQuery({
    queryKey: keys.portal.probe(base, email),
    queryFn: () => fetchPortalProbe(base),
    refetchInterval: POLL_INTERVAL_MS,
    refetchIntervalInBackground: false,
    staleTime: 0,
    gcTime: 0,
    retry: false,
  });

  const sessionEnded = probe.error instanceof PortalSessionEnded;
  useEffect(() => {
    if (sessionEnded) onSessionEnded();
  }, [sessionEnded, onSessionEnded]);

  const data = probe.data;
  if (sessionEnded || (!data && !probe.isError)) {
    return (
      <div className="portal-status">
        <Spin />
      </div>
    );
  }

  // A failed poll keeps the figures of the last one that worked on screen.
  const dataFrom = data?.fetchedAt ?? 0;
  return (
    <section className="portal-probe">
      {(probe.isError || data?.stale) && (
        <Alert
          type="warning"
          showIcon
          title={
            dataFrom > 0
              ? t('pages.probe.staleData', { time: TimeFormatter.formatClock(dataFrom / 1000) })
              : t('subscription.portal.probeUnavailable')
          }
        />
      )}
      {data &&
        (data.servers.length === 0 ? (
          <Empty description={t('subscription.portal.probeEmpty')} />
        ) : (
          <div className="probe-grid">
            {data.servers.map((server) => (
              <ProbeServerCard key={server.id} server={server} subtitle={server.provider} />
            ))}
          </div>
        ))}
    </section>
  );
}
