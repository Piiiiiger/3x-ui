import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQueryClient } from '@tanstack/react-query';
import { Alert, Button, Card, Empty, Segmented, Select } from 'antd';
import {
  ApiOutlined,
  AppstoreOutlined,
  DollarOutlined,
  FolderOutlined,
  MessageOutlined,
  PlusOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';

import { HttpUtil } from '@/utils';
import { PageHeader } from '@/components/ui';
import { keys } from '@/api/queryKeys';
import {
  useAiUsageOverviewQuery,
  type AiUsageApp,
  type AiUsagePeriod,
} from '@/api/queries/useAiUsageOverviewQuery';
import type { AiUsageDeviceView } from '@/generated/zod';
import StatTile from '@/pages/index/StatTile';
import { useRelativeTime } from '@/pages/nodes/relativeTime';
import AiQuotaCard from './AiQuotaCard';
import AiDailyCard from './AiDailyCard';
import AiRankCard from './AiRankCard';
import AiSessionsCard from './AiSessionsCard';
import AiDevicesCard from './AiDevicesCard';
import ConnectDeviceModal from './ConnectDeviceModal';
import { cacheHitRate, formatTokens, formatUsd, projectName, quotaSlots } from './aiUsageFormat';

/** AI 用量: what Claude Code and Codex cost, where it went, and the plans' limits. */
export default function AiUsageSection({ isMobile }: { isMobile: boolean }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const relativeTime = useRelativeTime();
  const [period, setPeriod] = useState<AiUsagePeriod>('month');
  const [app, setApp] = useState<AiUsageApp>('all');
  const [deviceId, setDeviceId] = useState(0);
  const [connectOpen, setConnectOpen] = useState(false);
  // Ticks the reset countdowns and the stale-sync marks between refetches.
  const [nowMs, setNowMs] = useState(Date.now);
  useEffect(() => {
    const timer = window.setInterval(() => setNowMs(Date.now()), 30_000);
    return () => window.clearInterval(timer);
  }, []);
  const { overview, fetched, fetchError, refetch } = useAiUsageOverviewQuery(period, deviceId, app);

  const devices = overview?.devices ?? [];
  const header = (
    <PageHeader
      title={t('pages.aiUsage.title')}
      description={t('pages.aiUsage.intro')}
      extra={
        devices.length > 0 && (
          <div className="ai-filters">
            {devices.length > 1 && (
              <Select<number>
                value={deviceId}
                onChange={setDeviceId}
                popupMatchSelectWidth={false}
                aria-label={t('pages.aiUsage.colDevice')}
                options={[
                  { value: 0, label: t('pages.aiUsage.deviceAll') },
                  ...devices.map((d) => ({ value: d.id, label: d.name })),
                ]}
              />
            )}
            <Segmented<AiUsageApp>
              value={app}
              onChange={setApp}
              options={[
                { label: t('pages.aiUsage.appAll'), value: 'all' },
                { label: 'Claude', value: 'claude' },
                { label: 'Codex', value: 'codex' },
              ]}
            />
          </div>
        )
      }
    />
  );
  const connectModal = (
    <ConnectDeviceModal open={connectOpen} onClose={() => setConnectOpen(false)} />
  );

  if (!fetched) {
    return (
      <>
        {header}
        <Card loading />
      </>
    );
  }
  if (!overview) {
    return (
      <>
        {header}
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
      </>
    );
  }
  if (devices.length === 0) {
    return (
      <>
        {header}
        <Card>
          <Empty description={t('pages.aiUsage.empty')}>
            <p className="ai-empty-hint">{t('pages.aiUsage.emptyHint')}</p>
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setConnectOpen(true)}>
              {t('pages.aiUsage.connect')}
            </Button>
          </Empty>
        </Card>
        {connectModal}
      </>
    );
  }

  const { totals } = overview;
  const tokens =
    totals.inputTokens + totals.outputTokens + totals.cacheReadTokens + totals.cacheWriteTokens;
  const hit = cacheHitRate(totals);
  const deleteDevice = async (device: AiUsageDeviceView) => {
    const msg = await HttpUtil.post(`/panel/api/aiUsage/devices/delete/${device.id}`);
    if (msg?.success) {
      if (deviceId === device.id) setDeviceId(0);
      await queryClient.invalidateQueries({ queryKey: keys.aiUsage.root() });
    }
  };

  return (
    <>
      {header}
      <div className="ov-stats">
        <StatTile
          icon={<DollarOutlined />}
          label={t('pages.aiUsage.cost')}
          value={formatUsd(totals.costUsd)}
          detail={t('pages.aiUsage.costDetail', {
            claude: formatUsd(totals.claudeCostUsd),
            codex: formatUsd(totals.codexCostUsd),
          })}
        />
        <StatTile
          icon={<ThunderboltOutlined />}
          label={t('pages.aiUsage.tokens')}
          value={formatTokens(tokens)}
          detail={
            hit === null
              ? ' '
              : t('pages.aiUsage.tokensDetail', { rate: `${(hit * 100).toFixed(1)}%` })
          }
        />
        <StatTile
          icon={<ApiOutlined />}
          label={t('pages.aiUsage.requests')}
          value={totals.requests.toLocaleString()}
          detail={t('pages.aiUsage.requestsDetail', {
            input: formatTokens(totals.inputTokens),
            output: formatTokens(totals.outputTokens),
          })}
        />
        <StatTile
          icon={<MessageOutlined />}
          label={t('pages.aiUsage.sessions')}
          value={totals.sessions.toLocaleString()}
          detail={t('pages.aiUsage.sessionsDetail', { projects: overview.projects.length })}
        />
      </div>

      <div className="ai-quota-grid">
        {quotaSlots(overview.quotas).map((slot) => (
          <AiQuotaCard key={slot.tool} slot={slot} nowMs={nowMs} relativeTime={relativeTime} />
        ))}
      </div>

      <AiDailyCard daily={overview.daily} isMobile={isMobile} />

      <div className="ov-period">
        <Segmented<AiUsagePeriod>
          value={period}
          onChange={setPeriod}
          options={[
            { label: t('pages.index.traffic.periodToday'), value: 'today' },
            { label: t('pages.index.traffic.periodWeek'), value: 'week' },
            { label: t('pages.index.traffic.periodMonth'), value: 'month' },
            { label: t('pages.aiUsage.periodAll'), value: 'all' },
          ]}
        />
        <span className="ov-sub">
          {overview.period === 'all'
            ? t('pages.aiUsage.sinceAll')
            : t('pages.aiUsage.since', { date: overview.periodStart })}
        </span>
      </div>

      <div className="ov-ranks">
        <AiRankCard
          icon={<FolderOutlined />}
          title={t('pages.aiUsage.projects')}
          nameTitle={t('pages.aiUsage.colProject')}
          rows={overview.projects}
          displayName={(row) => projectName(row.name) || t('pages.aiUsage.unknownProject')}
        />
        <AiRankCard
          icon={<AppstoreOutlined />}
          title={t('pages.aiUsage.models')}
          nameTitle={t('pages.aiUsage.colModel')}
          rows={overview.models}
        />
      </div>

      <AiSessionsCard
        sessions={overview.sessions}
        relativeTime={relativeTime}
        showDevice={devices.length > 1}
      />

      <AiDevicesCard
        devices={devices}
        nowMs={nowMs}
        relativeTime={relativeTime}
        onConnect={() => setConnectOpen(true)}
        onDelete={deleteDevice}
      />
      {connectModal}
    </>
  );
}
