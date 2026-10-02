import { useTranslation } from 'react-i18next';
import type { TFunction } from 'i18next';
import { Typography } from 'antd';

import ProbeServerCard, { ProbeMeter } from '@/components/probe/ProbeServerCard';
import type { ProbeServer } from '@/generated/zod';
import { SizeFormatter } from '@/utils';

// A server that never reported has none of these, and "none" is what a
// bare-metal one reports as its virtualization.
function systemLine(server: ProbeServer, t: TFunction): string {
  const virtualization = server.virtualization === 'none' ? '' : server.virtualization;
  const cores = server.cpuCores > 0 ? t('pages.probe.cores', { count: server.cpuCores }) : '';
  return [server.os, server.arch, virtualization, cores].filter(Boolean).join(' · ');
}

// The usage is known only while the server is online; without a limit the bar stays
// empty, so that every card ends alike.
function QuotaFooter({ server }: { server: ProbeServer }) {
  const { t } = useTranslation();
  if (server.status !== 'online') return null;
  const limited = server.trafficLimit > 0;
  const limit = limited ? SizeFormatter.sizeFormat(server.trafficLimit) : t('unlimited');
  return (
    <ProbeMeter
      label={t('pages.probe.quota')}
      percent={limited ? (server.trafficUsed / server.trafficLimit) * 100 : null}
      detail={`${SizeFormatter.sizeFormat(server.trafficUsed)} / ${limit}`}
    />
  );
}

/** 仅监控: servers the Lite monitor watches that run no node of this panel. */
export default function MonitorOnlySection({ servers }: { servers: ProbeServer[] }) {
  const { t } = useTranslation();
  if (servers.length === 0) return null;
  return (
    <section className="hosts-section" aria-label={t('pages.nodes.monitorOnly')}>
      <div className="hosts-section-head">
        <Typography.Title level={4}>{t('pages.nodes.monitorOnly')}</Typography.Title>
        <Typography.Text type="secondary">{t('pages.nodes.monitorOnlyHint')}</Typography.Text>
      </div>
      <div className="host-grid">
        {servers.map((server) => (
          <ProbeServerCard
            key={server.id}
            server={server}
            subtitle={systemLine(server, t)}
            footer={<QuotaFooter server={server} />}
          />
        ))}
      </div>
    </section>
  );
}
