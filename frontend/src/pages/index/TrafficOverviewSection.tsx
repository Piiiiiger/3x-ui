import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { TFunction } from 'i18next';
import { Alert, Button, Card, Segmented } from 'antd';
import {
  ArrowDownOutlined,
  ArrowUpOutlined,
  CloudServerOutlined,
  DashboardOutlined,
  HourglassOutlined,
  SwapOutlined,
  TeamOutlined,
} from '@ant-design/icons';

import { SizeFormatter } from '@/utils';
import { useTrafficOverviewQuery, type TrafficPeriod } from '@/api/queries/useTrafficOverviewQuery';
import { useNodesQuery } from '@/api/queries/useNodesQuery';
import { useStatusQuery } from '@/api/queries/useStatusQuery';
import type { TrafficHost, TrafficOverview } from '@/generated/zod';
import StatTile from './StatTile';
import DailyTrafficCard from './DailyTrafficCard';
import RankingCard from './RankingCard';
import HostOverviewCard from './HostOverviewCard';
import { liveSpeeds, sizeParts } from './trafficOverview';

const PERIOD_SINCE: Record<TrafficPeriod, string> = {
  today: 'pages.index.traffic.sinceToday',
  week: 'pages.index.traffic.sinceWeek',
  month: 'pages.index.traffic.sinceMonth',
};

function quotaDetail(servers: TrafficOverview['servers'], hosts: TrafficHost[], t: TFunction) {
  if (!servers.configured) return t('pages.index.traffic.probeOff');
  if (servers.error) return t('pages.index.traffic.probeError', { error: servers.error });
  const limited = hosts.filter((h) => h.linked && h.quotaBytes > 0).length;
  const detail = t('pages.index.traffic.serversQuotaDetail', {
    limited,
    unlimited: servers.unlimited,
  });
  return servers.unlinked > 0
    ? `${detail} · ${t('pages.index.traffic.unlinkedHosts', { count: servers.unlinked })}`
    : detail;
}

// A big panel sends only the busiest users, so the footer says how many the list holds.
function usersFooter(overview: TrafficOverview, t: TFunction) {
  if (overview.users === 0) return undefined;
  const shown = overview.userRanking.length;
  return shown < overview.users
    ? t('pages.index.traffic.usersFooterCapped', { count: overview.users, shown })
    : t('pages.index.traffic.usersFooter', { count: overview.users });
}

export default function TrafficOverviewSection({ isMobile }: { isMobile: boolean }) {
  const { t } = useTranslation();
  const [period, setPeriod] = useState<TrafficPeriod>('month');
  const { overview, fetched, fetchError, refetch } = useTrafficOverviewQuery(period);
  const { nodes } = useNodesQuery();
  const { status, fetched: statusFetched } = useStatusQuery();

  if (!fetched) return <Card loading />;
  if (!overview) {
    return (
      <Alert
        type="error"
        showIcon
        title={t('somethingWentWrong')}
        description={fetchError}
        action={
          <Button size="small" onClick={() => refetch()}>
            {t('refresh')}
          </Button>
        }
      />
    );
  }

  const { servers, hosts } = overview;
  const hostName = (host: { nodeId: number; name: string }) =>
    host.nodeId === 0 ? t('pages.inbounds.localPanel') : host.name;
  const speeds = liveSpeeds(hosts, nodes, statusFetched ? status.netIO : null);
  const quotaKnown = servers.configured && !servers.error;
  const [quotaValue, quotaUnit] = quotaKnown ? sizeParts(servers.quotaBytes) : ['—', ''];
  const [usedValue, usedUnit] = quotaKnown ? sizeParts(servers.usedBytes) : ['—', ''];
  const [remainingValue, remainingUnit] = quotaKnown
    ? sizeParts(servers.remainingBytes)
    : ['—', ''];

  return (
    <>
      <div className="ov-stats">
        <StatTile
          icon={<CloudServerOutlined />}
          label={t('pages.index.traffic.quota')}
          value={quotaValue}
          unit={quotaUnit}
          detail={quotaDetail(servers, hosts, t)}
        />
        <StatTile
          icon={<SwapOutlined />}
          label={t('pages.index.traffic.used')}
          value={usedValue}
          unit={usedUnit}
          detail={t('pages.index.traffic.serversUsedDetail')}
        />
        <StatTile
          icon={<HourglassOutlined />}
          label={t('pages.index.traffic.remaining')}
          value={remainingValue}
          unit={remainingUnit}
          detail={t('pages.index.traffic.serversRemainingDetail')}
        />
        <StatTile
          icon={<DashboardOutlined />}
          label={t('pages.index.traffic.speed')}
          value={
            <span className="ov-speed">
              <span className="ov-up">
                <ArrowUpOutlined /> {SizeFormatter.speedFormat(speeds.total.up)}
              </span>
              <span className="ov-down">
                <ArrowDownOutlined /> {SizeFormatter.speedFormat(speeds.total.down)}
              </span>
            </span>
          }
          detail={t('pages.index.traffic.speedDetail')}
        />
      </div>

      <DailyTrafficCard daily={overview.daily} isMobile={isMobile} />

      <div className="ov-period">
        <Segmented<TrafficPeriod>
          value={period}
          onChange={setPeriod}
          options={[
            { label: t('pages.index.traffic.periodToday'), value: 'today' },
            { label: t('pages.index.traffic.periodWeek'), value: 'week' },
            { label: t('pages.index.traffic.periodMonth'), value: 'month' },
          ]}
        />
        <span className="ov-sub">{t(PERIOD_SINCE[overview.period])}</span>
      </div>

      <div className="ov-ranks">
        <RankingCard
          icon={<CloudServerOutlined />}
          title={t('pages.index.traffic.hostRanking')}
          nameTitle={t('pages.index.traffic.colHost')}
          rows={overview.hostRanking.map((h) => ({
            key: String(h.nodeId),
            name: hostName(h),
            up: h.up,
            down: h.down,
          }))}
        />
        <RankingCard
          icon={<TeamOutlined />}
          title={t('pages.index.traffic.userRanking')}
          nameTitle={t('pages.index.traffic.colUser')}
          rows={overview.userRanking.map((u) => ({
            key: u.email,
            name: u.email,
            up: u.up,
            down: u.down,
          }))}
          footer={usersFooter(overview, t)}
        />
      </div>

      <HostOverviewCard hosts={hosts} speeds={speeds.byHost} hostName={hostName} />
    </>
  );
}
